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

	"math/rand"
	"os"
	"time"

	"github.com/aws/smithy-go"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/aws/aws-sdk-go-v2/service/s3files"
	s3filestypes "github.com/aws/aws-sdk-go-v2/service/s3files/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"

	"k8s.io/klog/v2"
)

const (
	AccessDeniedException           = "AccessDeniedException"
	AccessPointAlreadyExists        = "AccessPointAlreadyExists"
	ConflictException               = "ConflictException"
	PvcNameTagKey                   = "pvcName"
	AccessPointPerFsLimit           = 10000
	s3FilesListAccessPointsPageSize = 1000
)

var (
	ErrNotFound      = errors.New("Resource was not found")
	ErrAlreadyExists = errors.New("Resource already exists")
	ErrAccessDenied  = errors.New("Access denied")
)

type FileSystem struct {
	FileSystemId         string
	AvailabilityZoneName string
}

type AccessPoint struct {
	AccessPointId      string
	FileSystemId       string
	AccessPointRootDir string
	// Capacity is used for testing purpose only
	// EFS does not consider capacity while provisioning new file systems or access points
	CapacityGiB int64
	PosixUser   *PosixUser
}

type PosixUser struct {
	Gid int64
	Uid int64
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
	CreateAccessPoint(context.Context, *efs.CreateAccessPointInput, ...func(*efs.Options)) (*efs.CreateAccessPointOutput, error)
	DeleteAccessPoint(context.Context, *efs.DeleteAccessPointInput, ...func(*efs.Options)) (*efs.DeleteAccessPointOutput, error)
	DescribeAccessPoints(context.Context, *efs.DescribeAccessPointsInput, ...func(*efs.Options)) (*efs.DescribeAccessPointsOutput, error)
	DescribeFileSystems(context.Context, *efs.DescribeFileSystemsInput, ...func(*efs.Options)) (*efs.DescribeFileSystemsOutput, error)
	DescribeMountTargets(context.Context, *efs.DescribeMountTargetsInput, ...func(*efs.Options)) (*efs.DescribeMountTargetsOutput, error)
}

// S3Files abstracts S3Files client(https://docs.aws.amazon.com/sdk-for-go/api/service/s3files/)
type S3Files interface {
	CreateAccessPoint(context.Context, *s3files.CreateAccessPointInput, ...func(*s3files.Options)) (*s3files.CreateAccessPointOutput, error)
	DeleteAccessPoint(context.Context, *s3files.DeleteAccessPointInput, ...func(*s3files.Options)) (*s3files.DeleteAccessPointOutput, error)
	ListAccessPoints(context.Context, *s3files.ListAccessPointsInput, ...func(*s3files.Options)) (*s3files.ListAccessPointsOutput, error)
	ListFileSystems(context.Context, *s3files.ListFileSystemsInput, ...func(*s3files.Options)) (*s3files.ListFileSystemsOutput, error)
}

type Cloud interface {
	GetMetadata() MetadataService
	CreateAccessPoint(ctx context.Context, clientToken string, accessPointOpts *AccessPointOptions, fsType util.FileSystemType) (accessPoint *AccessPoint, err error)
	DeleteAccessPoint(ctx context.Context, accessPointId string, fsType util.FileSystemType) (err error)
	DescribeAccessPoint(ctx context.Context, accessPointId string, fileSystemId string, fsType util.FileSystemType) (accessPoint *AccessPoint, err error)
	FindAccessPointByClientToken(ctx context.Context, clientToken, fileSystemId string, fsType util.FileSystemType) (accessPoint *AccessPoint, err error)
	ListAccessPoints(ctx context.Context, fileSystemId string, fsType util.FileSystemType) (accessPoints []*AccessPoint, err error)
	DescribeFileSystem(ctx context.Context, fileSystemId string, fsType util.FileSystemType) (fs *FileSystem, err error)
	DescribeMountTargets(ctx context.Context, fileSystemId, az string, fsType util.FileSystemType) (fs *MountTarget, err error)
}

type cloud struct {
	metadata MetadataService
	efs      Efs
	s3files  S3Files
	rm       *retryManager
}

// NewCloud returns a new instance of AWS cloud
// It panics if session is invalid
func NewCloud(adaptiveRetryMode bool) (Cloud, error) {
	return createCloud("", "", adaptiveRetryMode)
}

