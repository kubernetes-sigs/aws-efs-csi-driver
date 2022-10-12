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
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

type MetadataProvider interface {
	getMetadata() (MetadataService, error)
}

// MetadataService represents AWS metadata service.
type MetadataService interface {
	GetInstanceID() string
	GetRegion() string
	GetAvailabilityZone() string
}

type metadata struct {
	instanceID       string
	region           string
	availabilityZone string
}

var _ MetadataService = &metadata{}

// GetInstanceID returns the instance identification.
func (m *metadata) GetInstanceID() string {
	return m.instanceID
}

// GetRegion returns the region Zone which the instance is in.
func (m *metadata) GetRegion() string {
	return m.region
}

// GetAvailabilityZone returns the Availability Zone which the instance is in.
func (m *metadata) GetAvailabilityZone() string {
	return m.availabilityZone
}

// GetNewMetadataProvider returns a MetadataProvider on which can be invoked getMetadata() to extract the metadata.
func GetNewMetadataProvider(svc EC2Metadata, clientset kubernetes.Interface) (MetadataProvider, error) {
	// check if it is running in ECS otherwise default fall back to ec2
	klog.Info("getting MetadataService...")
	if isDriverBootedInECS() {
		klog.Info("detected driver is running in ECS, returning task metadata...")
		return taskMetadataProvider{taskMetadataService: &taskMetadata{}}, nil
	} else if svc.Available() {
		klog.Info("retrieving metadata from EC2 metadata service")
		return ec2MetadataProvider{ec2MetadataService: svc}, nil
	} else if clientset != nil {
		klog.Info("retrieving metadata from Kubernetes API")
		return kubernetesApiMetadataProvider{api: clientset}, nil
	} else {
		return nil, fmt.Errorf("could not create MetadataProvider from any source")
	}
}
