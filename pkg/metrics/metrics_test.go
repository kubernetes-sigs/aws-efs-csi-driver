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
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestRecorder() *MetricRecorder {
	return &MetricRecorder{
		registry: prometheus.NewRegistry(),
		metrics:  make(map[string]any),
	}
}

func TestMetricRecorder(t *testing.T) {
	tests := []struct {
		name     string
		exec     func(m *MetricRecorder)
		expected string
	}{
		{
			name: "IncreaseCounterMetric",
			exec: func(m *MetricRecorder) {
				m.IncreaseCount("test_total", "help text", map[string]string{"key": "value"})
			},
			expected: `
# HELP test_total help text
# TYPE test_total counter
test_total{key="value"} 1
			`,
		},
		{
			name: "ObserveHistogramMetric",
			exec: func(m *MetricRecorder) {
				m.ObserveHistogram("test", "help text", 1.5, map[string]string{"key": "value"}, []float64{1, 2, 3})
			},
			expected: `
# HELP test help text
# TYPE test histogram
test_bucket{key="value",le="1"} 0
test_bucket{key="value",le="2"} 1
test_bucket{key="value",le="3"} 1
test_bucket{key="value",le="+Inf"} 1
test_sum{key="value"} 1.5
test_count{key="value"} 1
			`,
		},
		{
			name: "Re-register metric",
			exec: func(m *MetricRecorder) {
				m.IncreaseCount("test_re_register_total", "help text", map[string]string{"key": "value1"})
				m.mu.Lock()
				m.registerCounterVecLocked("test_re_register_total", "help text", []string{"key"})
				m.mu.Unlock()
				m.IncreaseCount("test_re_register_total", "help text", map[string]string{"key": "value1"})
				m.IncreaseCount("test_re_register_total", "help text", map[string]string{"key": "value2"})
			},
			expected: `
# HELP test_re_register_total help text
# TYPE test_re_register_total counter
test_re_register_total{key="value1"} 2
test_re_register_total{key="value2"} 1
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestRecorder()
			tt.exec(m)
			if err := testutil.GatherAndCompare(m.registry, strings.NewReader(tt.expected), getMetricNameFromExpected(tt.expected)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNilRecorderIsNoop(t *testing.T) {
	var rec *MetricRecorder
	rec.IncreaseCount("anything", "help", map[string]string{"k": "v"})
	rec.ObserveHistogram("anything", "help", 1.0, map[string]string{"k": "v"}, nil)
}

func TestRecorderSingleton(t *testing.T) {
	once = sync.Once{}
	r = nil

	r1 := InitializeRecorder()
	r2 := InitializeRecorder()

	if r1 != r2 {
		t.Error("InitializeRecorder must return the same singleton on repeated calls")
	}
}

func getMetricNameFromExpected(expected string) string {
	for line := range strings.SplitSeq(expected, "\n") {
		if strings.Contains(line, "{") {
			return strings.Split(line, "{")[0]
		}
	}
	return ""
}
