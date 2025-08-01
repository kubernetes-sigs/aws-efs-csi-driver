/*
Copyright 2025 The Kubernetes Authors.

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
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
)

// MountHealth represents the health status of an EFS mount
type MountHealth struct {
	MountPath   string    `json:"mountPath"`
	IsHealthy   bool      `json:"isHealthy"`
	LastChecked time.Time `json:"lastChecked"`
	Error       string    `json:"error,omitempty"`
	ResponseTime time.Duration `json:"responseTime"`
}

// HealthMonitor monitors the health of EFS mounts and provides enhanced health checking
type HealthMonitor struct {
	mounter         mount.Interface
	mountHealthMap  map[string]*MountHealth
	mutex           sync.RWMutex
	checkInterval   time.Duration
	timeout         time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewHealthMonitor creates a new health monitor instance
func NewHealthMonitor(mounter mount.Interface) *HealthMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &HealthMonitor{
		mounter:        mounter,
		mountHealthMap: make(map[string]*MountHealth),
		checkInterval:  30 * time.Second, // Check every 30 seconds
		timeout:        5 * time.Second,  // 5 second timeout for each check
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins the health monitoring routine
func (hm *HealthMonitor) Start() {
	go hm.monitorLoop()
	klog.V(2).Info("EFS health monitor started")
}

// Stop terminates the health monitoring routine
func (hm *HealthMonitor) Stop() {
	hm.cancel()
	klog.V(2).Info("EFS health monitor stopped")
}

// RegisterMount adds a mount point to be monitored
func (hm *HealthMonitor) RegisterMount(mountPath string) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	
	if _, exists := hm.mountHealthMap[mountPath]; !exists {
		hm.mountHealthMap[mountPath] = &MountHealth{
			MountPath:   mountPath,
			IsHealthy:   true,
			LastChecked: time.Now(),
		}
		klog.V(4).Infof("Registered mount %s for health monitoring", mountPath)
	}
}

// UnregisterMount removes a mount point from monitoring
func (hm *HealthMonitor) UnregisterMount(mountPath string) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	
	delete(hm.mountHealthMap, mountPath)
	klog.V(4).Infof("Unregistered mount %s from health monitoring", mountPath)
}

// GetMountHealth returns the health status of a specific mount
func (hm *HealthMonitor) GetMountHealth(mountPath string) (*MountHealth, bool) {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()
	
	health, exists := hm.mountHealthMap[mountPath]
	if !exists {
		return nil, false
	}
	
	// Return a copy to avoid race conditions
	return &MountHealth{
		MountPath:    health.MountPath,
		IsHealthy:    health.IsHealthy,
		LastChecked:  health.LastChecked,
		Error:        health.Error,
		ResponseTime: health.ResponseTime,
	}, true
}

// GetOverallHealth returns the overall health status of all monitored mounts
func (hm *HealthMonitor) GetOverallHealth() bool {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()
	
	for _, health := range hm.mountHealthMap {
		if !health.IsHealthy {
			return false
		}
	}
	return true
}

// GetHealthSummary returns a summary of all mount health statuses
func (hm *HealthMonitor) GetHealthSummary() map[string]*MountHealth {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()
	
	summary := make(map[string]*MountHealth)
	for path, health := range hm.mountHealthMap {
		summary[path] = &MountHealth{
			MountPath:    health.MountPath,
			IsHealthy:    health.IsHealthy,
			LastChecked:  health.LastChecked,
			Error:        health.Error,
			ResponseTime: health.ResponseTime,
		}
	}
	return summary
}

// monitorLoop is the main monitoring loop that runs in a goroutine
func (hm *HealthMonitor) monitorLoop() {
	ticker := time.NewTicker(hm.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-hm.ctx.Done():
			return
		case <-ticker.C:
			hm.checkAllMounts()
		}
	}
}

// checkAllMounts performs health checks on all registered mounts
func (hm *HealthMonitor) checkAllMounts() {
	hm.mutex.RLock()
	mountPaths := make([]string, 0, len(hm.mountHealthMap))
	for path := range hm.mountHealthMap {
		mountPaths = append(mountPaths, path)
	}
	hm.mutex.RUnlock()
	
	for _, mountPath := range mountPaths {
		hm.checkMountHealth(mountPath)
	}
}

// checkMountHealth performs a health check on a specific mount
func (hm *HealthMonitor) checkMountHealth(mountPath string) {
	start := time.Now()
	
	ctx, cancel := context.WithTimeout(hm.ctx, hm.timeout)
	defer cancel()
	
	isHealthy, err := hm.performHealthCheck(ctx, mountPath)
	responseTime := time.Since(start)
	
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	
	if health, exists := hm.mountHealthMap[mountPath]; exists {
		health.IsHealthy = isHealthy
		health.LastChecked = time.Now()
		health.ResponseTime = responseTime
		
		if err != nil {
			health.Error = err.Error()
			klog.V(4).Infof("Mount %s health check failed: %v (took %v)", mountPath, err, responseTime)
		} else {
			health.Error = ""
			klog.V(6).Infof("Mount %s health check passed (took %v)", mountPath, responseTime)
		}
	}
}

// performHealthCheck executes the actual health check operations
func (hm *HealthMonitor) performHealthCheck(ctx context.Context, mountPath string) (bool, error) {
	// Check if mount point exists
	if _, err := os.Stat(mountPath); err != nil {
		return false, fmt.Errorf("mount point does not exist: %v", err)
	}
	
	// Check if it's actually mounted
	isMounted, err := hm.mounter.IsMountPoint(mountPath)
	if err != nil {
		return false, fmt.Errorf("failed to check mount status: %v", err)
	}
	
	if !isMounted {
		return false, fmt.Errorf("path is not mounted")
	}
	
	// Perform a simple read test with timeout
	testFile := filepath.Join(mountPath, ".efs_health_check")
	
	// Try to create a test file (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- hm.performIOTest(testFile)
	}()
	
	select {
	case err := <-done:
		return err == nil, err
	case <-ctx.Done():
		return false, fmt.Errorf("health check timed out")
	}
}

// performIOTest performs a simple I/O test on the mount
func (hm *HealthMonitor) performIOTest(testFile string) error {
	// Write test
	testData := []byte(fmt.Sprintf("health-check-%d", time.Now().Unix()))
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		return fmt.Errorf("write test failed: %v", err)
	}
	
	// Read test
	readData, err := os.ReadFile(testFile)
	if err != nil {
		os.Remove(testFile) // Clean up
		return fmt.Errorf("read test failed: %v", err)
	}
	
	// Verify data
	if string(readData) != string(testData) {
		os.Remove(testFile) // Clean up
		return fmt.Errorf("data integrity check failed")
	}
	
	// Clean up
	if err := os.Remove(testFile); err != nil {
		klog.V(4).Infof("Failed to clean up test file %s: %v", testFile, err)
	}
	
	return nil
}

// ServeHealthEndpoint serves HTTP health check requests
func (hm *HealthMonitor) ServeHealthEndpoint(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		hm.handleOverallHealth(w, r)
	case "/healthz/ready":
		hm.handleReadinessCheck(w, r)
	case "/healthz/live":
		hm.handleLivenessCheck(w, r)
	case "/healthz/mounts":
		hm.handleMountHealth(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleOverallHealth handles the overall health check endpoint
func (hm *HealthMonitor) handleOverallHealth(w http.ResponseWriter, r *http.Request) {
	if hm.GetOverallHealth() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("unhealthy mounts detected"))
	}
}

// handleReadinessCheck handles readiness probe requests
func (hm *HealthMonitor) handleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	// For readiness, we're more strict - all mounts must be healthy
	if hm.GetOverallHealth() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready - unhealthy mounts"))
	}
}

// handleLivenessCheck handles liveness probe requests  
func (hm *HealthMonitor) handleLivenessCheck(w http.ResponseWriter, r *http.Request) {
	// For liveness, we're more lenient - driver is alive if it can respond
	// Individual mount failures shouldn't kill the driver
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("alive"))
}

// handleMountHealth provides detailed mount health information
func (hm *HealthMonitor) handleMountHealth(w http.ResponseWriter, r *http.Request) {
	summary := hm.GetHealthSummary()
	
	w.Header().Set("Content-Type", "application/json")
	
	var response strings.Builder
	response.WriteString("{\"mounts\":{")
	
	first := true
	for path, health := range summary {
		if !first {
			response.WriteString(",")
		}
		first = false
		
		status := "healthy"
		if !health.IsHealthy {
			status = "unhealthy"
		}
		
		response.WriteString(fmt.Sprintf(`"%s":{"status":"%s","lastChecked":"%s","responseTime":"%s"`,
			path, status, health.LastChecked.Format(time.RFC3339), health.ResponseTime))
		
		if health.Error != "" {
			response.WriteString(fmt.Sprintf(`,"error":"%s"`, health.Error))
		}
		
		response.WriteString("}")
	}
	
	response.WriteString("}}")
	
	if hm.GetOverallHealth() {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	w.Write([]byte(response.String()))
}
