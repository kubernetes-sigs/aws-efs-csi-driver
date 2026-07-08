/*
Copyright 2024 The Kubernetes Authors.

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

package metrics

import (
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
	"k8s.io/klog/v2"
)

const (
	metricsRateLimit = 5  // requests per second
	metricsRateBurst = 10 // burst capacity
)

var (
	r    *MetricRecorder
	once sync.Once

	efsOperations = []string{
		"CreateAccessPoint",
		"DeleteAccessPoint",
		"DescribeAccessPoints",
		"DescribeFileSystems",
		"DescribeMountTargets",
	}
	s3FilesOperations = []string{
		"CreateAccessPoint",
		"DeleteAccessPoint",
		"ListAccessPoints",
		"ListFileSystems",
	}
)

type MetricRecorder struct {
	mu       sync.RWMutex
	registry *prometheus.Registry
	metrics  map[string]any
}

// Recorder returns the singleton MetricRecorder, or nil if not yet initialized.
func Recorder() *MetricRecorder {
	return r
}

// InitializeRecorder creates the singleton MetricRecorder and returns it.
func InitializeRecorder() *MetricRecorder {
	once.Do(func() {
		r = &MetricRecorder{
			registry: prometheus.NewRegistry(),
			metrics:  make(map[string]any),
		}
	})
	return r
}

// InitializeAPIMetrics pre-registers the duration histogram for all known EFS and S3Files
// operations so that zero-value time series exist before any requests are made.
func (m *MetricRecorder) InitializeAPIMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.metrics[APIRequestDuration]; !exists {
		metric := m.registerHistogramVecLocked(APIRequestDuration, APIRequestDurationHelpText, []string{"request", "service"}, nil)
		for _, op := range efsOperations {
			metric.WithLabelValues(op, "efs")
		}
		for _, op := range s3FilesOperations {
			metric.WithLabelValues(op, "s3files")
		}
	}
}

// InitializeMetricsHandler starts a rate-limited HTTP server exposing Prometheus metrics.
func (m *MetricRecorder) InitializeMetricsHandler(address, path, certFile, keyFile string) {
	if m == nil {
		klog.InfoS("InitializeMetricsHandler: metric recorder is not initialized")
		return
	}

	limiter := rate.NewLimiter(metricsRateLimit, metricsRateBurst)
	mux := http.NewServeMux()
	metricsHandler := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError})
	mux.Handle(path, rateLimitMiddleware(limiter, metricsHandler))

	server := &http.Server{
		Addr:         address,
		Handler:      mux,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		var err error
		klog.InfoS("Metric server listening", "address", address, "path", path)
		if certFile != "" {
			err = server.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			klog.ErrorS(err, "Failed to start metric server", "address", address, "path", path)
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	}()
}

// IncreaseCount increments the named counter metric by 1.
func (m *MetricRecorder) IncreaseCount(name, helpText string, labels map[string]string) {
	if m == nil {
		return
	}

	m.mu.RLock()
	metric, ok := m.metrics[name]
	m.mu.RUnlock()
	if !ok {
		m.mu.Lock()
		if _, exists := m.metrics[name]; !exists {
			klog.V(4).InfoS("Metric not found, registering", "name", name, "labels", labels)
			m.registerCounterVecLocked(name, helpText, getLabelNames(labels))
		}
		metric = m.metrics[name]
		m.mu.Unlock()
	}

	if cv, ok := metric.(*prometheus.CounterVec); ok {
		cv.With(labels).Inc()
	} else {
		klog.V(4).InfoS("Could not assert metric as CounterVec", "name", name)
	}
}

// ObserveHistogram records value in the named histogram metric.
func (m *MetricRecorder) ObserveHistogram(name, helpText string, value float64, labels map[string]string, buckets []float64) {
	if m == nil {
		return
	}

	m.mu.RLock()
	metric, ok := m.metrics[name]
	m.mu.RUnlock()
	if !ok {
		m.mu.Lock()
		if _, exists := m.metrics[name]; !exists {
			klog.V(4).InfoS("Metric not found, registering", "name", name, "labels", labels)
			m.registerHistogramVecLocked(name, helpText, getLabelNames(labels), buckets)
		}
		metric = m.metrics[name]
		m.mu.Unlock()
	}

	if hv, ok := metric.(*prometheus.HistogramVec); ok {
		hv.With(labels).Observe(value)
	} else {
		klog.V(4).InfoS("Could not assert metric as HistogramVec", "name", name)
	}
}

func rateLimitMiddleware(limiter *rate.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// registerHistogramVecLocked registers a HistogramVec. Callers must hold m.mu.
func (m *MetricRecorder) registerHistogramVecLocked(name, help string, labels []string, buckets []float64) *prometheus.HistogramVec {
	if metric, exists := m.metrics[name]; exists {
		if hv, ok := metric.(*prometheus.HistogramVec); ok {
			return hv
		}
		klog.ErrorS(nil, "Metric exists but is not a HistogramVec", "name", name)
		return nil
	}
	hv := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: help, Buckets: buckets}, labels)
	m.metrics[name] = hv
	m.registry.MustRegister(hv)
	return hv
}

// registerCounterVecLocked registers a CounterVec. Callers must hold m.mu.
func (m *MetricRecorder) registerCounterVecLocked(name, help string, labels []string) {
	if _, exists := m.metrics[name]; exists {
		return
	}
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	m.metrics[name] = cv
	m.registry.MustRegister(cv)
}

func getLabelNames(labels map[string]string) []string {
	names := make([]string, 0, len(labels))
	for k := range labels {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
