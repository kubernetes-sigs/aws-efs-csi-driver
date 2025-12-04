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

package cloud

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type mockEC2API struct {
	instances map[string]*types.Instance
	err       error
}

func (m *mockEC2API) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}

	var reservations []types.Reservation
	for _, instanceID := range params.InstanceIds {
		if instance, ok := m.instances[instanceID]; ok {
			reservations = append(reservations, types.Reservation{
				Instances: []types.Instance{*instance},
			})
		}
	}

	return &ec2.DescribeInstancesOutput{
		Reservations: reservations,
	}, nil
}

func (m *mockEC2API) DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	return nil, nil
}

func TestGetAvailableENIs(t *testing.T) {
	testCases := []struct {
		name              string
		instanceID        string
		instance          *types.Instance
		expectedENICount  int
		expectedError     bool
		expectedENIIPs    []string
		shouldContainIP   string
	}{
		{
			name:       "Single ENI",
			instanceID: "i-1234567890abcdef0",
			instance: &types.Instance{
				InstanceId: aws.String("i-1234567890abcdef0"),
				NetworkInterfaces: []types.InstanceNetworkInterface{
					{
						NetworkInterfaceId: aws.String("eni-12345678"),
						Status:             types.NetworkInterfaceStatusInUse,
						PrivateIpAddresses: []types.InstancePrivateIpAddress{
							{
								PrivateIpAddress: aws.String("10.0.1.10"),
								Primary:          aws.Bool(true),
							},
						},
						Attachment: &types.InstanceNetworkInterfaceAttachment{
							DeviceIndex: aws.Int32(0),
						},
						SubnetId:           aws.String("subnet-12345678"),
						AvailabilityZone:   aws.String("us-east-1a"),
						AvailabilityZoneId: aws.String("use1-az1"),
					},
				},
			},
			expectedENICount: 1,
			expectedError:    false,
			shouldContainIP:  "10.0.1.10",
		},
		{
			name:       "Multiple ENIs",
			instanceID: "i-1234567890abcdef0",
			instance: &types.Instance{
				InstanceId: aws.String("i-1234567890abcdef0"),
				NetworkInterfaces: []types.InstanceNetworkInterface{
					{
						NetworkInterfaceId: aws.String("eni-12345678"),
						Status:             types.NetworkInterfaceStatusInUse,
						PrivateIpAddresses: []types.InstancePrivateIpAddress{
							{
								PrivateIpAddress: aws.String("10.0.1.10"),
								Primary:          aws.Bool(true),
							},
						},
						Attachment: &types.InstanceNetworkInterfaceAttachment{
							DeviceIndex: aws.Int32(0),
						},
						SubnetId:           aws.String("subnet-12345678"),
						AvailabilityZone:   aws.String("us-east-1a"),
						AvailabilityZoneId: aws.String("use1-az1"),
					},
					{
						NetworkInterfaceId: aws.String("eni-87654321"),
						Status:             types.NetworkInterfaceStatusInUse,
						PrivateIpAddresses: []types.InstancePrivateIpAddress{
							{
								PrivateIpAddress: aws.String("10.0.2.10"),
								Primary:          aws.Bool(true),
							},
						},
						Attachment: &types.InstanceNetworkInterfaceAttachment{
							DeviceIndex: aws.Int32(1),
						},
						SubnetId:           aws.String("subnet-87654321"),
						AvailabilityZone:   aws.String("us-east-1a"),
						AvailabilityZoneId: aws.String("use1-az1"),
					},
				},
			},
			expectedENICount: 2,
			expectedError:    false,
			shouldContainIP:  "10.0.1.10",
		},
		{
			name:             "Empty instance ID",
			instanceID:       "",
			instance:         nil,
			expectedENICount: 0,
			expectedError:    true,
		},
		{
			name:       "No ENIs",
			instanceID: "i-1234567890abcdef0",
			instance: &types.Instance{
				InstanceId:        aws.String("i-1234567890abcdef0"),
				NetworkInterfaces: []types.InstanceNetworkInterface{},
			},
			expectedENICount: 0,
			expectedError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := &mockEC2API{
				instances: map[string]*types.Instance{
					"i-1234567890abcdef0": tc.instance,
				},
			}

			detector := NewENIDetector(mockAPI)
			enis, err := detector.GetAvailableENIs(context.Background(), tc.instanceID)

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(enis) != tc.expectedENICount {
				t.Errorf("Expected %d ENIs but got %d", tc.expectedENICount, len(enis))
			}

			if tc.shouldContainIP != "" {
				found := false
				for _, eni := range enis {
					if eni.PrivateIPv4 == tc.shouldContainIP {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find ENI with IP %s but didn't", tc.shouldContainIP)
				}
			}
		})
	}
}