// NewCloudWithRole returns a new instance of AWS cloud after assuming an aws role
// It panics if driver does not have permissions to assume role.
func NewCloudWithRole(awsRoleArn string, externalId string, adaptiveRetryMode bool) (Cloud, error) {
	return createCloud(awsRoleArn, externalId, adaptiveRetryMode)
}

func createCloud(awsRoleArn string, externalId string, adaptiveRetryMode bool) (Cloud, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		klog.Warningf("Could not load config: %v", err)
	}

	svc := imds.NewFromConfig(cfg)
	api, err := DefaultKubernetesAPIClient()

	if err != nil && !isDriverBootedInECS() {
		klog.Warningf("Could not create Kubernetes Client: %v", err)
	}
	metadataProvider, err := GetNewMetadataProvider(svc, api)
	if err != nil {
		return nil, fmt.Errorf("error creating MetadataProvider: %v", err)
	}

	metadata, err := metadataProvider.getMetadata()

	if err != nil {
		return nil, fmt.Errorf("could not get metadata: %v", err)
	}

	rm := newRetryManager(adaptiveRetryMode)

	efs_client := createEfsClient(awsRoleArn, externalId, metadata)
	klog.V(5).Infof("EFS Client created using the following endpoint: %+v", cfg.BaseEndpoint)
	s3files_client := createS3FilesClient(metadata)
	klog.V(5).Infof("S3 Files Client created using the following endpoint: %+v", cfg.BaseEndpoint)

	return &cloud{
		metadata: metadata,
		efs:      efs_client,
		s3files:  s3files_client,
		rm:       rm,
	}, nil
}

func createEfsClient(awsRoleArn string, externalId string, metadata MetadataService) Efs {
	cfg, _ := config.LoadDefaultConfig(context.TODO(), config.WithRegion(metadata.GetRegion()))
	if awsRoleArn != "" {
		stsClient := sts.NewFromConfig(cfg)
		var roleProvider aws.CredentialsProvider
		if externalId != "" {
			roleProvider = stscreds.NewAssumeRoleProvider(stsClient, awsRoleArn, func(o *stscreds.AssumeRoleOptions) {
				o.ExternalID = &externalId
			})
		} else {
			roleProvider = stscreds.NewAssumeRoleProvider(stsClient, awsRoleArn)
		}
		cfg.Credentials = aws.NewCredentialsCache(roleProvider)
	}
	return efs.NewFromConfig(cfg)
}

func createS3FilesClient(metadata MetadataService) S3Files {
	cfg, _ := config.LoadDefaultConfig(context.TODO(), config.WithRegion(metadata.GetRegion()))
	return s3files.NewFromConfig(cfg)
}

func (c *cloud) GetMetadata() MetadataService {
	return c.metadata
}

