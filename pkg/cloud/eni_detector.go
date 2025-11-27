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
	"fmt"
	"net"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/klog/v2"
)

// ENIInfo represents network interface information for multipathing
type ENIInfo struct {
	ENIId       string
	PrivateIPv4 string
	SubnetId    string
	AZName      string
	AZId        string
	DeviceIndex int32
}

// ENIDetector provides methods to detect and manage ENIs for multipathing
type ENIDetector interface {
	GetAvailableENIs(ctx context.Context, instanceID string) ([]ENIInfo, error)
	GetENIsByAZ(ctx context.Context, instanceID string, azName string) ([]ENIInfo, error)
}

// EC2API defines the EC2 API methods needed for ENI detection
type EC2API interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
}

type eniDetector struct {
	ec2API EC2API
}

// NewENIDetector creates a new ENI detector
func NewENIDetector(ec2API EC2API) ENIDetector {
	return &eniDetector{
		ec2API: ec2API,
	}
}

// GetAvailableENIs retrieves all available ENIs for the given instance
func (e *eniDetector) GetAvailableENIs(ctx context.Context, instanceID string) ([]ENIInfo, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instanceID cannot be empty")
	}

	klog.V(5).Infof("Getting available ENIs for instance: %s", instanceID)

	// Describe the instance to get network interfaces
	describeInstancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	}

	result, err := e.ec2API.DescribeInstances(ctx, describeInstancesInput)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance %s: %v", instanceID, err)
	}

	if len(result.Reservations) == 0 || len(result.Reservations[0].Instances) == 0 {
		return nil, fmt.Errorf("instance %s not found", instanceID)
	}

	instance := result.Reservations[0].Instances[0]
	var eniInfos []ENIInfo

	// Extract network interface information from the instance
	for _, eni := range instance.NetworkInterfaces {
		if eni.Status == types.NetworkInterfaceStatusInUse {
			// Get primary private IPv4 address
			var primaryIPv4 string
			if len(eni.PrivateIpAddresses) > 0 {
				if eni.PrivateIpAddresses[0].PrivateIpAddress != nil {
					primaryIPv4 = *eni.PrivateIpAddresses[0].PrivateIpAddress
				}
			}

			// Skip ENIs without IPv4 addresses
			if primaryIPv4 == "" {
				klog.V(4).Infof("Skipping ENI %s: no IPv4 address", *eni.NetworkInterfaceId)
				continue
			}

			// Verify the address is not a loopback
			if net.ParseIP(primaryIPv4).IsLoopback() {
				klog.V(4).Infof("Skipping ENI %s: loopback address", *eni.NetworkInterfaceId)
				continue
			}

			azName := ""
			azId := ""
			if eni.AvailabilityZone != nil {
				azName = *eni.AvailabilityZone
			}
			if eni.AvailabilityZoneId != nil {
				azId = *eni.AvailabilityZoneId
			}

			subnetId := ""
			if eni.SubnetId != nil {
				subnetId = *eni.SubnetId
			}

			eniInfo := ENIInfo{
				ENIId:       *eni.NetworkInterfaceId,
				PrivateIPv4: primaryIPv4,
				SubnetId:    subnetId,
				AZName:      azName,
				AZId:        azId,
				DeviceIndex: *eni.Attachment.DeviceIndex,
			}

			eniInfos = append(eniInfos, eniInfo)
			klog.V(4).Infof("Found available ENI: %+v", eniInfo)
		}
	}

	if len(eniInfos) == 0 {
		klog.V(3).Infof("No available ENIs found for instance %s", instanceID)
		return nil, fmt.Errorf("no available ENIs found for instance %s", instanceID)
	}

	// Sort by device index for consistent ordering
	sort.Slice(eniInfos, func(i, j int) bool {
		return eniInfos[i].DeviceIndex < eniInfos[j].DeviceIndex
	})

	klog.V(4).Infof("Found %d available ENIs for instance %s", len(eniInfos), instanceID)
	return eniInfos, nil
}

// GetENIsByAZ retrieves ENIs in a specific availability zone
func (e *eniDetector) GetENIsByAZ(ctx context.Context, instanceID string, azName string) ([]ENIInfo, error) {
	allENIs, err := e.GetAvailableENIs(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if azName == "" {
		return allENIs, nil
	}

	var azENIs []ENIInfo
	for _, eni := range allENIs {
		if eni.AZName == azName {
			azENIs = append(azENIs, eni)
		}
	}

	if len(azENIs) == 0 {
		klog.V(3).Infof("No ENIs found in AZ %s for instance %s", azName, instanceID)
	}

	return azENIs, nil
}
