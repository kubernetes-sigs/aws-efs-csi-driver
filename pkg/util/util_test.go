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

package util

import (
	"reflect"
	"testing"
)

type TestRequest struct {
	Name    string
	Secrets map[string]string
}

func TestSanitizeRequest(t *testing.T) {
	tests := []struct {
		name     string
		req      interface{}
		expected interface{}
	}{
		{
			name: "Request with Secrets",
			req: &TestRequest{
				Name: "Test",
				Secrets: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			expected: &TestRequest{
				Name:    "Test",
				Secrets: map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeRequest(tt.req)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("SanitizeRequest() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