func (c *cloud) CreateAccessPoint(ctx context.Context, clientToken string, accessPointOpts *AccessPointOptions, fsType util.FileSystemType) (accessPoint *AccessPoint, err error) {
	switch fsType {
	case util.FileSystemTypeEFS:
		createAPInput := &efs.CreateAccessPointInput{
			ClientToken:  &clientToken,
			FileSystemId: &accessPointOpts.FileSystemId,
			PosixUser: &types.PosixUser{
				Gid: &accessPointOpts.Gid,
				Uid: &accessPointOpts.Uid,
			},
			RootDirectory: &types.RootDirectory{
				CreationInfo: &types.CreationInfo{
					OwnerGid:    &accessPointOpts.Gid,
					OwnerUid:    &accessPointOpts.Uid,
					Permissions: &accessPointOpts.DirectoryPerms,
				},
				Path: &accessPointOpts.DirectoryPath,
			},
			Tags: parseEfsTags(accessPointOpts.Tags),
		}

		klog.V(5).Infof("Calling Create AP with input: %+v", *createAPInput)
		res, err := c.efs.CreateAccessPoint(ctx, createAPInput, func(o *efs.Options) {
			o.Retryer = c.rm.createAccessPointRetryer
		})
		if err != nil {
			if isAccessDenied(err) {
				return nil, ErrAccessDenied
			}
			if isAccessPointAlreadyExists(err) {
				return nil, ErrAlreadyExists
			}
			return nil, fmt.Errorf("Failed to create access point: %v", err)
		}
		klog.V(5).Infof("Create AP response : %+v", res)

		return &AccessPoint{
			AccessPointId: *res.AccessPointId,
			FileSystemId:  *res.FileSystemId,
			CapacityGiB:   accessPointOpts.CapacityGiB,
		}, nil
	case util.FileSystemTypeS3Files:
		createAPInput := &s3files.CreateAccessPointInput{
			ClientToken:  &clientToken,
			FileSystemId: &accessPointOpts.FileSystemId,
			PosixUser: &s3filestypes.PosixUser{
				Gid: &accessPointOpts.Gid,
				Uid: &accessPointOpts.Uid,
			},
			RootDirectory: &s3filestypes.RootDirectory{
				CreationPermissions: &s3filestypes.CreationPermissions{
					OwnerGid:    &accessPointOpts.Gid,
					OwnerUid:    &accessPointOpts.Uid,
					Permissions: &accessPointOpts.DirectoryPerms,
				},
				Path: &accessPointOpts.DirectoryPath,
			},
			Tags: parseS3FilesTags(accessPointOpts.Tags),
		}

		klog.V(5).Infof("Calling Create AP with input: %+v", *createAPInput)
		res, err := c.s3files.CreateAccessPoint(ctx, createAPInput, func(o *s3files.Options) {
			o.Retryer = c.rm.createAccessPointRetryer
		})
		if err != nil {
			if isAccessDenied(err) {
				return nil, ErrAccessDenied
			}
			if isS3FilesAccessPointAlreadyExists(err) {
				return nil, ErrAlreadyExists
			}
			return nil, fmt.Errorf("Failed to create access point: %v", err)
		}
		klog.V(5).Infof("Create AP response : %+v", res)

		return &AccessPoint{
			AccessPointId: *res.AccessPointId,
			FileSystemId:  *res.FileSystemId,
			CapacityGiB:   accessPointOpts.CapacityGiB,
		}, nil
	default:
		return nil, fmt.Errorf("Unsupported fsType: %v", err)
	}
}

func (c *cloud) DeleteAccessPoint(ctx context.Context, accessPointId string, fsType util.FileSystemType) (err error) {
	switch fsType {
	case util.FileSystemTypeEFS:
		deleteAccessPointInput := &efs.DeleteAccessPointInput{AccessPointId: &accessPointId}
		_, err = c.efs.DeleteAccessPoint(ctx, deleteAccessPointInput, func(o *efs.Options) {
			o.Retryer = c.rm.deleteAccessPointRetryer
		})
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
	case util.FileSystemTypeS3Files:
		deleteAccessPointInput := &s3files.DeleteAccessPointInput{AccessPointId: &accessPointId}
		_, err = c.s3files.DeleteAccessPoint(ctx, deleteAccessPointInput, func(o *s3files.Options) {
			o.Retryer = c.rm.deleteAccessPointRetryer
		})
		if err != nil {
			if isAccessDenied(err) {
				return ErrAccessDenied
			}
			if isS3FilesAccessPointNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("Failed to delete access point: %v, error: %v", accessPointId, err)
		}

		return nil
	default:
		return fmt.Errorf("Unsupported fsType: %v", err)
	}
}

