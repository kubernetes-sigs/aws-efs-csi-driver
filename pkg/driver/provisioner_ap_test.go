package driver

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

func TestAccessPointProvisioner_Provision(t *testing.T) {
	var (
		fsId                = "fs-abcd1234"
		volumeName          = "volumeName"
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
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Fail: File system does not exist",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				ctx := context.Background()
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(nil, cloud.ErrNotFound)

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
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud: mockCloud,
					},
					tags: map[string]string{},
				}

				_, err := apProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected Provision to fail but it didn't")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: DescribeFileSystem Access Denied",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				ctx := context.Background()
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(nil, cloud.ErrAccessDenied)

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
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud: mockCloud,
					},
					tags: map[string]string{},
				}

				_, err := apProv.Provision(ctx, req)
				if err == nil {
					t.Fatal("Expected Provision to fail but it didn't")
				}

				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Describe File system call fails",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				ctx := context.Background()
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("DescribeFileSystem failed"))

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
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud: mockCloud,
					},
					tags: map[string]string{},
				}

				_, err := apProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected Provision to fail but it didn't")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Create Access Point call fails",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}

				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(nil, errors.New("CreateAccessPoint call failed"))

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
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud: mockCloud,
					},
					tags: map[string]string{},
				}

				_, err := apProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected Provision to fail but it didn't")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: CreateAccessPoint Access Denied",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				ctx := context.Background()
				fileSystem := &cloud.FileSystem{
					FileSystemId: fsId,
				}
				mockCloud.EXPECT().DescribeFileSystem(gomock.Eq(ctx), gomock.Any()).Return(fileSystem, nil)
				mockCloud.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(nil, cloud.ErrAccessDenied)

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
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud: mockCloud,
					},
					tags: map[string]string{},
				}

				_, err := apProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected Provision to fail but it didn't")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Cannot assume role for x-account",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

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
						Uid:              "1000",
						Gid:              "1000",
					},
					Secrets: secrets,
				}

				ctx := context.Background()

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud: mockCloud,
					},
					tags: map[string]string{},
				}

				_, err := apProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected Provision to fail but it didn't")
				}

				mockCtl.Finish()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.testFunc)
	}
}

func TestAccessPointProvisioner_Delete(t *testing.T) {
	var (
		fsId     = "fs-abcd1234"
		apId     = "fsap-abcd1234xyz987"
		volumeId = fmt.Sprintf("%s::%s", fsId, apId)
	)

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success: Setting deleteAccessPointRootDir causes rootDir to be deleted",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				accessPoint := &cloud.AccessPoint{
					AccessPointId:      apId,
					FileSystemId:       fsId,
					AccessPointRootDir: "",
					CapacityGiB:        0,
				}

				dirPresent := mocks.NewMockFileInfo(
					"testFile",
					0,
					0755,
					time.Now(),
					true,
					nil,
				)

				ctx := context.Background()
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil)
				mockMounter.EXPECT().Stat(gomock.Any()).Return(dirPresent, nil)
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(accessPoint, nil)
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud:   mockCloud,
						mounter: mockMounter,
					},
					tags:                     map[string]string{},
					deleteAccessPointRootDir: true,
				}

				err := apProv.Delete(ctx, req)

				if err != nil {
					t.Fatalf("Expected Delete to succeed but it failed: %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Success: If AccessPoint does not exist success is returned as no work needs to be done",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil, cloud.ErrNotFound)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud:   mockCloud,
						mounter: mockMounter,
					},
					tags:                     map[string]string{},
					deleteAccessPointRootDir: true,
				}

				err := apProv.Delete(ctx, req)

				if err != nil {
					t.Fatalf("Expected Delete to succeed but it failed: %v", err)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Return error if AccessDenied error from AWS",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil, cloud.ErrAccessDenied)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud:   mockCloud,
						mounter: mockMounter,
					},
					tags:                     map[string]string{},
					deleteAccessPointRootDir: true,
				}

				err := apProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected Delete to fail but it succeeded")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Return error if DescribeAccessPoints failed",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(nil, errors.New("Describe Access Point failed"))
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud:   mockCloud,
						mounter: mockMounter,
					},
					tags:                     map[string]string{},
					deleteAccessPointRootDir: true,
				}

				err := apProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected Delete to fail but it succeeded")
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
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil)
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(accessPoint, nil)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud:   mockCloud,
						mounter: mockMounter,
					},
					tags:                     map[string]string{},
					deleteAccessPointRootDir: true,
				}

				err := apProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected Delete to fail but it succeeded")
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
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil).Times(2)
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(accessPoint, nil)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud:   mockCloud,
						mounter: mockMounter,
					},
					tags:                     map[string]string{},
					deleteAccessPointRootDir: true,
				}

				err := apProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected Delete to fail but it succeeded")
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

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				accessPoint := &cloud.AccessPoint{
					AccessPointId:      apId,
					FileSystemId:       fsId,
					AccessPointRootDir: "",
					CapacityGiB:        0,
				}

				dirPresent := mocks.NewMockFileInfo(
					"testFile",
					0,
					0755,
					time.Now(),
					true,
					nil,
				)

				ctx := context.Background()
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(errors.New("Failed to unmount"))
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil).Times(2)
				mockMounter.EXPECT().Stat(gomock.Any()).Return(dirPresent, nil)
				mockCloud.EXPECT().DescribeAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(accessPoint, nil)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud:   mockCloud,
						mounter: mockMounter,
					},
					tags:                     map[string]string{},
					deleteAccessPointRootDir: true,
				}

				err := apProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected Delete to fail but it succeeded")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: DeleteAccessPoint access denied",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				ctx := context.Background()
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Eq(apId)).Return(cloud.ErrAccessDenied)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud: mockCloud,
					},
					tags: map[string]string{},
				}

				err := apProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected Delete to fail but it succeeded")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Cannot assume role for x-account",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				secrets := map[string]string{}
				secrets["awsRoleArn"] = "arn:aws:iam::1234567890:role/EFSCrossAccountRole"

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
					Secrets:  secrets,
				}

				ctx := context.Background()
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil)

				apProv := AccessPointProvisioner{
					BaseProvisioner: BaseProvisioner{
						cloud:   mockCloud,
						mounter: mockMounter,
					},
					tags:                     map[string]string{},
					deleteAccessPointRootDir: true,
				}

				err := apProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected Delete to fail but it succeeded")
				}
				mockCtl.Finish()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.testFunc)
	}
}
