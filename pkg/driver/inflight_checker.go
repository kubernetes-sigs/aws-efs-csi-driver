package driver

import (
	"sync"

	"k8s.io/klog/v2"
)

type InFlightChecker struct {
	mux      sync.Mutex
	count    int64
	maxCount int64
}

func NewInFlightChecker(maxCount int64) *InFlightChecker {
	if maxCount < 0 {
		klog.V(4).InfoS("InFlightChecker is disabled")
		return nil
	}
	return &InFlightChecker{
		maxCount: maxCount,
	}
}

func (checker *InFlightChecker) increment() bool {
	checker.mux.Lock()
	defer checker.mux.Unlock()

	if checker.count >= checker.maxCount {
		return false
	}

	checker.count++
	return true
}

func (checker *InFlightChecker) decrement() {
	checker.mux.Lock()
	defer checker.mux.Unlock()
	if checker.count == 0 {
		klog.Error("InFlightChecker: trying to decrement count when it is already 0")
		return
	}

	checker.count--
}
