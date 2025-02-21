package driver

import (
	"sync"
    "sync/atomic"

    "k8s.io/klog/v2"
)

type LockManager struct {
    mutex    sync.Mutex
    refCount atomic.Int32
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
func (lmm *LockManagerMap) lockMutex(id string) {
    value, _ := lmm.locks.LoadOrStore(id, &LockManager{})
    entry := value.(*LockManager)

    entry.refCount.Add(1)
    entry.mutex.Lock()
}

// unlockMutex Unlocks the mutex for a given id and deletes it if nobody else is waiting on it
func (lmm *LockManagerMap) unlockMutex(id string) {
    value, ok := lmm.locks.Load(id)
    if !ok {
        klog.Warningf("unlockMutex: LockManager not found for id: %s", id)
        return
    }
    entry := value.(*LockManager)
    entry.mutex.Unlock()

    //If this was the last reference, remove the lock from the map
    if entry.refCount.Add(-1) == 0 {
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