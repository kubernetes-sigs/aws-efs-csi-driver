package driver

import (
	"sync"

	"k8s.io/klog/v2"
)

type InFlightMountTracker struct {
	mux      sync.Mutex
	count    int64
	maxCount int64
}

func NewInFlightMountTracker(maxCount int64) *InFlightMountTracker {
	if maxCount <= 0 {
		klog.V(4).InfoS("InFlightMountTracker is disabled")
		return nil
	}
	return &InFlightMountTracker{
		maxCount: maxCount,
	}
}

func (checker *InFlightMountTracker) increment() bool {
	checker.mux.Lock()
	defer checker.mux.Unlock()

	if checker.count >= checker.maxCount {
		return false
	}

	checker.count++
	return true
}

func (checker *InFlightMountTracker) decrement() bool {
	checker.mux.Lock()
	defer checker.mux.Unlock()
	if checker.count == 0 {
		klog.Error("InFlightMountTracker: trying to decrement count when it is already 0")
		return false
	}

	checker.count--
	return true
}
