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
}

func NewFakeCloudProvider() *FakeCloudProvider {
	return &FakeCloudProvider{
		m:            &metadata{"instanceID", "region", "az"},
		fileSystems:  make(map[string]*FileSystem),
		accessPoints: make(map[string]*AccessPoint),
		mountTargets: make(map[string]*MountTarget),
	}
}

func (c *FakeCloudProvider) GetMetadata() MetadataService {
	return c.m
}

func (c *FakeCloudProvider) CreateAccessPoint(ctx context.Context, volumeName string, accessPointOpts *AccessPointOptions) (accessPoint *AccessPoint, err error) {
	ap, exists := c.accessPoints[volumeName]
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

	c.accessPoints[volumeName] = ap
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
func (c *FakeCloudProvider) DescribeFileSystem(ctx context.Context, fileSystemId string) (fileSystem *FileSystem, err error) {
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

func (c *FakeCloudProvider) DescribeMountTargets(ctx context.Context, fileSystemId, az string) (mountTarget *MountTarget, err error) {
	if mt, ok := c.mountTargets[fileSystemId]; ok {
		return mt, nil
	}

	return nil, ErrNotFound
}
