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
	"os"
	"os/signal"
	"syscall"

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

// runLoop waits for all child processes that exit
// currently only stunnel process is created by efs mount helper
// and is inherited as the child process of the driver
func (r *reaper) runLoop() {
	for {
		select {
		case <-r.sigs:
			var status syscall.WaitStatus
			var rusage syscall.Rusage
			childPid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, &rusage)
			if err != nil {
				klog.Warningf("Failed to wait for child process %s", err)
			} else {
				klog.V(4).Infof("Waited for child process %d", childPid)
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
