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
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/mitchellh/go-ps"
)

// These tests run in an ordinary process, not as PID 1. reapOrphanedZombies
// compares against os.Getpid(), so zombies created by the test process are
// exactly what a production run sees for orphans reparented to the driver.

func TestReaper(t *testing.T) {
	r := newReaper()

	r.start()
	time.Sleep(time.Second)
	r.stop()
}

func TestStopEndsLoop(t *testing.T) {
	r := newReaper()
	before := runtime.NumGoroutine()
	r.start()
	time.Sleep(50 * time.Millisecond)
	r.stop()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("reaper goroutine did not exit after stop()")
}

// spawnZombie forks a child that exits immediately and is not waited on, so
// it stays a zombie owned by this process until something calls Wait4 on it.
func spawnZombie(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if isZombie(pid) {
			return pid
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pid %d never became a zombie", pid)
	return 0
}

func reapDirect(pid int) {
	var ws syscall.WaitStatus
	var ru syscall.Rusage
	_, _ = syscall.Wait4(pid, &ws, syscall.WNOHANG, &ru)
}

// newTestReaper returns a reaper with a controllable clock and no signal
// subscription, so passes are driven explicitly.
func newTestReaper(start time.Time) (*reaper, *time.Time) {
	clock := start
	r := &reaper{
		stopCh:    make(chan struct{}),
		firstSeen: map[procKey]time.Time{},
		now:       func() time.Time { return clock },
	}
	return r, &clock
}

func TestIsZombie(t *testing.T) {
	pid := spawnZombie(t)
	defer reapDirect(pid)
	if !isZombie(pid) {
		t.Fatalf("expected pid %d to be a zombie", pid)
	}
	if isZombie(os.Getpid()) {
		t.Fatalf("test process must not be reported as a zombie")
	}
	if isZombie(1<<22 + 12345) {
		t.Fatalf("nonexistent pid must not be reported as a zombie")
	}
}

func TestProcInfo(t *testing.T) {
	self, ok := procInfo(os.Getpid())
	if !ok || self.start == 0 || self.state == 'Z' || self.ppid != os.Getppid() {
		t.Fatalf("self: ok=%v %+v, want ppid %d", ok, self, os.Getppid())
	}
	pid := spawnZombie(t)
	defer reapDirect(pid)
	if z, ok := procInfo(pid); !ok || z.state != 'Z' || z.start == 0 || z.ppid != os.Getpid() || z.comm != "true" {
		t.Fatalf("zombie: ok=%v %+v, want ppid %d comm true", ok, z, os.Getpid())
	}
	if _, ok := procInfo(1<<22 + 12345); ok {
		t.Fatalf("nonexistent pid must fail closed")
	}
}

// spawnNamedZombie starts a child whose command name is set to name via
// PR_SET_NAME and leaves it unwaited.
func spawnNamedZombie(t *testing.T, name string) int {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", `printf '%s' "$0" > /proc/self/comm && exit 0`, name)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := procInfo(pid); ok && p.state == 'Z' && p.comm == name {
			return pid
		}
		time.Sleep(5 * time.Millisecond)
	}
	p, _ := procInfo(pid)
	t.Fatalf("pid %d never became a zombie named %q, last %+v", pid, name, p)
	return 0
}

// A ')' in the command name must not shift the fields read after it; a
// misparse here would hide the parent pid the reaper filters on.
func TestProcInfoParenthesisInName(t *testing.T) {
	for _, name := range []string{"weird)name", "a) Z 1 2 3", "((x))"} {
		pid := spawnNamedZombie(t, name)
		p, ok := procInfo(pid)
		reapDirect(pid)
		if !ok || p.comm != name || p.state != 'Z' || p.ppid != os.Getpid() {
			t.Fatalf("name %q: ok=%v %+v, want ppid %d", name, ok, p, os.Getpid())
		}
	}
}

func TestReapsZombieWithParenthesisInName(t *testing.T) {
	r, clock := newTestReaper(time.Unix(1_000_000, 0))
	pid := spawnNamedZombie(t, "weird)name")
	r.reapOrphanedZombies()
	*clock = clock.Add(2 * minZombieDuration)
	r.reapOrphanedZombies()
	if isZombie(pid) {
		reapDirect(pid)
		t.Fatalf("zombie named with ')' was not reaped")
	}
}