func (c *cloud) DescribeAccessPoint(ctx context.Context, accessPointId string, fileSystemId string, fsType util.FileSystemType) (accessPoint *AccessPoint, err error) {
	switch fsType {
	case util.FileSystemTypeEFS:
		describeAPInput := &efs.DescribeAccessPointsInput{
			AccessPointId: &accessPointId,
		}
		res, err := c.efs.DescribeAccessPoints(ctx, describeAPInput, func(o *efs.Options) {
			o.Retryer = c.rm.describeAccessPointsRetryer
		})
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
			return nil, fmt.Errorf("DescribeAccessPoint failed. Expected exactly 1 access point in DescribeAccessPoint result. However, received %d access points", len(accessPoints))
		}

		return &AccessPoint{
			AccessPointId:      *accessPoints[0].AccessPointId,
			FileSystemId:       *accessPoints[0].FileSystemId,
			AccessPointRootDir: *accessPoints[0].RootDirectory.Path,
		}, nil

	case util.FileSystemTypeS3Files:
		var nextToken *string
		for {
			describeAPInput := &s3files.ListAccessPointsInput{
				NextToken:    nextToken,
				FileSystemId: &fileSystemId,
			}

			res, err := c.s3files.ListAccessPoints(ctx, describeAPInput, func(o *s3files.Options) {
				o.Retryer = c.rm.listAccessPointsRetryer
			})
			if err != nil {
				if isAccessDenied(err) {
					return nil, ErrAccessDenied
				}
				return nil, fmt.Errorf("Describe Access Point failed: %v", err)
			}

			for _, ap := range res.AccessPoints {
				if ap.AccessPointId != nil && *ap.AccessPointId == accessPointId {
					return &AccessPoint{
						AccessPointId:      *ap.AccessPointId,
						FileSystemId:       *ap.FileSystemId,
						AccessPointRootDir: *ap.RootDirectory.Path,
					}, nil
				}
			}

			if res.NextToken == nil {
				break
			}
			nextToken = res.NextToken
		}

		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("Unsupported fsType: %v", err)
	}
}

func (c *cloud) FindAccessPointByClientToken(ctx context.Context, clientToken, fileSystemId string, fsType util.FileSystemType) (accessPoint *AccessPoint, err error) {
	klog.V(5).Infof("Filesystem ID to find AP : %+v", fileSystemId)
	klog.V(2).Infof("ClientToken to find AP : %s", clientToken)

	switch fsType {
	case util.FileSystemTypeEFS:
		// TODO: Use pagniation implementation to reduce the value of MaxResults
		describeAPInput := &efs.DescribeAccessPointsInput{
			FileSystemId: &fileSystemId,
			MaxResults:   aws.Int32(AccessPointPerFsLimit), // 10000 is the uppper bound for MaxResults on efs DescribeAccessPoints
		}
		res, err := c.efs.DescribeAccessPoints(ctx, describeAPInput, func(o *efs.Options) {
			o.Retryer = c.rm.describeAccessPointsRetryer
		})
		if err != nil {
			if isAccessDenied(err) {
				return nil, ErrAccessDenied
			}
			if isFileSystemNotFound(err) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("failed to list Access Points of efs = %s : %v", fileSystemId, err)
		}
		for _, ap := range res.AccessPoints {
			// check if AP exists with same client token
			if *ap.ClientToken == clientToken {
				return &AccessPoint{
					AccessPointId:      *ap.AccessPointId,
					FileSystemId:       *ap.FileSystemId,
					AccessPointRootDir: *ap.RootDirectory.Path,
					PosixUser: &PosixUser{
						Gid: *ap.PosixUser.Gid,
						Uid: *ap.PosixUser.Uid,
					},
				}, nil
			}
		}
		klog.V(2).Infof("Access point does not exist")
		return nil, nil
	case util.FileSystemTypeS3Files:
		return nil, fmt.Errorf("FindAccessPointByClientToken is not implemented for s3files in CSI Driver")
	default:
		return nil, fmt.Errorf("Unsupported fsType: %v", err)
	}
}

