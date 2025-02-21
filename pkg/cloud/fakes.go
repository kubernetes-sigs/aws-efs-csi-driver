package cloud

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

type FakeCloudProvider struct {
	m            *metadata
	fileSystems  map[string]*FileSystem
	accessPoints map[string]*AccessPoint
	mountTargets map[string]*MountTarget
	tags         map[string]map[string]string
}

func NewFakeCloudProvider() *FakeCloudProvider {
	return &FakeCloudProvider{
		m:            &metadata{"instanceID", "region", "az"},
		fileSystems:  make(map[string]*FileSystem),
		accessPoints: make(map[string]*AccessPoint),
		mountTargets: make(map[string]*MountTarget),
		tags:         make(map[string]map[string]string),
	}
}

func (c *FakeCloudProvider) GetMetadata() MetadataService {
	return c.m
}

func (c *FakeCloudProvider) CreateAccessPoint(ctx context.Context, clientToken string, accessPointOpts *AccessPointOptions) (accessPoint *AccessPoint, err error) {
	ap, exists := c.accessPoints[clientToken]
	if exists {
		if accessPointOpts.CapacityGiB == ap.CapacityGiB {
			return ap, nil
		} else {
			return nil, ErrAlreadyExists
		}
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	apId := fmt.Sprintf("fsap-%d", r.Uint64())
	fsId := accessPointOpts.FileSystemId
	ap = &AccessPoint{
		AccessPointId: apId,
		FileSystemId:  fsId,
		CapacityGiB:   accessPointOpts.CapacityGiB,
	}

	c.accessPoints[clientToken] = ap
	return ap, nil
}

func (c *FakeCloudProvider) DeleteAccessPoint(ctx context.Context, accessPointId string) (err error) {
	for name, ap := range c.accessPoints {
		if ap.AccessPointId == accessPointId {
			delete(c.accessPoints, name)
		}
	}
	return nil
}

func (c *FakeCloudProvider) DescribeAccessPoint(ctx context.Context, accessPointId string) (accessPoint *AccessPoint, err error) {
	for _, ap := range c.accessPoints {
		if ap.AccessPointId == accessPointId {
			return ap, nil
		}
	}
	return nil, ErrNotFound
}

// CreateVolume calls DescribeFileSystem and then CreateAccessPoint.
// Add file system into the map here to allow CreateVolume sanity tests to succeed.
func (c *FakeCloudProvider) DescribeFileSystemById(ctx context.Context, fileSystemId string) (fileSystem *FileSystem, err error) {
	if fs, ok := c.fileSystems[fileSystemId]; ok {
		return fs, nil
	}

	fs := &FileSystem{
		FileSystemId: fileSystemId,
	}
	c.fileSystems[fileSystemId] = fs

	mt := &MountTarget{
		AZName:        "us-east-1a",
		AZId:          "mock-AZ-id",
		MountTargetId: "fsmt-abcd1234",
		IPAddress:     "127.0.0.1",
	}

	c.mountTargets[fileSystemId] = mt
	return fs, nil
}

func (c *FakeCloudProvider) DescribeFileSystemByToken(ctx context.Context, creationToken string) (fileSystem []*FileSystem, err error) {
	var efsList = make([]*FileSystem, 0)
	if fs, ok := c.fileSystems[creationToken]; ok {
		efsList = append(efsList, fs)
		return efsList, nil
	}

	tags := map[string]string{
		"env":   "prod",
		"owner": "avanishpatil23@gmail.com",
	}

	fs := &FileSystem{
		FileSystemId:  creationToken,
		FileSystemArn: "arn:aws:elasticfilesystem:us-west-2:xxxx:file-system/fs-xxxx",
		Tags:          tags,
	}
	c.fileSystems[creationToken] = fs

	mt := &MountTarget{
		AZName:        "us-east-1a",
		AZId:          "mock-AZ-id",
		MountTargetId: "fsmt-abcd1234",
		IPAddress:     "127.0.0.1",
	}

	c.mountTargets[creationToken] = mt
	efsList = append(efsList, c.fileSystems[creationToken])
	return efsList, nil
}

func (c *FakeCloudProvider) DescribeMountTargets(ctx context.Context, fileSystemId, az string) (mountTarget *MountTarget, err error) {
	if mt, ok := c.mountTargets[fileSystemId]; ok {
		return mt, nil
	}

	return nil, ErrNotFound
}

func (c *FakeCloudProvider) FindAccessPointByClientToken(ctx context.Context, clientToken, fileSystemId string) (accessPoint *AccessPoint, err error) {
	if ap, exists := c.accessPoints[clientToken]; exists {
		return ap, nil
	} else {
		return nil, nil
	}
}

func (c *FakeCloudProvider) ListAccessPoints(ctx context.Context, fileSystemId string) ([]*AccessPoint, error) {
	accessPoints := []*AccessPoint{
		c.accessPoints[fileSystemId],
	}
	return accessPoints, nil
}
