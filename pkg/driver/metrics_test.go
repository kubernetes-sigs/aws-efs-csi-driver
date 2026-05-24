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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordOperationMetrics(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		err       error
		wantLabel string
	}{
		{
			name:      "success",
			operation: "CreateVolume",
			err:       nil,
			wantLabel: "success",
		},
		{
			name:      "error",
			operation: "DeleteVolume",
			err:       errors.New("some error"),
			wantLabel: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operationTotal.Reset()
			operationDuration.Reset()

			start := time.Now().Add(-100 * time.Millisecond)
			recordOperationMetrics(tt.operation, start, tt.err)

			counter := operationTotal.WithLabelValues(tt.operation, tt.wantLabel)
			if got := testutil.ToFloat64(counter); got != 1 {
				t.Errorf("expected counter to be 1, got %f", got)
			}
		})
	}
}
