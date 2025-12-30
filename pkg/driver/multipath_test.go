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
	"testing"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
)

func TestBuildMultipathMountOptions(t *testing.T) {
	testCases := []struct {
		name                string
		mountTargets        []*cloud.MountTarget
		expectError         bool
		minExpectedOptions  int
		shouldContainAddr   bool
		shouldContainOption string
	}{
		{
			name: "Single mount target",
			mountTargets: []*cloud.MountTarget{
				{
					AZName:        "us-east-1a",
					AZId:          "use1-az1",
					MountTargetId: "fsmt-12345678",
					IPAddress:     "10.0.1.10",
				},
			},
			expectError:         false,
			minExpectedOptions:  1,
			shouldContainAddr:   true,
			shouldContainOption: "addr=10.0.1.10",
		},
		{
			name: "Multiple mount targets",
			mountTargets: []*cloud.MountTarget{
				{
					AZName:        "us-east-1a",
					AZId:          "use1-az1",
					MountTargetId: "fsmt-12345678",
					IPAddress:     "10.0.1.10",
				},
				{
					AZName:        "us-east-1b",
					AZId:          "use1-az2",
					MountTargetId: "fsmt-87654321",
					IPAddress:     "10.0.2.10",
				},
			},
			expectError:         false,
			minExpectedOptions:  2,
			shouldContainAddr:   true,
			shouldContainOption: "addr=10.0.1.10",
		},
		{
			name:               "Empty mount targets",
			mountTargets:       []*cloud.MountTarget{},
			expectError:        true,
			minExpectedOptions: 0,
		},
		{
			name:               "Nil mount targets",
			mountTargets:       nil,
			expectError:        true,
			minExpectedOptions: 0,
		},
		{
			name: "Duplicate IP addresses",
			mountTargets: []*cloud.MountTarget{
				{
					AZName:        "us-east-1a",
					AZId:          "use1-az1",
					MountTargetId: "fsmt-12345678",
					IPAddress:     "10.0.1.10",
				},
				{
					AZName:        "us-east-1b",
					AZId:          "use1-az2",
					MountTargetId: "fsmt-87654321",
					IPAddress:     "10.0.1.10",
				},
			},
			expectError:         false,
			minExpectedOptions:  1,
			shouldContainAddr:   true,
			shouldContainOption: "addr=10.0.1.10",
		},
	}

	builder := NewMultipathBuilder()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options, err := builder.BuildMultipathMountOptions(tc.mountTargets)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(options) < tc.minExpectedOptions {
				t.Errorf("Expected at least %d options but got %d: %v", tc.minExpectedOptions, len(options), options)
			}

			if tc.shouldContainOption != "" {
				found := false
				for _, opt := range options {
					if opt == tc.shouldContainOption {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find option '%s' but didn't. Got: %v", tc.shouldContainOption, options)
				}
			}
		})
	}
}

func TestBuildSingleMountOptions(t *testing.T) {
	testCases := []struct {
		name                string
		mountTarget         *cloud.MountTarget
		expectError         bool
		shouldContainOption string
	}{
		{
			name: "Valid mount target",
			mountTarget: &cloud.MountTarget{
				AZName:        "us-east-1a",
				AZId:          "use1-az1",
				MountTargetId: "fsmt-12345678",
				IPAddress:     "10.0.1.10",
			},
			expectError:         false,
			shouldContainOption: "addr=10.0.1.10",
		},
		{
			name:                "Nil mount target",
			mountTarget:         nil,
			expectError:         true,
			shouldContainOption: "",
		},
	}

	builder := NewMultipathBuilder()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			options, err := builder.BuildSingleMountOptions(tc.mountTarget)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tc.shouldContainOption != "" {
				found := false
				for _, opt := range options {
					if opt == tc.shouldContainOption {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find option '%s' but didn't. Got: %v", tc.shouldContainOption, options)
				}
			}
		})
	}
}

func TestSelectOptimalMountTargets(t *testing.T) {
	testCases := []struct {
		name                    string
		mountTargets            []*cloud.MountTarget
		eniInfo                 []cloud.ENIInfo
		maxTargets              int
		expectedSelectedCount   int
		expectedPrimaryAZ       string
	}{
		{
			name: "Select from multiple mount targets with matching ENIs",
			mountTargets: []*cloud.MountTarget{
				{
					AZName:        "us-east-1a",
					AZId:          "use1-az1",
					MountTargetId: "fsmt-1",
					IPAddress:     "10.0.1.10",
				},
				{
					AZName:        "us-east-1b",
					AZId:          "use1-az2",
					MountTargetId: "fsmt-2",
					IPAddress:     "10.0.2.10",
				},
				{
					AZName:        "us-east-1a",
					AZId:          "use1-az1",
					MountTargetId: "fsmt-3",
					IPAddress:     "10.0.3.10",
				},
			},
			eniInfo: []cloud.ENIInfo{
				{
					ENIId:       "eni-1",
					PrivateIPv4: "10.1.1.5",
					AZName:      "us-east-1a",
					DeviceIndex: 0,
				},
				{
					ENIId:       "eni-2",
					PrivateIPv4: "10.1.2.5",
					AZName:      "us-east-1a",
					DeviceIndex: 1,
				},
				{
					ENIId:       "eni-3",
					PrivateIPv4: "10.1.3.5",
					AZName:      "us-east-1b",
					DeviceIndex: 2,
				},
			},
			maxTargets:            3,
			expectedSelectedCount: 3,
			expectedPrimaryAZ:     "us-east-1a",
		},
		{
			name: "Limit selection with maxTargets",
			mountTargets: []*cloud.MountTarget{
				{
					AZName:        "us-east-1a",
					AZId:          "use1-az1",
					MountTargetId: "fsmt-1",
					IPAddress:     "10.0.1.10",
				},
				{
					AZName:        "us-east-1b",
					AZId:          "use1-az2",
					MountTargetId: "fsmt-2",
					IPAddress:     "10.0.2.10",
				},
				{
					AZName:        "us-east-1c",
					AZId:          "use1-az3",
					MountTargetId: "fsmt-3",
					IPAddress:     "10.0.3.10",
				},
			},
			eniInfo: []cloud.ENIInfo{
				{
					ENIId:       "eni-1",
					PrivateIPv4: "10.1.1.5",
					AZName:      "us-east-1a",
					DeviceIndex: 0,
				},
			},
			maxTargets:            2,
			expectedSelectedCount: 2,
			expectedPrimaryAZ:     "us-east-1a",
		},
		{
			name:                  "Empty mount targets",
			mountTargets:          []*cloud.MountTarget{},
			eniInfo:               []cloud.ENIInfo{},
			maxTargets:            2,
			expectedSelectedCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selected := SelectOptimalMountTargets(tc.mountTargets, tc.eniInfo, tc.maxTargets)

			if len(selected) != tc.expectedSelectedCount {
				t.Errorf("Expected %d selected mount targets but got %d", tc.expectedSelectedCount, len(selected))
			}

			if tc.expectedPrimaryAZ != "" && len(selected) > 0 {
				if selected[0].AZName != tc.expectedPrimaryAZ {
					t.Errorf("Expected primary AZ to be %s but got %s", tc.expectedPrimaryAZ, selected[0].AZName)
				}
			}
		})
	}
}
