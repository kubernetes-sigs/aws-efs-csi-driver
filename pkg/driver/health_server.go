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
	"fmt"
	"net/http"
	"strconv"

	"k8s.io/klog/v2"
)

// StartHealthServer starts the enhanced HTTP health server
func (d *Driver) StartHealthServer(port int) error {
	addr := fmt.Sprintf(":%d", port)

	mux := http.NewServeMux()

	// Register health check endpoints
	if d.healthMonitor != nil {
		mux.HandleFunc("/healthz", d.healthMonitor.ServeHealthEndpoint)
		mux.HandleFunc("/healthz/ready", d.healthMonitor.ServeHealthEndpoint)
		mux.HandleFunc("/healthz/live", d.healthMonitor.ServeHealthEndpoint)
		mux.HandleFunc("/healthz/mounts", d.healthMonitor.ServeHealthEndpoint)
	} else {
		// Fallback to simple health checks if health monitor is not available
		mux.HandleFunc("/healthz", d.simpleHealthCheck)
		mux.HandleFunc("/healthz/ready", d.simpleHealthCheck)
		mux.HandleFunc("/healthz/live", d.simpleHealthCheck)
	}

	// Add metrics endpoint (basic)
	mux.HandleFunc("/metrics", d.metricsHandler)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	klog.Infof("Starting enhanced health server on %s", addr)
	return server.ListenAndServe()
}

// simpleHealthCheck provides a basic health check when health monitor is not available
func (d *Driver) simpleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// metricsHandler provides basic metrics information
func (d *Driver) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	var response string

	// Basic driver metrics
	response += "# HELP efs_csi_driver_info Driver information\n"
	response += "# TYPE efs_csi_driver_info gauge\n"
	response += fmt.Sprintf(`efs_csi_driver_info{version="%s",node_id="%s"} 1`+"\n", driverVersion, d.nodeID)

	// Mount health metrics
	if d.healthMonitor != nil {
		summary := d.healthMonitor.GetHealthSummary()

		response += "# HELP efs_csi_mount_health Mount health status (1=healthy, 0=unhealthy)\n"
		response += "# TYPE efs_csi_mount_health gauge\n"

		for path, health := range summary {
			healthValue := 0
			if health.IsHealthy {
				healthValue = 1
			}
			response += fmt.Sprintf(`efs_csi_mount_health{mount_path="%s"} %d`+"\n", path, healthValue)
		}

		response += "# HELP efs_csi_mount_response_time_seconds Mount health check response time\n"
		response += "# TYPE efs_csi_mount_response_time_seconds gauge\n"

		for path, health := range summary {
			responseTime := health.ResponseTime.Seconds()
			response += fmt.Sprintf(`efs_csi_mount_response_time_seconds{mount_path="%s"} %f`+"\n", path, responseTime)
		}

		// Overall health metric
		response += "# HELP efs_csi_overall_health Overall health status (1=healthy, 0=unhealthy)\n"
		response += "# TYPE efs_csi_overall_health gauge\n"
		overallHealth := 0
		if d.healthMonitor.GetOverallHealth() {
			overallHealth = 1
		}
		response += fmt.Sprintf("efs_csi_overall_health %d\n", overallHealth)
	}

	// Volume metrics if enabled
	if d.volMetricsOptIn {
		response += "# HELP efs_csi_volume_count Number of mounted volumes\n"
		response += "# TYPE efs_csi_volume_count gauge\n"
		response += fmt.Sprintf("efs_csi_volume_count %d\n", len(volumeIdCounter))

		for volumeId, count := range volumeIdCounter {
			response += fmt.Sprintf(`efs_csi_volume_mount_count{volume_id="%s"} %d`+"\n", volumeId, count)
		}
	}

	w.Write([]byte(response))
}

// StartHealthServerBackground starts the health server in a goroutine
func (d *Driver) StartHealthServerBackground(portStr string) {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		klog.Errorf("Invalid health port %s: %v", portStr, err)
		return
	}

	go func() {
		if err := d.StartHealthServer(port); err != nil {
			klog.Errorf("Health server failed: %v", err)
		}
	}()
}