func (c *cloud) ListAccessPoints(ctx context.Context, fileSystemId string, fsType util.FileSystemType) (accessPoints []*AccessPoint, err error) {
	switch fsType {
	case util.FileSystemTypeEFS:
		// TODO: Use pagniation implementation to reduce the value of MaxResults
		describeAPInput := &efs.DescribeAccessPointsInput{
			FileSystemId: &fileSystemId,
			MaxResults:   aws.Int32(AccessPointPerFsLimit), // 10000 is the uppper bound for MaxResults on efs DescribeAccessPoints
		}
		res, err := c.efs.DescribeAccessPoints(ctx, describeAPInput, func(o *efs.Options) {
			o.Retryer = c.rm.describeAccessPointsRetryer
		})
		if err != nil {
			if isAccessDenied(err) {
				return nil, ErrAccessDenied
			}
			if isFileSystemNotFound(err) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("List Access Points failed: %v", err)
		}

		var posixUser *PosixUser
		for _, accessPointDescription := range res.AccessPoints {
			if accessPointDescription.PosixUser != nil {
				posixUser = &PosixUser{
					Gid: *accessPointDescription.PosixUser.Gid,
					Uid: *accessPointDescription.PosixUser.Uid,
				}
			} else {
				posixUser = nil
			}
			accessPoint := &AccessPoint{
				AccessPointId: *accessPointDescription.AccessPointId,
				FileSystemId:  *accessPointDescription.FileSystemId,
				PosixUser:     posixUser,
			}
			accessPoints = append(accessPoints, accessPoint)
		}

		return accessPoints, nil
	case util.FileSystemTypeS3Files:
		var nextToken *string
		for {
			describeAPInput := &s3files.ListAccessPointsInput{
				FileSystemId: &fileSystemId,
				MaxResults:   aws.Int32(s3FilesListAccessPointsPageSize), // 1000 is upper bound for MaxResults on s3files ListAccessPoints
				NextToken:    nextToken,
			}
			res, err := c.s3files.ListAccessPoints(ctx, describeAPInput, func(o *s3files.Options) {
				o.Retryer = c.rm.listAccessPointsRetryer
			})
			if err != nil {
				if isAccessDenied(err) {
					return nil, ErrAccessDenied
				}
				if isS3FilesFileSystemNotFound(err) {
					return nil, ErrNotFound
				}
				return nil, fmt.Errorf("List Access Points failed: %v", err)
			}

			var posixUser *PosixUser
			for _, accessPointDescription := range res.AccessPoints {
				if accessPointDescription.PosixUser != nil {
					posixUser = &PosixUser{
						Gid: *accessPointDescription.PosixUser.Gid,
						Uid: *accessPointDescription.PosixUser.Uid,
					}
				} else {
					posixUser = nil
				}
				accessPoint := &AccessPoint{
					AccessPointId: *accessPointDescription.AccessPointId,
					FileSystemId:  *accessPointDescription.FileSystemId,
					PosixUser:     posixUser,
				}
				accessPoints = append(accessPoints, accessPoint)
			}

			if res.NextToken == nil {
				break
			}
			nextToken = res.NextToken
		}

		return accessPoints, nil
	default:
		return nil, fmt.Errorf("Unsupported fsType: %v", err)
	}
}

func (c *cloud) DescribeFileSystem(ctx context.Context, fileSystemId string, fsType util.FileSystemType) (fs *FileSystem, err error) {
	switch fsType {
	case util.FileSystemTypeEFS:
		describeFsInput := &efs.DescribeFileSystemsInput{FileSystemId: &fileSystemId}
		klog.V(5).Infof("Calling DescribeFileSystems with input: %+v", *describeFsInput)
		res, err := c.efs.DescribeFileSystems(ctx, describeFsInput, func(o *efs.Options) {
			o.Retryer = c.rm.describeFileSystemsRetryer
		})
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

		// handle nil AvailabilityZoneName for regional efs fs
		var availabilityZoneName string
		if res.FileSystems[0].AvailabilityZoneName != nil {
			availabilityZoneName = *res.FileSystems[0].AvailabilityZoneName
		}

		return &FileSystem{
			FileSystemId:         *res.FileSystems[0].FileSystemId,
			AvailabilityZoneName: availabilityZoneName,
		}, nil
	case util.FileSystemTypeS3Files:
		var nextToken *string

		for {
			describeFsInput := &s3files.ListFileSystemsInput{
				NextToken: nextToken,
			}
			klog.V(5).Infof("Calling ListFileSystemsInput with input: %+v", *describeFsInput)
			res, err := c.s3files.ListFileSystems(ctx, describeFsInput, func(o *s3files.Options) {
				o.Retryer = c.rm.listFileSystemsRetryer
			})
			if err != nil {
				if isAccessDenied(err) {
					return nil, ErrAccessDenied
				}
				return nil, fmt.Errorf("Describe S3 Files failed: %v", err)
			}

			for _, fs := range res.FileSystems {
				if fs.FileSystemId != nil && *fs.FileSystemId == fileSystemId {
					return &FileSystem{
						FileSystemId: *fs.FileSystemId,
					}, nil
				}
			}

			if res.NextToken == nil {
				break
			}
			nextToken = res.NextToken
		}

		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("Unsupported fsType: %v", err)
	}
}

