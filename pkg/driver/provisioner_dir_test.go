package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

const fileSystemId = "fs-123456789"

func TestDirectoryProvisioner_Provision(t *testing.T) {
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
			name: "Success: Check path created is sensible",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)

				ctx := context.Background()

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: DirectoryMode,
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						BasePath:         "/dynamic",
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					osClient: &FakeOsClient{},
				}

				volume, err := dProv.Provision(ctx, req)

				if err != nil {
					t.Fatalf("Expected provision call to succeed but failed: %v", err)
				}

				expectedVolumeId := fmt.Sprintf("%s:/dynamic/%s", fsId, req.Name)
				if volume.VolumeId != expectedVolumeId {
					t.Fatalf("Expected volumeId to be %s but was %s", expectedVolumeId, volume.VolumeId)
				}
			},
		},
		{
			name: "Fail: Return error for empty fsId",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)

				ctx := context.Background()

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: DirectoryMode,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
				}

				_, err := dProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("Expected InvalidArgument error but instead got %v", err)
				}
			},
		},
		{
			name: "Fail: Mounter cannot create target directory on node",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(
					io.ErrUnexpectedEOF)

				ctx := context.Background()

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: DirectoryMode,
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
				}

				_, err := dProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), io.ErrUnexpectedEOF) {
					t.Fatalf("Expected mount error but instead got %v", err)
				}
			},
		},
		{
			name: "Fail: Mounter cannot mount into target directory",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(
					mount.NewMountError(mount.HasFilesystemErrors, "Errors"))

				ctx := context.Background()

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: DirectoryMode,
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
				}

				_, err := dProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), mount.MountError{}) {
					t.Fatalf("Expected mount error but instead got %v", err)
				}
			},
		},
		{
			name: "Fail: Could not create directory after mounting root",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)

				ctx := context.Background()

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: DirectoryMode,
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						BasePath:         "/dynamic",
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					osClient: &BrokenOsClient{},
				}

				_, err := dProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), &os.PathError{}) {
					t.Fatalf("Expected path error but instead got %v", err)
				}
			},
		},
		{
			name: "Fail: Could not unmount root directory post creation",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(mount.NewMountError(mount.FilesystemMismatch, "Error"))

				ctx := context.Background()

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: DirectoryMode,
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						BasePath:         "/dynamic",
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					osClient: &FakeOsClient{},
				}

				_, err := dProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), mount.MountError{}) {
					t.Fatalf("Expected mount error but instead got %v", err)
				}
			},
		},
		{
			name: "Fail: Could not delete target directory once unmounted",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)

				ctx := context.Background()

				req := &csi.CreateVolumeRequest{
					Name: volumeName,
					VolumeCapabilities: []*csi.VolumeCapability{
						stdVolCap,
					},
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: capacityRange,
					},
					Parameters: map[string]string{
						ProvisioningMode: DirectoryMode,
						FsId:             fsId,
						GidMin:           "1000",
						GidMax:           "2000",
						DirectoryPerms:   "777",
						BasePath:         "/dynamic",
						Uid:              "1000",
						Gid:              "1000",
					},
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					osClient: &BrokenOsClient{},
				}

				_, err := dProv.Provision(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), &os.PathError{}) {
					t.Fatalf("Expected mount error but instead got %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.testFunc)
	}
}

