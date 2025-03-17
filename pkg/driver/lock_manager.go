package driver

import (
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"
)

type LockManager struct {
	mutex    sync.Mutex
	refCount atomic.Int32
	isLocked atomic.Bool
}

type LockManagerMap struct {
	locks sync.Map
}

func NewLockManagerMap() LockManagerMap {
	return LockManagerMap{
		locks: sync.Map{},
	}
}

// lockMutex Locks a mutex for the given ID. The ref count will be
// incremented while the lock is in use by multiple threads to prevent
// it from being cleaned up prematurely
func (lmm *LockManagerMap) lockMutex(id string, timeout ...time.Duration) bool {
	const maxRetries = 3
	retryCount := 0

	for retryCount < maxRetries {
		value, _ := lmm.locks.LoadOrStore(id, &LockManager{})
		entry := value.(*LockManager)

		currentRef := entry.refCount.Add(1)

		// Check if the entry still exists in the map
		if currentRef <= 1 {
			if _, exists := lmm.locks.Load(id); !exists {
				// If the entry was deleted, retry
				entry.refCount.Add(-1)
				retryCount++
				continue
			}
		}

		// If a timeout is provided, return false and cleanup if that timeout is exceeded
		if len(timeout) > 0 {
			timer := time.NewTimer(timeout[0])
			defer timer.Stop()

			signalChan := make(chan struct{})
			go func() {
				entry.mutex.Lock()
				entry.isLocked.Store(true)
				close(signalChan)
			}()

			select {
			case <-signalChan:
				// Successfully acquired the lock
				return true
			case <-timer.C:
				// Timed out, decrement refCount and remove the lock from the map if it's no longer needed
				if entry.refCount.Add(-1) == 0 {
					lmm.locks.Delete(id)
				}
				return false
			}
		}

		entry.mutex.Lock()
		entry.isLocked.Store(true)
		return true
	}

	// Exceeded max retries
	return false
}

// unlockMutex Unlocks the mutex for a given id and deletes it if nobody else is waiting on it
func (lmm *LockManagerMap) unlockMutex(id string) {
	value, ok := lmm.locks.Load(id)
	if !ok {
		klog.Warningf("unlockMutex: LockManager not found for id: %s", id)
		return
	}
	entry := value.(*LockManager)

	// Decrease the reference count first
	newCount := entry.refCount.Add(-1)

	// Only unlock if it's currently locked
	if entry.isLocked.Swap(false) {
		entry.mutex.Unlock()
	}

	// Remove the lock from the map if needed
	if newCount == 0 {
		lmm.locks.Delete(id)
	}
}

// GetLockCount returns the current number of locks currently active in the map.
// For each active lock, the number of references for each lock ID is returned
func (lmm *LockManagerMap) GetLockCount() (int, map[string]int32) {
	count := 0
	refCounts := make(map[string]int32)

	lmm.locks.Range(func(key, value interface{}) bool {
		count++
		entry := value.(*LockManager)
		refCounts[key.(string)] = entry.refCount.Load()
		return true
	})

	return count, refCounts
}