func TestStopIsIdempotent(t *testing.T) {
	r := newReaper()
	r.start()
	done := make(chan struct{})
	go func() {
		r.stop()
		r.stop()
		r.stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("repeated stop() blocked")
	}
}

func TestReapsAfterMinZombieDuration(t *testing.T) {
	r, clock := newTestReaper(time.Unix(1_000_000, 0))
	pid := spawnZombie(t)
	defer reapDirect(pid)

	r.reapOrphanedZombies()
	if !isZombie(pid) {
		t.Fatalf("zombie must not be reaped on first sighting")
	}
	*clock = clock.Add(minZombieDuration - time.Second)
	r.reapOrphanedZombies()
	if !isZombie(pid) {
		t.Fatalf("zombie must not be reaped before minZombieDuration")
	}
	*clock = clock.Add(2 * time.Second)
	r.reapOrphanedZombies()
	if isZombie(pid) {
		t.Fatalf("zombie should have been reaped after minZombieDuration")
	}
}

func TestManyPassesInAnInstantDoNotReap(t *testing.T) {
	// A SIGCHLD storm can drive many passes within milliseconds. Frequency of
	// passes must not shorten the wait; only elapsed time may.
	r, _ := newTestReaper(time.Unix(1_000_000, 0))
	pid := spawnZombie(t)
	defer reapDirect(pid)
	for i := 0; i < 100; i++ {
		r.reapOrphanedZombies()
	}
	if !isZombie(pid) {
		t.Fatalf("100 passes at the same instant must not reap")
	}
}

func TestLiveChildNeverReaped(t *testing.T) {
	r, clock := newTestReaper(time.Unix(1_000_000, 0))
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	for i := 0; i < 5; i++ {
		r.reapOrphanedZombies()
		*clock = clock.Add(minZombieDuration)
	}
	if _, tracked := r.firstSeen[procKey{pid: cmd.Process.Pid}]; tracked {
		t.Fatalf("a live process must never be tracked as a zombie")
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("live child was disturbed: %v", err)
	}
}

// processState returns the single-letter state (field 3) from /proc/<pid>/stat.
func processState(t *testing.T, pid int) string {
	t.Helper()
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		t.Fatalf("read stat for %d: %v", pid, err)
	}
	s := string(b)
	return strings.Fields(s[strings.LastIndex(s, ")")+2:])[0]
}

func waitForState(t *testing.T, pid int, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if processState(t, pid) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pid %d never reached state %s (now %s)", pid, want, processState(t, pid))
}

// TestNonZombieStatesNeverReaped drives full reaper passes, with the clock
// advanced far past minZombieDuration, against children in every live state
// the kernel can report -- running, sleeping, and stopped -- and asserts none
// is tracked or touched. A real zombie runs alongside as the positive control.
func TestNonZombieStatesNeverReaped(t *testing.T) {
	r, clock := newTestReaper(time.Unix(1_000_000, 0))

	running := exec.Command("sh", "-c", "while :; do :; done")
	sleeping := exec.Command("sleep", "60")
	stopped := exec.Command("sleep", "60")
	for _, c := range []*exec.Cmd{running, sleeping, stopped} {
		if err := c.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		defer func(c *exec.Cmd) { _ = c.Process.Kill(); _ = c.Wait() }(c)
	}
	if err := stopped.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("SIGSTOP: %v", err)
	}
	waitForState(t, running.Process.Pid, "R")
	waitForState(t, sleeping.Process.Pid, "S")
	waitForState(t, stopped.Process.Pid, "T")

	zombie := spawnZombie(t)
	defer reapDirect(zombie)

	live := map[string]int{"R": running.Process.Pid, "S": sleeping.Process.Pid, "T": stopped.Process.Pid}
	for i := 0; i < 6; i++ {
		r.reapOrphanedZombies()
		*clock = clock.Add(minZombieDuration)
	}

	for want, pid := range live {
		if got := processState(t, pid); got != want {
			t.Fatalf("live child %d changed state %s -> %s", pid, want, got)
		}
		if err := syscall.Kill(pid, 0); err != nil {
			t.Fatalf("live child %d was disturbed: %v", pid, err)
		}
		st, _ := procInfo(pid)
		start := st.start
		if _, tracked := r.firstSeen[procKey{pid: pid, start: start}]; tracked {
			t.Fatalf("live child %d in state %s must never be tracked as a zombie", pid, want)
		}
	}
	if isZombie(zombie) {
		t.Fatalf("positive control: the real zombie should have been reaped")
	}
}