func TestDirectoryProvisioner_Delete(t *testing.T) {
	var (
		fsId     = "fs-abcd1234"
		volumeId = fmt.Sprintf("%s:%s", fsId, "/dynamic/newDir")
	)

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success: If retain directory is set nothing happens",
			testFunc: func(t *testing.T) {
				ctx := context.Background()

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				dProv := DirectoryProvisioner{
					deleteProvisionedDir: false,
				}

				err := dProv.Delete(ctx, req)

				if err != nil {
					t.Fatalf("Expected success but found %v", err)
				}
			},
		},
		{
			name: "Success: If not retaining directory folder and contents are deleted",
			testFunc: func(t *testing.T) {
				ctx := context.Background()
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					deleteProvisionedDir: true,
					osClient:             &FakeOsClient{},
				}

				err := dProv.Delete(ctx, req)

				if err != nil {
					t.Fatalf("Expected success but found %v", err)
				}
			},
		},
		{
			name: "Fail: Mounter cannot create target directory on node",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(
					io.ErrUnexpectedEOF)

				ctx := context.Background()

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					deleteProvisionedDir: true,
				}

				err := dProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), io.ErrUnexpectedEOF) {
					t.Fatalf("Expected mount error but instead got %v", err)
				}
			},
		},
		{
			name: "Fail: Cannot delete contents of provisioned directory",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)

				ctx := context.Background()

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					deleteProvisionedDir: true,
					osClient:             &BrokenOsClient{},
				}

				err := dProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), &os.PathError{}) {
					t.Fatalf("Expected path error but instead got %v", err)
				}
			},
		},
		{
			name: "Fail: Cannot unmount directory after contents have been deleted",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(mount.NewMountError(mount.HasFilesystemErrors, "Errors"))

				ctx := context.Background()

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					deleteProvisionedDir: true,
					osClient:             &FakeOsClient{},
				}

				err := dProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), mount.MountError{}) {
					t.Fatalf("Expected mount error but instead got %v", err)
				}
			},
		},
		{
			name: "Fail: Cannot delete temporary directory after unmount",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtl)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().Mount(fsId, gomock.Any(), "efs", gomock.Any()).Return(nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)

				ctx := context.Background()

				req := &csi.DeleteVolumeRequest{
					VolumeId: volumeId,
				}

				dProv := DirectoryProvisioner{
					BaseProvisioner: BaseProvisioner{
						mounter: mockMounter,
					},
					deleteProvisionedDir: true,
					osClient:             &BrokenOsClient{},
				}

				err := dProv.Delete(ctx, req)

				if err == nil {
					t.Fatal("Expected error but found none")
				}
				if status.Code(err) != codes.Internal && errors.Is(errors.Unwrap(err), &os.PathError{}) {
					t.Fatalf("Expected path error but instead got %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.testFunc)
	}
}

func TestDirectoryProvisioner_GetMountOptions_NoRoleArnGivesStandardOptions(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)
	ctx := context.Background()
	expectedOptions := []string{"tls", "iam"}

	options, _ := getMountOptions(ctx, mockCloud, fileSystemId, "", false)

	if !reflect.DeepEqual(options, expectedOptions) {
		t.Fatalf("Expected returned options to be %v but was %v", expectedOptions, options)
	}
}

func TestDirectoryProvisioner_GetMountOptions_RoleArnAddsMountTargetIp(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)
	fakeMountTarget := cloud.MountTarget{
		AZName:        "foo",
		AZId:          "",
		MountTargetId: "",
		IPAddress:     "8.8.8.8",
	}
	ctx := context.Background()
	mockCloud.EXPECT().DescribeMountTargets(ctx, fileSystemId, "").Return(&fakeMountTarget, nil)

	expectedOptions := []string{"tls", "iam", MountTargetIp + "=" + fakeMountTarget.IPAddress}

	options, _ := getMountOptions(ctx, mockCloud, fileSystemId, "roleArn", false)

	if !reflect.DeepEqual(options, expectedOptions) {
		t.Fatalf("Expected returned options to be %v but was %v", expectedOptions, options)
	}
}

func TestDirectoryProvisioner_GetMountOptions_RoleArnCross(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)
	ctx := context.Background()

	expectedOptions := []string{"tls", "iam", CrossAccount}

	options, _ := getMountOptions(ctx, mockCloud, fileSystemId, "roleArn", true)

	if !reflect.DeepEqual(options, expectedOptions) {
		t.Fatalf("Expected returned options to be %v but was %v", expectedOptions, options)
	}
}
