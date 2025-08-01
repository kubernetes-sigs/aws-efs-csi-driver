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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

func TestHealthMonitor_RegisterMount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	mounter := mocks.NewMockMounter(ctrl)
	hm := NewHealthMonitor(mounter)
	
	testPath := "/test/mount/path"
	hm.RegisterMount(testPath)
	
	health, exists := hm.GetMountHealth(testPath)
	if !exists {
		t.Errorf("Mount should be registered")
	}
	
	if health.MountPath != testPath {
		t.Errorf("Expected mount path %s, got %s", testPath, health.MountPath)
	}
	
	if !health.IsHealthy {
		t.Errorf("Mount should be initially healthy")
	}
}

func TestHealthMonitor_UnregisterMount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	mounter := mocks.NewMockMounter(ctrl)
	hm := NewHealthMonitor(mounter)
	
	testPath := "/test/mount/path"
	hm.RegisterMount(testPath)
	hm.UnregisterMount(testPath)
	
	_, exists := hm.GetMountHealth(testPath)
	if exists {
		t.Errorf("Mount should be unregistered")
	}
}

func TestHealthMonitor_GetOverallHealth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	mounter := mocks.NewMockMounter(ctrl)
	hm := NewHealthMonitor(mounter)
	
	// No mounts - should be healthy
	if !hm.GetOverallHealth() {
		t.Errorf("Overall health should be true when no mounts are registered")
	}
	
	// Register a healthy mount
	testPath := "/test/mount/path"
	hm.RegisterMount(testPath)
	
	if !hm.GetOverallHealth() {
		t.Errorf("Overall health should be true with healthy mounts")
	}
}

func TestHealthMonitor_HTTPEndpoints(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	mounter := mocks.NewMockMounter(ctrl)
	hm := NewHealthMonitor(mounter)
	
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "efs-health-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create a test mount point
	testMount := filepath.Join(tempDir, "testmount")
	if err := os.MkdirAll(testMount, 0755); err != nil {
		t.Fatalf("Failed to create test mount: %v", err)
	}
	
	// Set up mock expectations
	mounter.EXPECT().IsMountPoint(testMount).Return(true, nil).AnyTimes()
	
	hm.RegisterMount(testMount)
	
	testCases := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "overall health",
			path:           "/healthz",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "readiness check",
			path:           "/healthz/ready",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "liveness check",
			path:           "/healthz/live",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "mount health details",
			path:           "/healthz/mounts",
			expectedStatus: http.StatusOK,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tc.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			
			rr := httptest.NewRecorder()
			hm.ServeHealthEndpoint(rr, req)
			
			if rr.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, rr.Code)
			}
		})
	}
}

func TestHealthMonitor_MountHealthDetails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	mounter := mocks.NewMockMounter(ctrl)
	hm := NewHealthMonitor(mounter)
	
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "efs-health-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create a test mount point
	testMount := filepath.Join(tempDir, "testmount")
	if err := os.MkdirAll(testMount, 0755); err != nil {
		t.Fatalf("Failed to create test mount: %v", err)
	}
	
	// Set up mock expectations
	mounter.EXPECT().IsMountPoint(testMount).Return(true, nil).AnyTimes()
	
	hm.RegisterMount(testMount)
	
	req, err := http.NewRequest("GET", "/healthz/mounts", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	rr := httptest.NewRecorder()
	hm.ServeHealthEndpoint(rr, req)
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}
	
	body := rr.Body.String()
	if !strings.Contains(body, testMount) {
		t.Errorf("Response should contain mount path %s, got: %s", testMount, body)
	}
	
	if !strings.Contains(body, "healthy") {
		t.Errorf("Response should indicate healthy status, got: %s", body)
	}
}

func TestHealthMonitor_StartStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	mounter := mocks.NewMockMounter(ctrl)
	hm := NewHealthMonitor(mounter)
	
	// Start health monitor
	hm.Start()
	
	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)
	
	// Stop health monitor
	hm.Stop()
	
	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)
	
	// Test passes if no panics or deadlocks occur
}

func TestHealthMonitor_HealthSummary(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	
	mounter := mocks.NewMockMounter(ctrl)
	hm := NewHealthMonitor(mounter)
	
	// Register multiple mounts
	testPaths := []string{"/mount1", "/mount2", "/mount3"}
	for _, path := range testPaths {
		hm.RegisterMount(path)
	}
	
	summary := hm.GetHealthSummary()
	
	if len(summary) != len(testPaths) {
		t.Errorf("Expected %d mounts in summary, got %d", len(testPaths), len(summary))
	}
	
	for _, path := range testPaths {
		if health, exists := summary[path]; !exists {
			t.Errorf("Mount %s should be in summary", path)
		} else if !health.IsHealthy {
			t.Errorf("Mount %s should be healthy", path)
		}
	}
}
