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
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/efs"
	"k8s.io/klog"
	"math/rand"
	"time"
)

const (
	AccessDeniedException = "AccessDeniedException"
)

var (
	ErrNotFound      = errors.New("Resource was not found")
	ErrAlreadyExists = errors.New("Resource already exists")
	ErrAccessDenied  = errors.New("Access denied")
)

type FileSystem struct {
	FileSystemId string
}

type AccessPoint struct {
	AccessPointId      string
	FileSystemId       string
	AccessPointRootDir string
	// Capacity is used for testing purpose only
	// EFS does not consider capacity while provisioning new file systems or access points
	CapacityGiB int64
}

type AccessPointOptions struct {
	// Capacity is used for testing purpose only.
	// EFS does not consider capacity while provisioning new file systems or access points
	// Capacity is used to satisfy this test: https://github.com/kubernetes-csi/csi-test/blob/v3.1.1/pkg/sanity/controller.go#L559
	CapacityGiB    int64
	FileSystemId   string
	Uid            int64
	Gid            int64
	DirectoryPerms string
	DirectoryPath  string
	Tags           map[string]string
}

type MountTarget struct {
	AZName        string
	AZId          string
	MountTargetId string
	IPAddress     string
}

// Efs abstracts efs client(https://docs.aws.amazon.com/sdk-for-go/api/service/efs/)
type Efs interface {
	CreateAccessPointWithContext(aws.Context, *efs.CreateAccessPointInput, ...request.Option) (*efs.CreateAccessPointOutput, error)
	DeleteAccessPointWithContext(aws.Context, *efs.DeleteAccessPointInput, ...request.Option) (*efs.DeleteAccessPointOutput, error)
	DescribeAccessPointsWithContext(aws.Context, *efs.DescribeAccessPointsInput, ...request.Option) (*efs.DescribeAccessPointsOutput, error)
	DescribeFileSystemsWithContext(aws.Context, *efs.DescribeFileSystemsInput, ...request.Option) (*efs.DescribeFileSystemsOutput, error)
	DescribeMountTargetsWithContext(aws.Context, *efs.DescribeMountTargetsInput, ...request.Option) (*efs.DescribeMountTargetsOutput, error)
}

type Cloud interface {
	GetMetadata() MetadataService
	CreateAccessPoint(ctx context.Context, volumeName string, accessPointOpts *AccessPointOptions) (accessPoint *AccessPoint, err error)
	DeleteAccessPoint(ctx context.Context, accessPointId string) (err error)
	DescribeAccessPoint(ctx context.Context, accessPointId string) (accessPoint *AccessPoint, err error)
	DescribeFileSystem(ctx context.Context, fileSystemId string) (fs *FileSystem, err error)
	DescribeMountTargets(ctx context.Context, fileSystemId, az string) (fs *MountTarget, err error)
}

type cloud struct {
	metadata MetadataService
	efs      Efs
}

// NewCloud returns a new instance of AWS cloud
// It panics if session is invalid
func NewCloud() (Cloud, error) {
	sess := session.Must(session.NewSession(&aws.Config{}))
	metadata, err := NewMetadataService(sess)
	if err != nil {
		return nil, fmt.Errorf("could not get metadata from AWS: %v", err)
	}

	efsClient := efs.New(session.Must(session.NewSession(aws.NewConfig().WithRegion(metadata.GetRegion()))))
	return &cloud{
		metadata: metadata,
		efs:      efsClient,
	}, nil
}

// NewCloudWithRole returns a new instance of AWS cloud after assuming an aws role
// It panics if driver does not have permissions to assume role.
func NewCloudWithRole(awsRoleArn string) (Cloud, error) {
	sess := session.Must(session.NewSession(&aws.Config{}))
	metadata, err := NewMetadataService(sess)
	if err != nil {
		return nil, fmt.Errorf("Could not get metadata from AWS: %v", err)
	}

	creds := stscreds.NewCredentials(sess, awsRoleArn)
	efsClient := efs.New(session.Must(session.NewSession(aws.NewConfig().WithCredentials(creds).WithRegion(metadata.GetRegion()))))
	return &cloud{
		metadata: metadata,
		efs:      efsClient,
	}, nil
}

