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
	"strings"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"k8s.io/klog/v2"
)

const (
	// MultipathingEnabled is the volume context key to enable multipathing
	MultipathingEnabled = "multipathing"
	// MultipathAddresses is used internally to pass multiple addresses
	MultipathAddresses = "multipath_addresses"
)

// MultipathBuilder provides methods for building multipath mount options
type MultipathBuilder interface {
	// BuildMultipathMountOptions creates mount options for multipathing across multiple mount targets
	BuildMultipathMountOptions(mountTargets []*cloud.MountTarget) ([]string, error)
	// BuildSingleMountOptions creates mount options for a single mount target
	BuildSingleMountOptions(mountTarget *cloud.MountTarget) ([]string, error)
}

type multipathBuilder struct{}

// NewMultipathBuilder creates a new multipath mount options builder
func NewMultipathBuilder() MultipathBuilder {
	return &multipathBuilder{}
}

// BuildMultipathMountOptions generates mount options that enable multipathing across multiple mount targets
// This implements session trunking by using NFS mount options that bind to multiple network addresses
func (m *multipathBuilder) BuildMultipathMountOptions(mountTargets []*cloud.MountTarget) ([]string, error) {
	if len(mountTargets) == 0 {
		return nil, fmt.Errorf("no mount targets provided for multipath mount options")
	}

	klog.V(4).Infof("Building multipath mount options for %d mount targets", len(mountTargets))

	// If only one mount target, no multipathing needed
	if len(mountTargets) == 1 {
		klog.V(4).Infof("Only one mount target provided, using single path mount")
		return m.BuildSingleMountOptions(mountTargets[0])
	}

	// For multipathing, we can use various approaches:
	// 1. NFS4 session trunking (if supported): multiple addr option values
	// 2. Linux NFS bonding through mount options
	// 3. Multiple mount targets with addr specification

	mountOptions := []string{}

	// Extract unique IP addresses from mount targets
	var ipAddresses []string
	seen := make(map[string]bool)

	for _, mt := range mountTargets {
		if !seen[mt.IPAddress] {
			ipAddresses = append(ipAddresses, mt.IPAddress)
			seen[mt.IPAddress] = true
			klog.V(4).Infof("Adding mount target IP %s (AZ: %s) to multipath", mt.IPAddress, mt.AZName)
		}
	}

	if len(ipAddresses) <= 1 {
		// All mount targets have the same IP, fall back to single target
		klog.V(4).Infof("All mount targets resolve to same IP, using single path")
		return m.BuildSingleMountOptions(mountTargets[0])
	}

	// Build comma-separated list of addresses for NFSv4 session trunking
	// This tells the NFS client to establish connections to multiple addresses
	addressList := strings.Join(ipAddresses, ",")
	klog.V(4).Infof("Multipath address list: %s", addressList)

	// Store the address list for use in mount options
	// Standard NFS mount options for multipathing
	mountOptions = append(mountOptions, fmt.Sprintf("addr=%s", ipAddresses[0]))

	// Add additional addresses as mount options (this supports some NFS implementations)
	// Note: The primary address is in the mount source (fsid:subpath), additional ones go in options
	for i := 1; i < len(ipAddresses); i++ {
		mountOptions = append(mountOptions, fmt.Sprintf("multipath_addr%d=%s", i, ipAddresses[i]))
	}

	klog.V(4).Infof("Generated multipath mount options: %v", mountOptions)
	return mountOptions, nil
}

// BuildSingleMountOptions creates mount options for a single mount target
func (m *multipathBuilder) BuildSingleMountOptions(mountTarget *cloud.MountTarget) ([]string, error) {
	if mountTarget == nil {
		return nil, fmt.Errorf("mount target cannot be nil")
	}

	mountOptions := []string{
		fmt.Sprintf("addr=%s", mountTarget.IPAddress),
	}

	klog.V(4).Infof("Generated single mount options for IP %s", mountTarget.IPAddress)
	return mountOptions, nil
}

// SelectOptimalMountTargets selects the best mount targets based on instance ENI configuration
// This prioritizes mount targets in the same AZ as the instance's primary ENI
func SelectOptimalMountTargets(mountTargets []*cloud.MountTarget, eniInfo []cloud.ENIInfo, maxTargets int) []*cloud.MountTarget {
	if len(mountTargets) == 0 {
		return nil
	}

	if maxTargets <= 0 {
		maxTargets = len(mountTargets)
	}

	klog.V(4).Infof("Selecting optimal mount targets from %d available, max %d, with %d ENIs", len(mountTargets), maxTargets, len(eniInfo))

	// Group ENIs by AZ
	enisByAZ := make(map[string]int)
	for _, eni := range eniInfo {
		enisByAZ[eni.AZName]++
	}

	// Create a scoring map for mount targets based on ENI distribution
	type mtScore struct {
		mt    *cloud.MountTarget
		score int
	}

	var scores []mtScore
	for _, mt := range mountTargets {
		score := enisByAZ[mt.AZName]
		scores = append(scores, mtScore{mt: mt, score: score})
		klog.V(5).Infof("Mount target %s (AZ: %s) has score %d", mt.IPAddress, mt.AZName, score)
	}

	// Sort by score (descending) to prioritize mount targets with more ENIs in that AZ
	sortByScore := func(i, j int) bool {
		if scores[i].score != scores[j].score {
			return scores[i].score > scores[j].score
		}
		// If scores are equal, maintain original order
		return false
	}

	// Simple bubble sort for the scores
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if sortByScore(j, i) {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// Select the top maxTargets mount targets
	var selected []*cloud.MountTarget
	for i := 0; i < maxTargets && i < len(scores); i++ {
		selected = append(selected, scores[i].mt)
		klog.V(4).Infof("Selected mount target %s (AZ: %s) for multipathing", scores[i].mt.IPAddress, scores[i].mt.AZName)
	}

	return selected
}
