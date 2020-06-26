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
	"os/exec"
	"sync"

	"k8s.io/klog"
)

// Watchdog defines the interface for process monitoring and supervising
type Watchdog interface {
	// start starts the watch dog along with the process
	start()

	// stop stops the watch dog along with the process
	stop()
}

// execWatchdog is a watch dog that monitors a process and restart it
// if it has crashed accidentally
type execWatchdog struct {
	// the command to be exec and monitored
	execCmd string
	// the command arguments
	execArg []string
	// the cmd that is running
	cmd *exec.Cmd
	// stopCh indicates if it should be stopped
	stopCh chan struct{}

	mu sync.Mutex
}

func newExecWatchdog(cmd string, arg ...string) Watchdog {
	return &execWatchdog{
		execCmd: cmd,
		execArg: arg,
		stopCh:  make(chan struct{}),
	}
}

func (w *execWatchdog) start() {
	go w.runLoop(w.stopCh)
}

// stop kills the underlying process and stops the watchdog
func (w *execWatchdog) stop() {
	close(w.stopCh)

	w.mu.Lock()
	if w.cmd.Process != nil {
		p := w.cmd.Process
		err := p.Kill()
		if err != nil {
			klog.Errorf("Failed to kill process: %s", err)
		}
	}
	w.mu.Unlock()
}

// runLoop starts the monitoring loop
func (w *execWatchdog) runLoop(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			klog.Info("stopping...")
			break
		default:
			err := w.exec()
			if err != nil {
				klog.Errorf("Process %s exits %s", w.execCmd, err)
			}
		}
	}
}

func (w *execWatchdog) exec() error {
	cmd := exec.Command(w.execCmd, w.execArg...)
	cmd.Stdout = newInfoRedirect(w.execCmd)
	cmd.Stderr = newErrRedirect(w.execCmd)

	w.cmd = cmd

	w.mu.Lock()
	err := cmd.Start()
	if err != nil {
		return err
	}
	w.mu.Unlock()

	return cmd.Wait()
}

type logRedirect struct {
	processName string
	level       string
	logFunc     func(string, ...interface{})
}

func newInfoRedirect(name string) *logRedirect {
	return &logRedirect{
		processName: name,
		level:       "Info",
		logFunc:     klog.V(4).Infof,
	}
}

func newErrRedirect(name string) *logRedirect {
	return &logRedirect{
		processName: name,
		level:       "Error",
		logFunc:     klog.Errorf,
	}
}
func (l *logRedirect) Write(p []byte) (n int, err error) {
	msg := fmt.Sprintf("%s[%s]: %s", l.processName, l.level, string(p))
	l.logFunc("%s", msg)
	return len(msg), nil
}
