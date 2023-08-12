package driver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

func TestCreateVolume(t *testing.T) {
	var (
		endpoint                      = "endpoint"
		volumeName                    = "volumeName"
		fsId                          = "fs-abcd1234"
		apId                          = "fsap-abcd1234xyz987"
		apProvisioningVolumeId        = "fs-abcd1234::fsap-abcd1234xyz987"
		dirProvisioningVolumeId       = "fs-abcd1234:/dynamic/" + volumeName
		capacityRange           int64 = 5368709120
		stdVolCap                     = &csi.VolumeCapability{
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
			name: "Success: Normal flow, Access Point Provisioning",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil)

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != apProvisioningVolumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", apProvisioningVolumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow, Directory Provisioning",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)

				driver := buildDriver(endpoint, nil, "", mockMounter, false, false)

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: "efs-dir",
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						BasePath:         "/dynamic",
					},
				}

				ctx := context.Background()

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != dirProvisioningVolumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", apProvisioningVolumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow, no GID range specified",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil)

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != apProvisioningVolumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", apProvisioningVolumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Normal flow with invalid tags",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := buildDriver(endpoint, mockCloud, "cluster-efs", nil, false, false)

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
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil)

				res, err := driver.CreateVolume(ctx, req)

				if err != nil {
					t.Fatalf("CreateVolume failed: %v", err)
				}

				if res.Volume == nil {
					t.Fatal("Volume is nil")
				}

				if res.Volume.VolumeId != apProvisioningVolumeId {
					t.Fatalf("Volume Id mismatched. Expected: %v, Actual: %v", apProvisioningVolumeId, res.Volume.VolumeId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Volume name missing",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					Parameters: map[string]string{
						ProvisioningMode: "foobar",
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
			name: "Fail: Run out of GIDs",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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
						GidMax:           "1000",
						GidMin:           "1001",
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

				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil).AnyTimes()
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(accessPoint, nil).AnyTimes()

				var err error
				// Input grants 2 GIDS, third CreateVolume call should result in error
				for i := 0; i < 3; i++ {
					_, err = driver.CreateVolume(ctx, req)
				}

				if err == nil {
					t.Fatalf("CreateVolume did not fail")
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
		fsId                   = "fs-abcd1234"
		apId                   = "fsap-abcd1234xyz987"
		provisionedDir         = "/dynamic/newVolume"
		endpoint               = "endpoint"
		apProvisionedVolumeId  = fmt.Sprintf("%s::%s", fsId, apId)
		dirProvisionedVolumeId = fmt.Sprintf("%s:%s", fsId, provisionedDir)
	)

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success: Normal flow, Access Point Provisioning",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

				req := &csi.DeleteVolumeRequest{
					VolumeId: apProvisionedVolumeId,
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
			name: "Success: Normal flow, Directory Provisioning",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)

				driver := buildDriver(endpoint, mockCloud, "", mockMounter, false, true)

				req := &csi.DeleteVolumeRequest{
					VolumeId: dirProvisionedVolumeId,
				}

				ctx := context.Background()
				_, err := driver.DeleteVolume(ctx, req)
				if err != nil {
					t.Fatalf("Delete Volume failed: %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: Cannot parse details from volumeId",
			testFunc: func(t *testing.T) {
				driver := buildDriver(endpoint, nil, "", nil, false, true)

				req := &csi.DeleteVolumeRequest{
					VolumeId: "foobarbash",
				}

				ctx := context.Background()
				_, err := driver.DeleteVolume(ctx, req)
				if err != nil {
					t.Fatalf("Expected success but found: %v", err)
				}
			},
		},
		{
			name: "Fail: DeleteVolume fails if access point cannot be deleted",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				ctx := context.Background()
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(errors.New("Delete Volume failed"))

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

				req := &csi.DeleteVolumeRequest{
					VolumeId: apProvisionedVolumeId,
				}

				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatal("DeleteVolume did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: volumeId not provided",
			testFunc: func(t *testing.T) {
				driver := buildDriver(endpoint, nil, "", nil, false, true)

				req := &csi.DeleteVolumeRequest{}

				ctx := context.Background()
				_, err := driver.DeleteVolume(ctx, req)
				if err == nil {
					t.Fatal("Expected failure but operation succeeded")
				}
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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

				driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

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

	driver := buildDriver(endpoint, mockCloud, "", nil, false, false)

	ctx := context.Background()
	_, err := driver.ControllerGetCapabilities(ctx, &csi.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("ControllerGetCapabilities failed: %v", err)
	}
}

func buildDriver(endpoint string, cloud cloud.Cloud, tags string, mounter Mounter, deleteAccessPointRootDir bool, deleteProvisionedDir bool) *Driver {
	parsedTags := parseTagsFromStr(tags)

	driver := &Driver{
		endpoint:          endpoint,
		cloud:             cloud,
		provisioners:      getProvisioners(parsedTags, cloud, deleteAccessPointRootDir, mounter, &FakeOsClient{}, deleteProvisionedDir),
		tags:              parsedTags,
		mounter:           mounter,
		fsIdentityManager: NewFileSystemIdentityManager(),
	}
	return driver
}
