package driver

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

func TestCreateVolume(t *testing.T) {
	var (
		endpoint            = "endpoint"
		volumeName          = "volumeName"
		fsId                = "fs-abcd1234"
		apId                = "fsap-abcd1234xyz987"
		volumeId            = "fs-abcd1234::fsap-abcd1234xyz987"
		capacityRange int64 = 5368709120
		stdVolCap           = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			},
		}
	)
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success: Using fixed UID/GID",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						BasePath:         "test",
						Uid:              "1000",
						Gid:              "1001",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.Uid != 1000 {
							t.Fatalf("Uid mimatched. Expected: %v, actual: %v", accessPointOpts.Uid, 1000)
						}
						if accessPointOpts.Gid != 1001 {
							t.Fatalf("Gid mimatched. Expected: %v, actual: %v", accessPointOpts.Uid, 1001)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Using fixed UID/GID and GID range",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						BasePath:         "test",
						GidMin:           "5000",
						GidMax:           "10000",
						Uid:              "1000",
						Gid:              "1001",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.Uid != 1000 {
							t.Fatalf("Uid mimatched. Expected: %v, actual: %v", accessPointOpts.Uid, 1000)
						}
						if accessPointOpts.Gid != 1001 {
							t.Fatalf("Gid mimatched. Expected: %v, actual: %v", accessPointOpts.Uid, 1001)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: avoiding GID collision",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						BasePath:         "test",
						GidMin:           "1000",
						GidMax:           "1003",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				accessPoints := []*cloud.AccessPoint{
					{
						AccessPointId: apId,
						FileSystemId:  fsId,
						PosixUser: &cloud.PosixUser{
							Gid: 1003,
							Uid: 1003,
						},
					},
					{
						AccessPointId: apId,
						FileSystemId:  fsId,
						PosixUser: &cloud.PosixUser{
							Gid: 1002,
							Uid: 1002,
						},
					},
				}

				var expectedGid int64 = 1001 //1003 and 1002 are taken, next available is 1001
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.Uid != expectedGid {
							t.Fatalf("Uid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Uid)
						}
						if accessPointOpts.Gid != expectedGid {
							t.Fatalf("Gid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Gid)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: reuse released GID",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						BasePath:         "test",
						GidMin:           "1000",
						GidMax:           "1004",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				ap1 := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1001,
						Uid: 1001,
					},
				}
				ap2 := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1002,
						Uid: 1002,
					},
				}
				ap3 := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1003,
						Uid: 1003,
					},
				}
				ap4 := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1004,
						Uid: 1004,
					},
				}

				// Let allocator jump over some GIDS.
				accessPoints := []*cloud.AccessPoint{ap3, ap4}
				var expectedGid int64 = 1002 // 1003 and 1004 is taken.

				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(ap2, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.Uid != expectedGid {
							t.Fatalf("Uid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Uid)
						}
						if accessPointOpts.Gid != expectedGid {
							t.Fatalf("Gid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Gid)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				// 2. Simulate access point removal and verify their GIDs returned to allocator.
				accessPoints = []*cloud.AccessPoint{}
				expectedGid = 1004 // 1003 and 1004 are now free, if no GID return would happen allocator would pick 1001.

				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(ap3, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.Uid != expectedGid {
							t.Fatalf("Uid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Uid)
						}
						if accessPointOpts.Gid != expectedGid {
							t.Fatalf("Gid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Gid)
						}
					})

				res, err = driver.CreateVolume(ctx, req)
				////
				accessPoints = []*cloud.AccessPoint{ap1, ap4}

				expectedGid = 1003
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(ap2, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.Uid != expectedGid {
							t.Fatalf("Uid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Uid)
						}
						if accessPointOpts.Gid != expectedGid {
							t.Fatalf("Gid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Gid)
						}
					})

				res, err = driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: EFS access point limit",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						BasePath:         "test",
						GidMin:           "1000",
						GidMax:           "1200",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}

				accessPoints := []*cloud.AccessPoint{}
				for i := 0; i < 119; i++ {
					gidMax, err := strconv.Atoi(req.Parameters[GidMax])
					if err != nil {
						t.Fatalf("Failed to convert GidMax Parameter to int.")
					}
					userGid := gidMax - i
					ap := &cloud.AccessPoint{
						AccessPointId: apId,
						FileSystemId:  fsId,
						PosixUser: &cloud.PosixUser{
							Gid: int64(userGid),
							Uid: int64(userGid),
						},
					}
					accessPoints = append(accessPoints, ap)
				}

				lastAccessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1081,
						Uid: 1081,
					},
				}

				expectedGid := 1081
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(lastAccessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.Uid != int64(expectedGid) {
							t.Fatalf("Uid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Uid)
						}
						if accessPointOpts.Gid != int64(expectedGid) {
							t.Fatalf("Gid mismatched. Expected: %v, actual: %v", expectedGid, accessPointOpts.Gid)
						}
					})

				var err error

				// Allocate last available GID
				_, err = driver.CreateVolume(ctx, req)
				if err != nil {
					t.Fatalf("CreateVolume failed.")
				}

				accessPoints = append(accessPoints, lastAccessPoint)
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(lastAccessPoint, nil).AnyTimes()

				// All 120 GIDs are taken now, internal limit should take effect causing CreateVolume to fail.
				_, err = driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatalf("CreateVolume should have failed.")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						AzName:           "us-east-1a",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1000,
						Uid: 1000,
					},
				}
				accessPoints := []*cloud.AccessPoint{accessPoint}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil)

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Using Default GID ranges",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						BasePath:         "test",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: DefaultGidMin - 1, //use GID that is not in default range
					},
				}
				accessPoints := []*cloud.AccessPoint{accessPoint}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil)

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with tags",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr("cluster:efs"),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1000,
						Uid: 1000,
					},
				}
				accessPoints := []*cloud.AccessPoint{accessPoint}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil)

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with invalid tags",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr("cluster-efs"),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1000,
						Uid: 1000,
					},
				}
				accessPoints := []*cloud.AccessPoint{accessPoint}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(accessPoints, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil)

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with a valid directory structure set",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				pvName := "foo"
				pvcName := "bar"
				directoryCreated := fmt.Sprintf("/%s/%s", pvName, pvcName)

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						SubPathPattern:   "${.PV.name}/${.PVC.name}",
						PvName:           pvName,
						PvcName:          pvcName,
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if !verifyPathWhenUUIDIncluded(accessPointOpts.DirectoryPath, directoryCreated) {
							t.Fatalf("Root directory mismatch. Expected: %v (with UID appended), actual: %v",
								directoryCreated,
								accessPointOpts.DirectoryPath)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with a valid directory structure set, using a single element",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				pvcName := "foo"
				directoryCreated := fmt.Sprintf("/%s", pvcName)

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						SubPathPattern:   "${.PVC.name}",
						PvcName:          pvcName,
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if !verifyPathWhenUUIDIncluded(accessPointOpts.DirectoryPath, directoryCreated) {
							t.Fatalf("Root directory mismatch. Expected: %v (with UID appended), actual: %v",
								directoryCreated,
								accessPointOpts.DirectoryPath)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with a valid directory structure set, and a basePath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				pvcName := "foo"
				basePath := "bash"
				directoryCreated := fmt.Sprintf("/%s/%s", basePath, pvcName)

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode:      "efs-ap",
						FsId:                  fsId,
						GidMin:                "1000",
						GidMax:                "2000",
						DirectoryPerms:        "777",
						SubPathPattern:        "${.PVC.name}",
						BasePath:              basePath,
						EnsureUniqueDirectory: "true",
						PvcName:               pvcName,
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if !verifyPathWhenUUIDIncluded(accessPointOpts.DirectoryPath, directoryCreated) {
							t.Fatalf("Root directory mismatch. Expected: %v (with UID appended), actual: %v",
								directoryCreated,
								accessPointOpts.DirectoryPath)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with a valid directory structure set, and a basePath, and uniqueness guarantees turned off",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				pvcName := "foo"
				basePath := "bash"
				directoryCreated := fmt.Sprintf("/%s/%s", basePath, pvcName)

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode:      "efs-ap",
						FsId:                  fsId,
						GidMin:                "1000",
						GidMax:                "2000",
						DirectoryPerms:        "777",
						SubPathPattern:        "${.PVC.name}",
						BasePath:              basePath,
						EnsureUniqueDirectory: "false",
						PvcName:               pvcName,
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.DirectoryPath != directoryCreated {
							t.Fatalf("Root directory mismatch. Expected: %v, actual: %v",
								directoryCreated,
								accessPointOpts.DirectoryPath)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with a valid directory structure set, but ensuring uniqueness is set incorrectly, so default of true is used." +
				"",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				pvcName := "foo"
				directoryCreated := fmt.Sprintf("/%s", pvcName)

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode:      "efs-ap",
						FsId:                  fsId,
						GidMin:                "1000",
						GidMax:                "2000",
						DirectoryPerms:        "777",
						SubPathPattern:        "${.PVC.name}",
						EnsureUniqueDirectory: "banana",
						PvcName:               pvcName,
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if !verifyPathWhenUUIDIncluded(accessPointOpts.DirectoryPath, directoryCreated) {
							t.Fatalf("Root directory mismatch. Expected: %v (with UID appended), actual: %v",
								directoryCreated,
								accessPointOpts.DirectoryPath)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with an empty subPath Pattern, no basePath and uniqueness guarantees turned off",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode:      "efs-ap",
						FsId:                  fsId,
						GidMin:                "1000",
						GidMax:                "2000",
						DirectoryPerms:        "777",
						SubPathPattern:        "",
						EnsureUniqueDirectory: "false",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.DirectoryPath != "/" {
							t.Fatalf("Root directory mismatch. Expected: %v, actual: %v",
								"/",
								accessPointOpts.DirectoryPath)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with an empty subPath Pattern, and basePath set to /",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode:      "efs-ap",
						FsId:                  fsId,
						GidMin:                "1000",
						GidMax:                "2000",
						DirectoryPerms:        "777",
						SubPathPattern:        "",
						BasePath:              "/",
						EnsureUniqueDirectory: "false",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if accessPointOpts.DirectoryPath != "/" {
							t.Fatalf("Root directory mismatch. Expected: %v, actual: %v",
								"/",
								accessPointOpts.DirectoryPath)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with a valid directory structure set, using repeated elements (uses PVC Name in subpath pattern multiple times)",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				pvcName := "foo"
				directoryCreated := fmt.Sprintf("/%s/%s", pvcName, pvcName)

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						SubPathPattern:   "${.PVC.name}/${.PVC.name}",
						PvcName:          pvcName,
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				accessPoint := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).
					Do(func(ctx context.Context, volumeName string, accessPointOpts *cloud.AccessPointOptions) {
						if !verifyPathWhenUUIDIncluded(accessPointOpts.DirectoryPath, directoryCreated) {
							t.Fatalf("Root directory mismatch. Expected: %v (with UID appended), actual: %v",
								directoryCreated,
								accessPointOpts.DirectoryPath)
						}
					})

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != volumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", volumeId, res.Volume.VolumeId)
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Volume name missing",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Capacity Range missing",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Volume capability Missing",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Volume capability Not Supported",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{
								Mount: &csi.VolumeCapability_MountVolume{},
							},
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
							},
						},
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Volume fsType not supported",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{
								Mount: &csi.VolumeCapability_MountVolume{
									FsType: "abc",
								},
							},
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
							},
						},
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						AzName:           "us-east-1a",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: AccessType is block",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Block{
								Block: &csi.VolumeCapability_BlockVolume{},
							},
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
							},
						},
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Provisioning Mode Not Supported",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-fs",
						FsId:             fsId,
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Missing Provisioning Mode parameter",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						FsId:           fsId,
						DirectoryPerms: "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Missing Parameter FsId",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: FsId cannot be blank",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             "     ",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Uid invalid",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						Uid:              "invalid",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Uid cannot be negative",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						Uid:              "-5",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Gid invalid",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						Gid:              "invalid",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Gid cannot be negative",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						Gid:              "-5",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Gid min cannot be 0",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						GidMin:           "0",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: GidMin must be an integer",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						GidMin:           "test",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: GidMax must be an integer",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						GidMin:           "2000",
						GidMax:           "test",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: GidMax must be greater than GidMin",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						GidMin:           "2000",
						GidMax:           "1000",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: GidMax must be provided with GidMin",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						GidMin:           "2000",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: GidMin must be provided with GidMax",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						GidMax:           "2000",
					},
				}

				ctx := context.Background()
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: File system does not exist",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(nil, cloud.ErrNotFound)
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: DescribeFileSystem Access Denied",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(nil, cloud.ErrAccessDenied)
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Describe File system call fails",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("DescribeFileSystem failed"))
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Create Access Point call fails",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return([]*cloud.AccessPoint{}, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(nil, errors.New("CreateAccessPoint call failed"))
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: CreateAccessPoint Access Denied",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return([]*cloud.AccessPoint{}, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(nil, cloud.ErrAccessDenied)
				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Run out of GIDs",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "1001",
						DirectoryPerms:   "777",
					},
				}

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				ap1 := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1000,
						Uid: 1000,
					},
				}
				ap2 := &cloud.AccessPoint{
					AccessPointId: apId,
					FileSystemId:  fsId,
					PosixUser: &cloud.PosixUser{
						Gid: 1001,
						Uid: 1001,
					},
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil).AnyTimes()
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return([]*cloud.AccessPoint{ap1, ap2}, nil).AnyTimes()
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(ap2, nil).AnyTimes()

				var err error
				// All GIDs from available range are taken, CreateVolume should fail.
				_, err = driver.CreateVolume(ctx, req)

				if err == nil {
					t.Fatalf("CreateVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Cannot assume role for x-account",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				secrets := map[string]string{}
				secrets["awsRoleArn"] = "arn:aws:iam::1234567890:role/EFSCrossAccountRole"

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						AzName:           "us-east-1a",
					},
					Secrets: secrets,
				}

				ctx := context.Background()

				_, err := driver.CreateVolume(ctx, req)

				if err == nil {
					t.Fatalf("CreateVolume did not fail")
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Fail: subPathPattern is specified but uses unsupported attributes",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				subPathPattern := "${.PVC.name}/${foo}"

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						SubPathPattern:   subPathPattern,
					},
				}

				ctx := context.Background()

				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}

				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("Did not throw InvalidArgument error, instead threw %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: resulting accessPointDirectory is too over 100 characters",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				subPathPattern := "this-directory-name-is-far-too-long-for-any-practical-purposes-and-only-serves-to-prove-a-point"

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						SubPathPattern:   subPathPattern,
					},
				}

				ctx := context.Background()

				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}

				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("Did not throw InvalidArgument error, instead threw %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail:  resulting accessPointDirectory contains over 4 subdirectories",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				subPathPattern := "a/b/c/d/e/f"

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-ap",
						FsId:             fsId,
						DirectoryPerms:   "777",
						SubPathPattern:   subPathPattern,
					},
				}

				ctx := context.Background()

				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}

				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().ListAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, nil)

				_, err := driver.CreateVolume(ctx, req)
				if err == nil {
					t.Fatal("CreateVolume did not fail")
				}
				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("Did not throw InvalidArgument error, instead threw %v", err)
				}
				mockCtl.Finish()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestDeleteVolume(t *testing.T) {
	var (
		apId     = "fsap-abcd1234xyz987"
		fsId     = "fs-abcd1234"
		endpoint = "endpoint"
		volumeId = "fs-abcd1234::fsap-abcd1234xyz987"
	)

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success: Normal flow",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil)
				_, err := driver.DeleteVolume(ctx, req)
				if err != nil {
					t.Fatalf("Delete Volume failed: %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with deleteAccessPointRootDir",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				driver := &Driver{
					endpoint:                 endpoint,
					cloud:                    mockCloud,
					mounter:                  mockMounter,
					gidAllocator:             NewGidAllocator(mockCloud),
					deleteAccessPointRootDir: true,
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				accessPoint := &cloud.AccessPoint{
					AccessPointId:      apId,
					FileSystemId:       fsId,
					AccessPointRootDir: "",
					CapacityGiB:        0,
				}

				ctx := context.Background()
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(accessPoint, nil)
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil)
				_, err := driver.DeleteVolume(ctx, req)
				if err != nil {
					t.Fatalf("Delete Volume failed: %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: DescribeAccessPoint Access Point Does not exist",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				driver := &Driver{
					endpoint:                 endpoint,
					cloud:                    mockCloud,
					mounter:                  mockMounter,
					gidAllocator:             NewGidAllocator(mockCloud),
					deleteAccessPointRootDir: true,
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil, cloud.ErrNotFound)
				_, err := driver.DeleteVolume(ctx, req)
				if err != nil {
					t.Fatalf("Delete Volume failed: %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: DescribeAccessPoint Access Denied",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				driver := &Driver{
					endpoint:                 endpoint,
					cloud:                    mockCloud,
					mounter:                  mockMounter,
					gidAllocator:             NewGidAllocator(mockCloud),
					deleteAccessPointRootDir: true,
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil, cloud.ErrAccessDenied)
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatalf("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: DescribeAccessPoint failed",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				driver := &Driver{
					endpoint:                 endpoint,
					cloud:                    mockCloud,
					mounter:                  mockMounter,
					gidAllocator:             NewGidAllocator(mockCloud),
					deleteAccessPointRootDir: true,
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil, errors.New("Describe Access Point failed"))
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatalf("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Fail to make directory for access point mount",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				driver := &Driver{
					endpoint:                 endpoint,
					cloud:                    mockCloud,
					mounter:                  mockMounter,
					gidAllocator:             NewGidAllocator(mockCloud),
					deleteAccessPointRootDir: true,
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				accessPoint := &cloud.AccessPoint{
					AccessPointId:      apId,
					FileSystemId:       fsId,
					AccessPointRootDir: "",
					CapacityGiB:        0,
				}

				ctx := context.Background()
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(errors.New("Failed to makeDir"))
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(accessPoint, nil)
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatal("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Fail to mount file system on directory for access point root directory removal",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				driver := &Driver{
					endpoint:                 endpoint,
					cloud:                    mockCloud,
					mounter:                  mockMounter,
					gidAllocator:             NewGidAllocator(mockCloud),
					deleteAccessPointRootDir: true,
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				accessPoint := &cloud.AccessPoint{
					AccessPointId:      apId,
					FileSystemId:       fsId,
					AccessPointRootDir: "",
					CapacityGiB:        0,
				}

				ctx := context.Background()
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("Failed to mount"))
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(accessPoint, nil)
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatal("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Fail to unmount file system after access point root directory removal",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				driver := &Driver{
					endpoint:                 endpoint,
					cloud:                    mockCloud,
					mounter:                  mockMounter,
					gidAllocator:             NewGidAllocator(mockCloud),
					deleteAccessPointRootDir: true,
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				accessPoint := &cloud.AccessPoint{
					AccessPointId:      apId,
					FileSystemId:       fsId,
					AccessPointRootDir: "",
					CapacityGiB:        0,
				}

				ctx := context.Background()
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(errors.New("Failed to unmount"))
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(accessPoint, nil)
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatal("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Access Point already deleted",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(cloud.ErrNotFound)
				_, err := driver.DeleteVolume(ctx, req)
				if err != nil {
					t.Fatalf("Delete Volume failed: %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: DeleteAccessPoint access denied",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(cloud.ErrAccessDenied)
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatal("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: DeleteVolume fails",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(errors.New("Delete Volume failed"))
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatal("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Access Point is missing in volume Id",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
				}

				req := &csi.DeleteVolumeRequest{
					VolumeId: "fs-abcd1234",
				}

				ctx := context.Background()
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatal("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Cannot assume role for x-account",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint:     endpoint,
					cloud:        mockCloud,
					gidAllocator: NewGidAllocator(mockCloud),
					tags:         parseTagsFromStr(""),
				}

				secrets := map[string]string{}
				secrets["awsRoleArn"] = "arn:aws:iam::1234567890:role/EFSCrossAccountRole"

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
					Secrets:  secrets,
				}

				ctx := context.Background()

				_, err := driver.DeleteVolume(ctx, req)

				if err == nil {
					t.Fatalf("DeleteVolume did not fail")
				}

				mockCtl.Finish()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestValidateVolumeCapabilities(t *testing.T) {
	var (
		endpoint       = "endpoint"
		volumeId       = "fs-abcd1234::fsap-abcd1234xyz987"
		stdVolCapValid = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			},
		}
		stdVolCapInvalid = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			},
		}
	)
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint: endpoint,
					cloud:    mockCloud,
				}

				req := &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId: volumeId,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCapValid,
					},
				}

				ctx := context.Background()
				res, err := driver.ValidateVolumeCapabilities(ctx, req)
				if err != nil {
					t.Fatalf("ValidateVolumeCapabilities failed: %v", err)
				}

				if res.Confirmed == nil {
					t.Fatalf("Capability is not supported")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Unsupported volume capability",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint: endpoint,
					cloud:    mockCloud,
				}

				req := &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId: volumeId,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCapInvalid,
					},
				}

				ctx := context.Background()
				res, err := driver.ValidateVolumeCapabilities(ctx, req)
				if err != nil {
					t.Fatalf("ValidateVolumeCapabilities failed: %v", err)
				}

				if res.Confirmed != nil {
					t.Fatal("ValidateVolumeCapabilities did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Volume Id is missing",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint: endpoint,
					cloud:    mockCloud,
				}

				req := &csi.ValidateVolumeCapabilitiesRequest{
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCapValid,
					},
				}

				ctx := context.Background()
				_, err := driver.ValidateVolumeCapabilities(ctx, req)
				if err == nil {
					t.Fatal("ValidateVolumeCapabilities did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Volume Capabilities is missing",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := &Driver{
					endpoint: endpoint,
					cloud:    mockCloud,
				}

				req := &csi.ValidateVolumeCapabilitiesRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				_, err := driver.ValidateVolumeCapabilities(ctx, req)
				if err == nil {
					t.Fatal("ValidateVolumeCapabilities did not fail")
				}
				mockCtl.Finish()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	var endpoint = "endpoint"
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)

	driver := &Driver{
		endpoint: endpoint,
		cloud:    mockCloud,
	}

	ctx := context.Background()
	_, err := driver.ControllerGetCapabilities(ctx, &csi.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("ControllerGetCapabilities failed: %v", err)
	}
}

func verifyPathWhenUUIDIncluded(pathToVerify string, expectedPathWithoutUUID string) bool {
	r := regexp.MustCompile("(.*)-([0-9A-fA-F]+-[0-9A-fA-F]+-[0-9A-fA-F]+-[0-9A-fA-F]+-[0-9A-fA-F]+$)")
	matches := r.FindStringSubmatch(pathToVerify)
	doesPathMatchWithUuid := matches[1] == expectedPathWithoutUUID
	_, err := uuid.Parse(matches[2])
	return err == nil && doesPathMatchWithUuid
}
