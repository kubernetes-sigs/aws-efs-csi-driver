/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"k8s.io/klog/v2"
)

// Reaping thresholds.
const (
	// defaultSweepInterval bounds how long an orphaned zombie can linger when no further
	// SIGCHLD arrives to trigger a pass.
	defaultSweepInterval = 5 * time.Second
	// minZombieDuration is how long a process must have been observed as a
	// zombie before it is reaped. Children the driver waits on through os/exec
	// (mount, umount, the watchdog) are zombies only for the instant between
	// exiting and cmd.Wait() returning, so they can never reach this.
	//
	// Do not lower this casually. If the reaper ever collects a child that
	// os/exec is about to Wait() on, cmd.Wait() returns "no child processes"
	// with a nil ProcessState; mount-utils then either reports a spurious
	// mount failure or, on Go versions where the error text still matches its
	// tolerance branch, dereferences the nil ProcessState and panics. Ten
	// seconds against a sub-millisecond normal case is the margin that makes
	// that impossible without a multi-second stall elsewhere in the driver.
	minZombieDuration = 10 * time.Second
)

// procKey identifies a process by pid and start time, so a pid recycled for a
// new process is never confused with the one that held it before.
type procKey struct {
	pid   int
	start uint64
}

// ownedChildren holds pids of children the driver started itself and will
// Wait() on (currently the efs-utils watchdog). The reaper never touches them
// regardless of state or how long they have been zombies. Exec sites register
// with ownChild after Start() and release with disownChild after Wait().
var (
	ownedChildrenMu sync.Mutex
	ownedChildren   = map[int]bool{}
)

// ownChild marks pid as a child the driver will Wait() on itself.
func ownChild(pid int) {
	ownedChildrenMu.Lock()
	defer ownedChildrenMu.Unlock()
	ownedChildren[pid] = true
}

// disownChild releases a pid registered with ownChild.
func disownChild(pid int) {
	ownedChildrenMu.Lock()
	defer ownedChildrenMu.Unlock()
	delete(ownedChildren, pid)
}

func isOwnedChild(pid int) bool {
	ownedChildrenMu.Lock()
	defer ownedChildrenMu.Unlock()
	return ownedChildren[pid]
}

type reaper struct {
	sigs     chan os.Signal
	stopCh   chan struct{}
	stopOnce sync.Once
	// firstSeen records when each zombie child was first observed. A zombie
	// is reaped once it has been a zombie for at least minZombieDuration.
	firstSeen     map[procKey]time.Time
	sweepInterval time.Duration
	now           func() time.Time
}

func newReaper() *reaper {
	sigs := make(chan os.Signal, 1)
	stopCh := make(chan struct{})

	signal.Notify(sigs, syscall.SIGCHLD)
	return &reaper{
		sigs:          sigs,
		stopCh:        stopCh,
		firstSeen:     map[procKey]time.Time{},
		now:           time.Now,
		sweepInterval: defaultSweepInterval,
	}
}

func (r *reaper) start() {
	go r.runLoop()
}

func (r *reaper) stop() {
	r.stopOnce.Do(func() { close(r.stopCh) })
}

// runLoop reaps zombie processes that have been reparented to the driver. The
// driver is PID 1 in its container, so any process whose parent exits without
// waiting on it becomes a zombie that only the driver can collect. On
// clusters with a pod PID limit uncollected zombies count against it.
func (r *reaper) runLoop() {
	ticker := time.NewTicker(r.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.sigs:
			r.safePass()
		case <-ticker.C:
			r.safePass()
		case <-r.stopCh:
			return
		}
	}
}

// safePass runs one reaping pass and guarantees the loop survives a panic from
// unexpected /proc content, which must never take the reaper down.
func (r *reaper) safePass() {
	defer func() {
		if rec := recover(); rec != nil {
			klog.Errorf("reaper: recovered from panic during pass: %v", rec)
		}
	}()
	r.reapOrphanedZombies()
}

// reapOrphanedZombies waits on every zombie child of this process that has
// been a zombie for at least minZombieDuration. The parent check is against
// our own pid rather than 1, so the reaper does the right thing if the driver
// is ever run behind an init or with hostPID: orphans then belong to that init
// and we reap nothing.
func (r *reaper) reapOrphanedZombies() {
	pids, err := listPids()
	if err != nil {
		klog.Warningf("reaper: failed to get all procs: %v", err)
		return
	}
	self := os.Getpid()
	now := r.now()
	seen := map[procKey]time.Time{}
	for _, pid := range pids {
		if pid == self || isOwnedChild(pid) {
			continue
		}
		p, ok := procInfo(pid)
		if !ok || p.ppid != self || p.state != 'Z' {
			continue
		}
		key := procKey{pid: pid, start: p.start}
		first, known := r.firstSeen[key]
		if !known {
			first = now
		}
		seen[key] = first
		if now.Sub(first) < minZombieDuration {
			continue
		}
		var wstatus syscall.WaitStatus
		var rusage syscall.Rusage
		wpid, err := syscall.Wait4(pid, &wstatus, syscall.WNOHANG, &rusage)
		if err != nil {
			klog.Warningf("reaper: failed to wait for process %v (%s): %v", pid, p.comm, err)
			continue
		}
		if wpid != pid {
			// Not waitable yet; keep the record and retry next pass.
			continue
		}
		delete(seen, key)
		klog.V(2).Infof("reaper: waited for orphaned process %v (%s), zombie for %s", pid, p.comm, now.Sub(first).Round(time.Second))
	}
	r.firstSeen = seen
}

// listPids returns the pid of every process visible in /proc.
func listPids() ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	pids := make([]int, 0, len(entries))
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// procStat holds the /proc/<pid>/stat fields the reaper uses.
type procStat struct {
	comm  string
	state byte
	ppid  int
	start uint64
}

// procInfo reads /proc/<pid>/stat once and returns the command name (field
// 2), run state (field 3), parent pid (field 4) and start time in clock ticks
// since boot (field 22)
func procInfo(pid int) (procStat, bool) {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%v/stat", pid))
	if err != nil {
		// The process was collected between listing and reading.
		return procStat{}, false
	}
	s := string(stat)
	open := strings.Index(s, "(")
	end := strings.LastIndex(s, ")")
	if open < 0 || end < open {
		return procStat{}, false
	}
	fields := strings.Fields(s[end+1:])
	// starttime is field 22 overall, so index 19 here.
	if len(fields) < 20 || len(fields[0]) != 1 {
		return procStat{}, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return procStat{}, false
	}
	start, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return procStat{}, false
	}
	return procStat{comm: s[open+1 : end], state: fields[0][0], ppid: ppid, start: start}, true
}

func isZombie(pid int) bool {
	p, ok := procInfo(pid)
	return ok && p.state == 'Z'
}
