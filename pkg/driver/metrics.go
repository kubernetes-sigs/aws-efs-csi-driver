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
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

const (
	metricsSubsystem = "efs_csi_controller"
)

var (
	operationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: metricsSubsystem,
			Name:      "operations_total",
			Help:      "Total number of CSI controller operations.",
		},
		[]string{"operation", "status"},
	)

	operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: metricsSubsystem,
			Name:      "operation_duration_seconds",
			Help:      "Latency of CSI controller operations in seconds.",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10),
		},
		[]string{"operation"},
	)
)

func init() {
	prometheus.MustRegister(operationTotal, operationDuration)
}

// recordOperationMetrics records the result and duration of a controller operation.
func recordOperationMetrics(operation string, start time.Time, err error) {
	duration := time.Since(start).Seconds()
	operationDuration.WithLabelValues(operation).Observe(duration)
	status := "success"
	if err != nil {
		status = "error"
	}
	operationTotal.WithLabelValues(operation, status).Inc()
}

// StartMetricsServer starts an HTTP server to expose Prometheus metrics.
func StartMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{Addr: addr, Handler: mux}
	klog.Infof("Starting metrics server on %s", addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			klog.Errorf("Metrics server failed: %v", err)
		}
	}()
}
