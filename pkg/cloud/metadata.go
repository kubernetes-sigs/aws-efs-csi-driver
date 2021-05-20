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
	"github.com/aws/aws-sdk-go/aws/session"
	"os"
)

type EC2Metadata interface {
	Available() bool
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

// getEC2Metadata returns a new MetadataServiceImplementation.
func NewMetadataService(sess *session.Session) (MetadataService, error) {

	return &metadata{
		instanceID:       os.Getenv("instanceID"),
		region:           os.Getenv("region"),
		availabilityZone: os.Getenv("availabilityZone"),
	}, nil
}
