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

package cloud

import (
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud/mocks"
)

func TestGetMetadataProvider(t *testing.T) {
	testCases := []struct {
		name                          string
		expectedType                  string
		isRunningInECS                bool
		isEC2MetadataServiceAvailable bool
	}{
		{
			name:           "success: should use ECS first if booted in ECS",
			expectedType:   "taskMetadataProvider",
			isRunningInECS: true,
		},
		{
			name:                          "success: should use ECS first if booted in ECS, even if other services are available",
			expectedType:                  "taskMetadataProvider",
			isRunningInECS:                true,
			isEC2MetadataServiceAvailable: true,
		},
		{
			name:                          "success: should use EC2 if not in ECS and EC2 is available",
			expectedType:                  "ec2MetadataProvider",
			isRunningInECS:                false,
			isEC2MetadataServiceAvailable: true,
		},
		{
			name:                          "success: should use Kubernetes if not in ECS and EC2 is unavailable",
			expectedType:                  "kubernetesApiMetadataProvider",
			isRunningInECS:                false,
			isEC2MetadataServiceAvailable: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockEC2Metadata := mocks.NewMockEC2Metadata(mockCtrl)

			if tc.isRunningInECS {
				envVars := map[string]string{taskMetadataV4EnvName: "foobar"}
				for k, v := range envVars {
					t.Setenv(k, v)
				}
			}

			if !tc.isRunningInECS {
				mockEC2Metadata.EXPECT().Available().Return(tc.isEC2MetadataServiceAvailable)
			}

			defer mockCtrl.Finish()

			mp, _ := GetNewMetadataProvider(mockEC2Metadata, fake.NewSimpleClientset())

			providerType := reflect.TypeOf(mp).Name()

			if providerType != tc.expectedType {
				t.Errorf("Expected %s, but got %s", tc.expectedType, providerType)
			}
		})
	}
}