func (c *cloud) GetMetadata() MetadataService {
	return c.metadata
}

func (c *cloud) CreateAccessPoint(ctx context.Context, volumeName string, accessPointOpts *AccessPointOptions) (accessPoint *AccessPoint, err error) {
	efsTags := parseEfsTags(accessPointOpts.Tags)
	createAPInput := &efs.CreateAccessPointInput{
		ClientToken:  &volumeName,
		FileSystemId: &accessPointOpts.FileSystemId,
		PosixUser: &efs.PosixUser{
			Gid: &accessPointOpts.Gid,
			Uid: &accessPointOpts.Uid,
		},
		RootDirectory: &efs.RootDirectory{
			CreationInfo: &efs.CreationInfo{
				OwnerGid:    &accessPointOpts.Gid,
				OwnerUid:    &accessPointOpts.Uid,
				Permissions: &accessPointOpts.DirectoryPerms,
			},
			Path: &accessPointOpts.DirectoryPath,
		},
		Tags: efsTags,
	}

	klog.V(5).Infof("Calling Create AP with input: %+v", *createAPInput)
	res, err := c.efs.CreateAccessPointWithContext(ctx, createAPInput)
	if err != nil {
		if isAccessDenied(err) {
			return nil, ErrAccessDenied
		}
		return nil, fmt.Errorf("Failed to create access point: %v", err)
	}

	return &AccessPoint{
		AccessPointId: *res.AccessPointId,
		FileSystemId:  *res.FileSystemId,
		CapacityGiB:   accessPointOpts.CapacityGiB,
	}, nil
}

func (c *cloud) DeleteAccessPoint(ctx context.Context, accessPointId string) (err error) {
	deleteAccessPointInput := &efs.DeleteAccessPointInput{AccessPointId: &accessPointId}
	_, err = c.efs.DeleteAccessPointWithContext(ctx, deleteAccessPointInput)
	if err != nil {
		if isAccessDenied(err) {
			return ErrAccessDenied
		}
		if isAccessPointNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("Failed to delete access point: %v, error: %v", accessPointId, err)
	}

	return nil
}

func (c *cloud) DescribeAccessPoint(ctx context.Context, accessPointId string) (accessPoint *AccessPoint, err error) {
	describeAPInput := &efs.DescribeAccessPointsInput{
		AccessPointId: &accessPointId,
	}
	res, err := c.efs.DescribeAccessPointsWithContext(ctx, describeAPInput)
	if err != nil {
		if isAccessDenied(err) {
			return nil, ErrAccessDenied
		}
		if isAccessPointNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("Describe Access Point failed: %v", err)
	}

	accessPoints := res.AccessPoints
	if len(accessPoints) == 0 || len(accessPoints) > 1 {
		return nil, fmt.Errorf("DescribeAccessPoint failed. Expected exactly 1 access point in DescribeAccessPoint result. However, recevied %d access points", len(accessPoints))
	}

	return &AccessPoint{
		AccessPointId:      *accessPoints[0].AccessPointId,
		FileSystemId:       *accessPoints[0].FileSystemId,
		AccessPointRootDir: *accessPoints[0].RootDirectory.Path,
	}, nil
}

func (c *cloud) DescribeFileSystem(ctx context.Context, fileSystemId string) (fs *FileSystem, err error) {
	describeFsInput := &efs.DescribeFileSystemsInput{FileSystemId: &fileSystemId}
	klog.V(5).Infof("Calling DescribeFileSystems with input: %+v", *describeFsInput)
	res, err := c.efs.DescribeFileSystemsWithContext(ctx, describeFsInput)
	if err != nil {
		if isAccessDenied(err) {
			return nil, ErrAccessDenied
		}
		if isFileSystemNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("Describe File System failed: %v", err)
	}

	fileSystems := res.FileSystems
	if len(fileSystems) == 0 || len(fileSystems) > 1 {
		return nil, fmt.Errorf("DescribeFileSystem failed. Expected exactly 1 file system in DescribeFileSystem result. However, recevied %d file systems", len(fileSystems))
	}
	return &FileSystem{
		FileSystemId: *res.FileSystems[0].FileSystemId,
	}, nil
}