func (c *cloud) DescribeMountTargets(ctx context.Context, fileSystemId, azName string, fsType util.FileSystemType) (fs *MountTarget, err error) {
	switch fsType {
	case util.FileSystemTypeEFS:
		describeMtInput := &efs.DescribeMountTargetsInput{FileSystemId: &fileSystemId}
		klog.V(5).Infof("Calling DescribeMountTargets with input: %+v", *describeMtInput)
		res, err := c.efs.DescribeMountTargets(ctx, describeMtInput, func(o *efs.Options) {
			o.Retryer = c.rm.describeMountTargetsRetryer
		})
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

		var mountTarget *types.MountTargetDescription
		if azName != "" {
			mountTarget = getMountTargetForAz(availableMountTargets, azName)
		}

		// Pick random Mount target from available mount target if azName is not provided.
		// Or if there is no mount target matching azName
		if mountTarget == nil {
			klog.Infof("Picking a random mount target from available mount target")
			rand.Seed(time.Now().Unix())
			mountTarget = &availableMountTargets[rand.Intn(len(availableMountTargets))]
		}

		return &MountTarget{
			AZName:        *mountTarget.AvailabilityZoneName,
			AZId:          *mountTarget.AvailabilityZoneId,
			MountTargetId: *mountTarget.MountTargetId,
			IPAddress:     *mountTarget.IpAddress,
		}, nil
	case util.FileSystemTypeS3Files:
		return nil, fmt.Errorf("DescribeMountTargets is not implemented for s3files in CSI Driver")
	default:
		return nil, fmt.Errorf("Unsupported fsType: %v", err)
	}
}

func isFileSystemNotFound(err error) bool {
	var FileSystemNotFoundErr *types.FileSystemNotFound
	if errors.As(err, &FileSystemNotFoundErr) {
		return true
	}
	return false
}

func isS3FilesFileSystemNotFound(err error) bool {
	var FileSystemNotFoundErr *s3filestypes.ResourceNotFoundException
	return errors.As(err, &FileSystemNotFoundErr)
}

func isAccessPointNotFound(err error) bool {
	var AccessPointNotFoundErr *types.AccessPointNotFound
	if errors.As(err, &AccessPointNotFoundErr) {
		return true
	}
	return false
}

func isS3FilesAccessPointNotFound(err error) bool {
	var AccessPointNotFoundErr *s3filestypes.ResourceNotFoundException
	return errors.As(err, &AccessPointNotFoundErr)
}

func isAccessDenied(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == AccessDeniedException {
			return true
		}
	}
	return false
}

func isAccessPointAlreadyExists(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == AccessPointAlreadyExists {
			return true
		}
	}
	return false
}

func isS3FilesAccessPointAlreadyExists(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == ConflictException {
			return true
		}
	}
	return false
}

func isDriverBootedInECS() bool {
	ecsContainerMetadataUri := os.Getenv(taskMetadataV4EnvName)
	return ecsContainerMetadataUri != ""
}

func parseEfsTags(tagMap map[string]string) []types.Tag {
	efsTags := []types.Tag{}
	for k, v := range tagMap {
		key := k
		value := v
		efsTags = append(efsTags, types.Tag{
			Key:   &key,
			Value: &value,
		})
	}
	return efsTags
}

func parseS3FilesTags(tagMap map[string]string) []s3filestypes.Tag {
	s3FilesTags := []s3filestypes.Tag{}
	for k, v := range tagMap {
		key := k
		value := v
		s3FilesTags = append(s3FilesTags, s3filestypes.Tag{
			Key:   &key,
			Value: &value,
		})
	}
	return s3FilesTags
}

func getAvailableMountTargets(mountTargets []types.MountTargetDescription) []types.MountTargetDescription {
	availableMountTargets := []types.MountTargetDescription{}
	for _, mt := range mountTargets {
		if mt.LifeCycleState == "available" {
			availableMountTargets = append(availableMountTargets, mt)
		}
	}

	return availableMountTargets
}

func getMountTargetForAz(mountTargets []types.MountTargetDescription, azName string) *types.MountTargetDescription {
	for _, mt := range mountTargets {
		if *mt.AvailabilityZoneName == azName {
			return &mt
		}
	}
	klog.Infof("There is no mount target match %v", azName)
	return nil
}