func TestGetENIsByAZ(t *testing.T) {
	testCases := []struct {
		name             string
		instanceID       string
		azName           string
		instance         *types.Instance
		expectedENICount int
		expectedError    bool
	}{
		{
			name:       "Filter by AZ",
			instanceID: "i-1234567890abcdef0",
			azName:     "us-east-1a",
			instance: &types.Instance{
				InstanceId: aws.String("i-1234567890abcdef0"),
				NetworkInterfaces: []types.InstanceNetworkInterface{
					{
						NetworkInterfaceId: aws.String("eni-12345678"),
						Status:             types.NetworkInterfaceStatusInUse,
						PrivateIpAddresses: []types.InstancePrivateIpAddress{
							{
								PrivateIpAddress: aws.String("10.0.1.10"),
								Primary:          aws.Bool(true),
							},
						},
						Attachment: &types.InstanceNetworkInterfaceAttachment{
							DeviceIndex: aws.Int32(0),
						},
						SubnetId:           aws.String("subnet-12345678"),
						AvailabilityZone:   aws.String("us-east-1a"),
						AvailabilityZoneId: aws.String("use1-az1"),
					},
					{
						NetworkInterfaceId: aws.String("eni-87654321"),
						Status:             types.NetworkInterfaceStatusInUse,
						PrivateIpAddresses: []types.InstancePrivateIpAddress{
							{
								PrivateIpAddress: aws.String("10.0.2.10"),
								Primary:          aws.Bool(true),
							},
						},
						Attachment: &types.InstanceNetworkInterfaceAttachment{
							DeviceIndex: aws.Int32(1),
						},
						SubnetId:           aws.String("subnet-87654321"),
						AvailabilityZone:   aws.String("us-east-1b"),
						AvailabilityZoneId: aws.String("use1-az2"),
					},
				},
			},
			expectedENICount: 1,
			expectedError:    false,
		},
		{
			name:             "Empty AZ returns all",
			instanceID:       "i-1234567890abcdef0",
			azName:           "",
			expectedENICount: 2,
			expectedError:    false,
			instance: &types.Instance{
				InstanceId: aws.String("i-1234567890abcdef0"),
				NetworkInterfaces: []types.InstanceNetworkInterface{
					{
						NetworkInterfaceId: aws.String("eni-12345678"),
						Status:             types.NetworkInterfaceStatusInUse,
						PrivateIpAddresses: []types.InstancePrivateIpAddress{
							{
								PrivateIpAddress: aws.String("10.0.1.10"),
								Primary:          aws.Bool(true),
							},
						},
						Attachment: &types.InstanceNetworkInterfaceAttachment{
							DeviceIndex: aws.Int32(0),
						},
						SubnetId:           aws.String("subnet-12345678"),
						AvailabilityZone:   aws.String("us-east-1a"),
						AvailabilityZoneId: aws.String("use1-az1"),
					},
					{
						NetworkInterfaceId: aws.String("eni-87654321"),
						Status:             types.NetworkInterfaceStatusInUse,
						PrivateIpAddresses: []types.InstancePrivateIpAddress{
							{
								PrivateIpAddress: aws.String("10.0.2.10"),
								Primary:          aws.Bool(true),
							},
						},
						Attachment: &types.InstanceNetworkInterfaceAttachment{
							DeviceIndex: aws.Int32(1),
						},
						SubnetId:           aws.String("subnet-87654321"),
						AvailabilityZone:   aws.String("us-east-1b"),
						AvailabilityZoneId: aws.String("use1-az2"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := &mockEC2API{
				instances: map[string]*types.Instance{
					"i-1234567890abcdef0": tc.instance,
				},
			}

			detector := NewENIDetector(mockAPI)
			enis, err := detector.GetENIsByAZ(context.Background(), tc.instanceID, tc.azName)

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(enis) != tc.expectedENICount {
				t.Errorf("Expected %d ENIs but got %d", tc.expectedENICount, len(enis))
			}
		})
	}
}
