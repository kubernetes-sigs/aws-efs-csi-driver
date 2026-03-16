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
	"context"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume/util/fs"
)

type volMetrics struct {
	mountHealthy bool
	volPath      string
	timeStamp    time.Time
	volUsage     []*csi.VolumeUsage
}

type MountHealth interface {
	AsyncMountHealthRecovery(volId, volPath string)
}

var (
	volUsageCache        = make(map[string]*volMetrics)
	volStatterJobTracker = make(map[string]bool)
	fsRateLimiter        = make(map[string]int)
	mu                   sync.RWMutex
	jitter               = time.Duration(5 * time.Minute)
)

type VolStatter interface {
	computeVolumeMetrics(volId, volPath string, refreshRate float64, fsRateLimit int) (*volMetrics, error)
	retrieveFromCache(volId string) (*volMetrics, bool)
	removeFromCache(volId string)
}

type VolStatterImpl struct {
	healthWatchdog MountHealth
}

func (v *VolStatterImpl) SethealthDriver(r MountHealth) {
	mu.Lock()
	defer mu.Unlock()
	v.healthWatchdog = r
}

func NewVolStatter() VolStatter {
	return &VolStatterImpl{}
}

func (v VolStatterImpl) computeVolumeMetrics(volId, volPath string, refreshRate float64, fsRateLimit int) (*volMetrics, error) {
	if value, ok := v.retrieveFromCache(volId); ok {
		if _, ok := volStatterJobTracker[volId]; ok {
			klog.V(4).Infof("Volume metrics computation is underway for volume Id : %v. Returning cached metrics for now", volId)
			return value, nil
		}
		if time.Since(value.timeStamp).Minutes() > refreshRate {
			// Time to refresh volume stats
			v.launchVolStatsRoutine(volId, volPath, fsRateLimit)
		}
		return value, nil
	} else {
		klog.V(4).Infof("Did not find volume metrics in cache for vol ID: %v , vol path: %v. Computing now!", volId, volPath)
	}

	v.launchVolStatsRoutine(volId, volPath, fsRateLimit)

	// Return nil as kubelet might timeout waiting for volume stats
	klog.Warningf("Volume metrics computation is underway for Vol ID: %v and metrics are not available yet.", volId)
	return &volMetrics{
		mountHealthy: false,
		volPath:      volPath,
		timeStamp:    time.Now(),
		volUsage: []*csi.VolumeUsage{
			{
				Unit: csi.VolumeUsage_UNKNOWN,
			},
		},
	}, nil
}

func (v VolStatterImpl) retrieveFromCache(volId string) (*volMetrics, bool) {
	mu.RLock()
	defer mu.RUnlock()
	if value, ok := volUsageCache[volId]; ok {
		return value, true
	} else {
		return nil, false
	}
}

func (v VolStatterImpl) removeFromCache(volId string) {
	mu.Lock()
	delete(volUsageCache, volId)
	mu.Unlock()
}

func (v VolStatterImpl) launchVolStatsRoutine(volId, volPath string, fsRateLimit int) {
	fsId, _, _, err := parseVolumeId(volId)
	if err != nil {
		klog.Errorf("Failed to launch Stat routine: Could not parse File System ID from volume Id - %s.", volId)
		return
	}

	mu.Lock()
	if _, ok := volStatterJobTracker[volId]; ok {
		klog.V(5).Infof("Volume stats computation job is underway for volume Id : %v. Awaiting results", volId)
	} else {
		if ok := canStatFS(fsId, fsRateLimit); ok {
			go v.computeDiskUsage(fsId, volId, volPath)
		} else {
			klog.V(5).Infof("Too many stat routines are running against FS : %s. Retry stat for volume Id: %s later", fsId, volId)
		}
	}
	mu.Unlock()
}

func (v VolStatterImpl) computeDiskUsage(fsId, volId, volPath string) {
	waitTime := wait.Jitter(jitter, 2.0) //reduce jitter with better thundering herd management
	var used fs.UsageInfo
	var err error
	update_time := time.Now().UTC()
	health := false
	done := make(chan error, 1)
	klog.V(5).Infof("Compute volume metrics invoked for Vol ID: %v, Sleeping for %v before execution", volId, waitTime)

	volMetrics := &volMetrics{
		mountHealthy: health,
		volPath:      volPath,
		timeStamp:    update_time,
		volUsage:     []*csi.VolumeUsage{{Unit: csi.VolumeUsage_UNKNOWN}},
	}
	if _, ok := v.retrieveFromCache(volId); !ok {
		volUsageCache[volId] = volMetrics
	}

	// ensure fsRateLimiter cleanup for failing execution paths
	// ToDo: fix kubelet thrundering herd with empty cahce response pending first await volStatter execution completion
	defer func() {
		mu.Lock()
		volUsageCache[volId] = volMetrics
		delete(volStatterJobTracker, volId)
		if count, ok := fsRateLimiter[fsId]; ok && count > 0 {
			fsRateLimiter[fsId] = count - 1
		}
		mu.Unlock()
	}()

	//jittered execution
	time.Sleep(waitTime)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	health, err = checkMountHealth(ctx, volPath)
	if !health {
		updateVolMetricsWithHealth(volId, health)
		// attempt a single async recovery to update cached health status back to healthy
		go v.healthWatchdog.AsyncMountHealthRecovery(volId, volPath)
		klog.Errorf("Failed mount health check for volume path %s: %v", volPath, err)
		return
	}
	// close go routines on dead/inactive mount point that may hang indefinitely
	go func() {
		used, err = fs.DiskUsage(volPath)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			health = false
			updateVolMetricsWithHealth(volId, health)
			klog.Errorf("Failed to compute volume usage on path %s: %v", volPath, err)
			return
		}
	case <-ctx.Done():
		health = false
		updateVolMetricsWithHealth(volId, health)
		klog.Errorf("Volume usage computation timed out for path %s", volPath)
		return
	}

	volUsed := used.Bytes

	// this Info call can also enter a Ss if the mount point is dead,
	// so we need should ordinarily put a timeout on it as well
	available, capacity, _, _, _, _, err := fs.Info(volPath)
	if err != nil {
		klog.Errorf("Failed to fetch FsInfo on volume path %s: %v", volPath, err)
		return
	}

	usage := []*csi.VolumeUsage{
		{
			Unit:      csi.VolumeUsage_BYTES,
			Used:      volUsed,
			Available: available,
			Total:     capacity,
		},
	}

	volMetrics.mountHealthy = health
	volMetrics.volUsage = usage
	update_time = time.Now().UTC()
	volMetrics.timeStamp = update_time
	klog.V(5).Infof("Compute volume metrics complete for Vol ID: %v, Vol Health: %v, Used: %v, Available: %v, Capacity: %v", volId, health, volUsed, available, capacity)
	return
}

func canStatFS(fsId string, fsRateLimit int) bool {
	if count, ok := fsRateLimiter[fsId]; ok {
		if count < fsRateLimit {
			fsRateLimiter[fsId] = count + 1
		} else {
			return false
		}
	} else {
		fsRateLimiter[fsId] = 1
	}

	return true
}

func updateVolMetricsWithHealth(volId string, health bool) {
	mu.Lock()
	defer mu.Unlock()
	if value, ok := volUsageCache[volId]; ok {
		value.mountHealthy = health
		value.timeStamp = time.Now()
	}
}