func TestIdentityIncludesStartTime(t *testing.T) {
	r, clock := newTestReaper(time.Unix(1_000_000, 0))
	pid := spawnZombie(t)
	st, _ := procInfo(pid)
	start := st.start
	r.reapOrphanedZombies()
	if _, ok := r.firstSeen[procKey{pid: pid, start: start}]; !ok {
		t.Fatalf("zombie should be tracked under its (pid, start) identity")
	}
	if _, ok := r.firstSeen[procKey{pid: pid, start: start + 1}]; ok {
		t.Fatalf("a different start time must be a different identity")
	}
	*clock = clock.Add(2 * minZombieDuration)
	r.reapOrphanedZombies()
	if isZombie(pid) {
		t.Fatalf("zombie should have been reaped")
	}
	if len(r.firstSeen) != 0 {
		t.Fatalf("reaped zombie must be forgotten, got %v", r.firstSeen)
	}
}

func TestForgetsZombiesCollectedElsewhere(t *testing.T) {
	r, clock := newTestReaper(time.Unix(1_000_000, 0))
	pid := spawnZombie(t)
	r.reapOrphanedZombies()
	reapDirect(pid) // someone else (os/exec) collected it
	*clock = clock.Add(2 * minZombieDuration)
	r.reapOrphanedZombies()
	if len(r.firstSeen) != 0 {
		t.Fatalf("a zombie collected elsewhere must drop out of tracking, got %v", r.firstSeen)
	}
}

// TestNoStealFromExec is the regression guard for the one real risk of a
// general reaper: collecting a child that os/exec is about to Wait() on. The
// reaper runs live here with production thresholds shortened only in interval,
// not in the zombie-duration guard, while os/exec children of every lifetime
// exit and are waited on.
func TestNoStealFromExec(t *testing.T) {
	r := newReaper()
	r.sweepInterval = 10 * time.Millisecond
	r.start()
	defer r.stop()

	for i := 0; i < 60; i++ {
		want := i % 3
		cmd := exec.Command("sh", "-c", fmt.Sprintf("sleep 0.0%d; exit %d", i%5, want))
		err := cmd.Run()
		if err != nil && strings.Contains(err.Error(), "no child processes") {
			t.Fatalf("iteration %d: exit status was stolen by the reaper", i)
		}
		if want == 0 && err != nil {
			t.Fatalf("iteration %d: unexpected error %v", i, err)
		}
		if want != 0 {
			ee, ok := err.(*exec.ExitError)
			if !ok || ee.ExitCode() != want {
				t.Fatalf("iteration %d: want exit %d, got %v", i, want, err)
			}
		}
	}
}

