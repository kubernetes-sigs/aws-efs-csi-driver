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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/volume/util/fs"
	"sync"
	"time"
)

type volMetrics struct {
	volPath   string
	timeStamp time.Time
	volUsage  []*csi.VolumeUsage
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
}

func NewVolStatter() VolStatter {
	return &VolStatterImpl{}
}

func (v VolStatterImpl) computeVolumeMetrics(volId, volPath string, refreshRate float64, fsRateLimit int) (*volMetrics, error) {
	if value, ok := v.retrieveFromCache(volId); ok {
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
		volPath:   volPath,
		timeStamp: time.Now(),
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
			volStatterJobTracker[volId] = true
			go v.computeDiskUsage(fsId, volId, volPath)
		} else {
			klog.V(5).Infof("Too many stat routines are running against FS : %s. Retry stat for volume Id: %s later", fsId, volId)
		}
	}
	mu.Unlock()
}

func (v VolStatterImpl) computeDiskUsage(fsId, volId, volPath string) {
	waitTime := wait.Jitter(jitter, 2.0)
	klog.V(5).Infof("Compute Volume Metrics invoked for Vol ID: %v, Sleeping for %v before execution", volId, waitTime)

	//jittered execution
	time.Sleep(waitTime)

	used, err := fs.DiskUsage(volPath)
	if err != nil {
		klog.Errorf("Failed to compute volume usage on path %s: %v", volPath, err)
		return
	}

	volUsed := used.Bytes

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

	volMetrics := &volMetrics{
		volPath:   volPath,
		timeStamp: time.Now(),
		volUsage:  usage}

	mu.Lock()
	volUsageCache[volId] = volMetrics
	delete(volStatterJobTracker, volId)
	if count, ok := fsRateLimiter[fsId]; ok && count > 0 {
		fsRateLimiter[fsId] = count - 1
	}
	mu.Unlock()
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
