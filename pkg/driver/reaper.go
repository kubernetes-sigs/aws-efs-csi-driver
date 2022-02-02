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
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mitchellh/go-ps"
	"k8s.io/klog"
)

type reaper struct {
	sigs   chan os.Signal
	stopCh chan struct{}
}

func newReaper() *reaper {
	sigs := make(chan os.Signal, 1)
	stopCh := make(chan struct{})

	signal.Notify(sigs, syscall.SIGCHLD)
	return &reaper{
		sigs:   sigs,
		stopCh: stopCh,
	}
}

// start starts the reaper
func (r *reaper) start() {
	go r.runLoop()
}

// runLoop catches SIGCHLD signals to remove stunnel zombie processes. stunnel
// processes are guaranteed to become zombies because their parent is the
// driver process but they get terminated by the efs-utils watchdog process:
// https://github.com/aws/efs-utils/blob/v1.31.2/src/watchdog/__init__.py#L608
// pid_max is 4194303.
func (r *reaper) runLoop() {
	for {
		select {
		case <-r.sigs:
			procs, err := ps.Processes()
			if err != nil {
				klog.Warningf("reaper: failed to get all procs: %v", err)
			} else {
				for _, p := range procs {
					reaped := waitIfZombieStunnel(p)
					if reaped {
						// wait for only one process per SIGCHLD received over channel. It
						// doesn't have to be the same process that triggered the
						// particular SIGCHLD (there's no way to tell anyway), the
						// intention is to reap zombies as they come.
						break
					}
				}
			}
		case <-r.stopCh:
			break
		}
	}
}

// stop stops the reaper
func (r *reaper) stop() {
	r.stopCh <- struct{}{}
}

func waitIfZombieStunnel(p ps.Process) bool {
	if !strings.Contains(p.Executable(), "stunnel") {
		return false
	}
	data, err := ioutil.ReadFile(fmt.Sprintf("/proc/%v/status", p.Pid()))
	if err != nil {
		klog.Warningf("reaper: failed to read process %v's status: %v", p, err)
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "State") {
			if strings.Contains(line, "zombie") {
				var wstatus syscall.WaitStatus
				var rusage syscall.Rusage
				_, err := syscall.Wait4(p.Pid(), &wstatus, 0, &rusage)
				if err != nil {
					klog.Warningf("reaper: failed to wait for process %v: %v", p, err)
				}
				klog.V(4).Infof("reaper: waited for process %v", p)
				return true
			} else {
				return false
			}
		}
	}

	return false
}