// TestNoStealWhenWaitIsDelayed is the reason minZombieDuration exists. A child
// that has exited but whose parent has not yet called Wait() is a zombie with
// our pid as parent, indistinguishable in /proc from an orphan. Only the time
// it has spent as a zombie separates the two. Here the parent deliberately
// stalls between the child exiting and Wait(); the shipped guard must keep the
// reaper's hands off every one of them. With the guard at zero the reaper
// collects all of them, which is what lowering the constant would reintroduce.
func TestNoStealWhenWaitIsDelayed(t *testing.T) {
	r := newReaper()
	r.sweepInterval = 5 * time.Millisecond
	r.start()
	defer r.stop()

	const children = 50
	// The stall is long enough that any guard a reviewer might plausibly
	// "tune down" (hundreds of milliseconds, a second or two) is caught. The
	// shipped guard must also keep a wide margin over the stall itself.
	stall := 2 * time.Second
	if minZombieDuration < 5*stall {
		t.Fatalf("minZombieDuration %v is under 5x the %v stall this test simulates; do not lower it", minZombieDuration, stall)
	}
	var wg sync.WaitGroup
	var stolen atomic.Int64
	sem := make(chan struct{}, 25)
	for i := 0; i < children; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			cmd := exec.Command("true")
			if err := cmd.Start(); err != nil {
				t.Errorf("start: %v", err)
				return
			}
			time.Sleep(stall)
			if err := cmd.Wait(); err != nil && strings.Contains(err.Error(), "no child processes") {
				stolen.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := stolen.Load(); got != 0 {
		t.Fatalf("%d of %d exit statuses stolen with a %v stall and %v guard", got, children, stall, minZombieDuration)
	}
}

// TestWatchdogChildIsOwnedWhileRunning proves the watchdog actually uses the
// owned-children registry: while exec() is running its child, that pid is
// registered and a reaper pass will not even track it; once exec() returns the
// pid is released. Removing the ownChild/disownChild calls from exec() fails
// this test.
func TestWatchdogChildIsOwnedWhileRunning(t *testing.T) {
	w := &execWatchdog{execCmd: "sleep", execArg: []string{"30"}, stopCh: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- w.exec() }()

	// Find the child through the registry itself: it is the only child of this
	// test process that exec() will have registered. w.cmd is deliberately not
	// read here; exec() assigns it outside the lock (pre-existing).
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		procs, err := ps.Processes()
		if err != nil {
			t.Fatalf("list procs: %v", err)
		}
		for _, p := range procs {
			if p.PPid() == os.Getpid() && isOwnedChild(p.Pid()) {
				pid = p.Pid()
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatalf("watchdog child was not registered with ownChild while running")
	}

	// Kill the child so it becomes a zombie the moment exec()'s Wait() is
	// racing to collect it; a reaper pass in this window must ignore it.
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill: %v", err)
	}
	r, clock := newTestReaper(time.Unix(1_000_000, 0))
	for i := 0; i < 3; i++ {
		r.reapOrphanedZombies()
		*clock = clock.Add(minZombieDuration)
	}
	st, _ := procInfo(pid)
	start := st.start
	if _, tracked := r.firstSeen[procKey{pid: pid, start: start}]; tracked {
		t.Fatalf("reaper tracked the watchdog's owned child %d", pid)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected exec() to report the killed child")
		}
		if strings.Contains(err.Error(), "no child processes") {
			t.Fatalf("watchdog's exit status was stolen: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("exec() did not return after child was killed")
	}
	if isOwnedChild(pid) {
		t.Fatalf("watchdog child %d still registered after exec() returned", pid)
	}
}

func TestOwnedChildren(t *testing.T) {
	const pid = 424242
	if isOwnedChild(pid) {
		t.Fatalf("should not be owned before registration")
	}
	ownChild(pid)
	if !isOwnedChild(pid) {
		t.Fatalf("should be owned after ownChild")
	}
	disownChild(pid)
	if isOwnedChild(pid) {
		t.Fatalf("should not be owned after disownChild")
	}
}

func TestOwnedZombieNeverReaped(t *testing.T) {
	r, clock := newTestReaper(time.Unix(1_000_000, 0))
	pid := spawnZombie(t)
	defer reapDirect(pid)
	ownChild(pid)
	defer disownChild(pid)
	for i := 0; i < 5; i++ {
		r.reapOrphanedZombies()
		*clock = clock.Add(minZombieDuration)
	}
	if !isZombie(pid) {
		t.Fatalf("an owned child must never be reaped, however long it is a zombie")
	}
	if len(r.firstSeen) != 0 {
		t.Fatalf("an owned child must not even be tracked, got %v", r.firstSeen)
	}
}

func TestDisownedZombieIsReaped(t *testing.T) {
	r, clock := newTestReaper(time.Unix(1_000_000, 0))
	pid := spawnZombie(t)
	ownChild(pid)
	r.reapOrphanedZombies()
	disownChild(pid)
	r.reapOrphanedZombies()
	*clock = clock.Add(2 * minZombieDuration)
	r.reapOrphanedZombies()
	if isZombie(pid) {
		reapDirect(pid)
		t.Fatalf("a disowned zombie should be reaped once minZombieDuration has passed")
	}
}

func TestSafePassRecoversFromPanic(t *testing.T) {
	r := newReaper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if rec := recover(); rec != nil {
				t.Errorf("panic escaped safePass: %v", rec)
			}
		}()
		r.safePass()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("safePass did not return")
	}
}
