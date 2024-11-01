/*
Copyright 2022 The Kubernetes Authors.

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

package cacher

import (
	"context"
	"fmt"
	"sync"
)

type status int

const (
	Pending status = iota
	Ready
	Stopped
)

// ready is a three state condition variable that blocks until is Ready if is not Stopped.
// Its initial state is Pending and its state machine diagram is as follow.
//
// Pending <------> Ready -----> Stopped
//
//	|                           ^
//	└---------------------------┘
type ready struct {
	state       status        // represent the state of the variable
	lock        sync.RWMutex  // protect the state variable
	restartLock sync.Mutex    // protect the transition from ready to pending where the channel is recreated
	waitCh      chan struct{} // blocks until is ready or stopped
}

func newReady() *ready {
	return &ready{
		waitCh: make(chan struct{}),
		state:  Pending,
	}
}

// done close the channel once the state is Ready or Stopped
func (r *ready) done() chan struct{} {
	r.restartLock.Lock()
	defer r.restartLock.Unlock()
	return r.waitCh
}

// wait blocks until it is Ready or Stopped, it returns an error if is Stopped.
func (r *ready) wait(ctx context.Context) error {
	for {
		// r.done() only blocks if state is Pending
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.done():
		}

		r.lock.RLock()
		switch r.state {
		case Pending:
			// since we allow to switch between the states Pending and Ready
			// if there is a quick transition from Pending -> Ready -> Pending
			// a process that was waiting can get unblocked and see a Pending
			// state again. If the state is Pending we have to wait again to
			// avoid an inconsistent state on the system, with some processes not
			// waiting despite the state moved back to Pending.
			r.lock.RUnlock()
		case Ready:
			r.lock.RUnlock()
			return nil
		case Stopped:
			r.lock.RUnlock()
			return fmt.Errorf("apiserver cacher is stopped")
		default:
			r.lock.RUnlock()
			return fmt.Errorf("unexpected apiserver cache state: %v", r.state)
		}
	}
}

// check returns true only if it is Ready.
func (r *ready) check() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.state == Ready
}

// set the state to Pending (false) or Ready (true), it does not have effect if the state is Stopped.
func (r *ready) set(ok bool) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.state == Stopped {
		return
	}
	if ok && r.state == Pending {
		r.state = Ready
		select {
		case <-r.waitCh:
		default:
			close(r.waitCh)
		}
	} else if !ok && r.state == Ready {
		// creating the waitCh can be racy if
		// something enter the wait() method
		select {
		case <-r.waitCh:
			r.restartLock.Lock()
			r.waitCh = make(chan struct{})
			r.restartLock.Unlock()
		default:
		}
		r.state = Pending
	}
}

// stop the condition variable and set it as Stopped. This state is irreversible.
func (r *ready) stop() {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.state != Stopped {
		r.state = Stopped
	}
	select {
	case <-r.waitCh:
	default:
		close(r.waitCh)
	}
}