func (c *cloud) DescribeMountTargets(ctx context.Context, fileSystemId, azName string) (fs *MountTarget, err error) {
	describeMtInput := &efs.DescribeMountTargetsInput{FileSystemId: &fileSystemId}
	klog.V(5).Infof("Calling DescribeMountTargets with input: %+v", *describeMtInput)
	res, err := c.efs.DescribeMountTargetsWithContext(ctx, describeMtInput)
	if err != nil {
		if isAccessDenied(err) {
			return nil, ErrAccessDenied
		}
		if isFileSystemNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("Describe Mount Targets failed: %v", err)
	}

	mountTargets := res.MountTargets
	if len(mountTargets) == 0 {
		return nil, fmt.Errorf("Cannot find mount targets for file system %v. Please create mount targets for file system.", fileSystemId)
	}

	availableMountTargets := getAvailableMountTargets(mountTargets)

	if len(availableMountTargets) == 0 {
		return nil, fmt.Errorf("No mount target for file system %v is in available state. Please retry in 5 minutes.", fileSystemId)
	}

	var mountTarget *efs.MountTargetDescription
	if azName != "" {
		mountTarget = getMountTargetForAz(availableMountTargets, azName)
	}

	// Pick random Mount target from available mount target if azName is not provided.
	// Or if there is no mount target matching azName
	if mountTarget == nil {
		klog.Infof("Picking a random mount target from available mount target")
		rand.Seed(time.Now().Unix())
		mountTarget = availableMountTargets[rand.Intn(len(availableMountTargets))]
	}

	return &MountTarget{
		AZName:        *mountTarget.AvailabilityZoneName,
		AZId:          *mountTarget.AvailabilityZoneId,
		MountTargetId: *mountTarget.MountTargetId,
		IPAddress:     *mountTarget.IpAddress,
	}, nil
}

func isFileSystemNotFound(err error) bool {
	if awsErr, ok := err.(awserr.Error); ok {
		if awsErr.Code() == efs.ErrCodeFileSystemNotFound {
			return true
		}
	}
	return false
}

func isAccessPointNotFound(err error) bool {
	if awsErr, ok := err.(awserr.Error); ok {
		if awsErr.Code() == efs.ErrCodeAccessPointNotFound {
			return true
		}
	}
	return false
}

func isAccessDenied(err error) bool {
	if awsErr, ok := err.(awserr.Error); ok {
		if awsErr.Code() == AccessDeniedException {
			return true
		}
	}
	return false
}

func parseEfsTags(tagMap map[string]string) []*efs.Tag {
	efsTags := []*efs.Tag{}
	for k, v := range tagMap {
		key := k
		value := v
		efsTags = append(efsTags, &efs.Tag{
			Key:   &key,
			Value: &value,
		})
	}
	return efsTags
}

func getAvailableMountTargets(mountTargets []*efs.MountTargetDescription) []*efs.MountTargetDescription {
	availableMountTargets := []*efs.MountTargetDescription{}
	for _, mt := range mountTargets {
		if *mt.LifeCycleState == "available" {
			availableMountTargets = append(availableMountTargets, mt)
		}
	}

	return availableMountTargets
}

func getMountTargetForAz(mountTargets []*efs.MountTargetDescription, azName string) *efs.MountTargetDescription {
	for _, mt := range mountTargets {
		if *mt.AvailabilityZoneName == azName {
			return mt
		}
	}
	klog.Infof("There is no mount target match %v", azName)
	return nil
}
