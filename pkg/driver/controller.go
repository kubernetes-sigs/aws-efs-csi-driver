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

package driver

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

const (
	AccessPointMode     = "efs-ap"
	AzName              = "az"
	BasePath            = "basePath"
	DefaultGidMin       = 50000
	DefaultGidMax       = 7000000
	DefaultTagKey       = "efs.csi.aws.com/cluster"
	DefaultTagValue     = "true"
	DirectoryPerms      = "directoryPerms"
	FsId                = "fileSystemId"
	Gid                 = "gid"
	GidMin              = "gidRangeStart"
	GidMax              = "gidRangeEnd"
	MountTargetIp       = "mounttargetip"
	ProvisioningMode    = "provisioningMode"
	RoleArn             = "awsRoleArn"
	TempMountPathPrefix = "/var/lib/csi/pv"
	Uid                 = "uid"
)

var (
	// controllerCaps represents the capability of controller service
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	}
)

func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(4).Infof("CreateVolume: called with args %+v", *req)
	volName := req.GetName()
	if volName == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume name not provided")
	}

	// Volume size is required to match PV to PVC by k8s.
	// Volume size is not consumed by EFS for any purposes.
	volSize := req.GetCapacityRange().GetRequiredBytes()

	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}

	if !d.isValidVolumeCapabilities(volCaps) {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not supported")
	}

	var (
		azName           string
		basePath         string
		err              error
		gid              int
		gidMin           int
		gidMax           int
		localCloud       cloud.Cloud
		provisioningMode string
		roleArn          string
		uid              int
	)

	//Parse parameters
	volumeParams := req.GetParameters()
	if value, ok := volumeParams[ProvisioningMode]; ok {
		provisioningMode = value
		//TODO: Add FS provisioning mode check when implemented
		if provisioningMode != AccessPointMode {
			errStr := "Provisioning mode " + provisioningMode + " is not supported. Only Access point provisioning: 'efs-ap' is supported"
			return nil, status.Error(codes.InvalidArgument, errStr)
		}
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "Missing %v parameter", ProvisioningMode)
	}

	// Create tags
	tags := map[string]string{
		DefaultTagKey: DefaultTagValue,
	}

	// Append input tags to default tag
	if len(d.tags) != 0 {
		for k, v := range d.tags {
			tags[k] = v
		}
	}

	accessPointsOptions := &cloud.AccessPointOptions{
		CapacityGiB: volSize,
		Tags:        tags,
	}

	if value, ok := volumeParams[FsId]; ok {
		if strings.TrimSpace(value) == "" {
			return nil, status.Errorf(codes.InvalidArgument, "Parameter %v cannot be empty", FsId)
		}
		accessPointsOptions.FileSystemId = value
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "Missing %v parameter", FsId)
	}

	uid = -1
	if value, ok := volumeParams[Uid]; ok {
		uid, err = strconv.Atoi(value)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Failed to parse invalid %v: %v", Uid, err)
		}
		if uid < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "%v must be greater or equal than 0", Uid)
		}
	}

	gid = -1
	if value, ok := volumeParams[Gid]; ok {
		gid, err = strconv.Atoi(value)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Failed to parse invalid %v: %v", Gid, err)
		}
		if uid < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "%v must be greater or equal than 0", Gid)
		}
	}

	if value, ok := volumeParams[GidMin]; ok {
		gidMin, err = strconv.Atoi(value)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Failed to parse invalid %v: %v", GidMin, err)
		}
		if gidMin <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "%v must be greater than 0", GidMin)
		}
	}

	if value, ok := volumeParams[GidMax]; ok {
		// Ensure GID min is provided with GID max
		if gidMin == 0 {
			return nil, status.Errorf(codes.InvalidArgument, "Missing %v parameter", GidMin)
		}
		gidMax, err = strconv.Atoi(value)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Failed to parse invalid %v: %v", GidMax, err)
		}
		if gidMax <= gidMin {
			return nil, status.Errorf(codes.InvalidArgument, "%v must be greater than %v", GidMax, GidMin)
		}
	} else {
		// Ensure GID max is provided with GID min
		if gidMin != 0 {
			return nil, status.Errorf(codes.InvalidArgument, "Missing %v parameter", GidMax)
		}
	}

	// Assign default GID ranges if not provided
	if gidMin == 0 && gidMax == 0 {
		gidMin = DefaultGidMin
		gidMax = DefaultGidMax
	}

	if value, ok := volumeParams[DirectoryPerms]; ok {
		accessPointsOptions.DirectoryPerms = value
	}

	if value, ok := volumeParams[BasePath]; ok {
		basePath = value
	}

	// Storage class parameter `az` will be used to fetch preferred mount target for cross account mount.
	// If the `az` storage class parameter is not provided, a random mount target will be picked for mounting.
	// This storage class parameter different from `az` mount option provided by efs-utils https://github.com/aws/efs-utils/blob/v1.31.1/src/mount_efs/__init__.py#L195
	// The `az` mount option provided by efs-utils is used for cross az mount or to provide az of efs one zone file system mount within the same aws-account.
	// To make use of the `az` mount option, add it under storage class's `mountOptions` section. https://kubernetes.io/docs/concepts/storage/storage-classes/#mount-options
	if value, ok := volumeParams[AzName]; ok {
		azName = value
	}

	localCloud, roleArn, err = getCloud(req.GetSecrets(), d)
	if err != nil {
		return nil, err
	}

	// Check if file system exists. Describe FS handles appropriate error codes
	if _, err = localCloud.DescribeFileSystem(ctx, accessPointsOptions.FileSystemId); err != nil {
		if err == cloud.ErrAccessDenied {
			return nil, status.Errorf(codes.Unauthenticated, "Access Denied. Please ensure you have the right AWS permissions: %v", err)
		}
		if err == cloud.ErrNotFound {
			return nil, status.Errorf(codes.InvalidArgument, "File System does not exist: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "Failed to fetch File System info: %v", err)
	}

	var allocatedGid int
	if uid == -1 || gid == -1 {
		allocatedGid, err = d.gidAllocator.getNextGid(accessPointsOptions.FileSystemId, gidMin, gidMax)
		if err != nil {
			return nil, err
		}
	}
	if uid == -1 {
		uid = allocatedGid
	}
	if gid == -1 {
		gid = allocatedGid
	}

	rootDirName := volName
	rootDir := basePath + "/" + rootDirName

	accessPointsOptions.Uid = int64(uid)
	accessPointsOptions.Gid = int64(gid)
	accessPointsOptions.DirectoryPath = rootDir

	accessPointId, err := localCloud.CreateAccessPoint(ctx, volName, accessPointsOptions)
	if err != nil {
		d.gidAllocator.releaseGid(accessPointsOptions.FileSystemId, gid)
		if err == cloud.ErrAccessDenied {
			return nil, status.Errorf(codes.Unauthenticated, "Access Denied. Please ensure you have the right AWS permissions: %v", err)
		}
		if err == cloud.ErrAlreadyExists {
			return nil, status.Errorf(codes.AlreadyExists, "Access Point already exists")
		}
		return nil, status.Errorf(codes.Internal, "Failed to create Access point in File System %v : %v", accessPointsOptions.FileSystemId, err)
	}

	volContext := map[string]string{}

	// Fetch mount target Ip for cross-account mount
	if roleArn != "" {
		mountTarget, err := localCloud.DescribeMountTargets(ctx, accessPointsOptions.FileSystemId, azName)
		if err != nil {
			klog.Warningf("Failed to describe mount targets for file system %v. Skip using `mounttargetip` mount option: %v", accessPointsOptions.FileSystemId, err)
		} else {
			volContext[MountTargetIp] = mountTarget.IPAddress
		}
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: volSize,
			VolumeId:      accessPointsOptions.FileSystemId + "::" + accessPointId.AccessPointId,
			VolumeContext: volContext,
		},
	}, nil
}

func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	var (
		localCloud cloud.Cloud
		roleArn    string
		err        error
	)

	localCloud, roleArn, err = getCloud(req.GetSecrets(), d)
	if err != nil {
		return nil, err
	}

	klog.V(4).Infof("DeleteVolume: called with args %+v", *req)
	volId := req.GetVolumeId()
	if volId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	fileSystemId, _, accessPointId, err := parseVolumeId(volId)
	if err != nil {
		//Returning success for an invalid volume ID. See here - https://github.com/kubernetes-csi/csi-test/blame/5deb83d58fea909b2895731d43e32400380aae3c/pkg/sanity/controller.go#L733
		klog.V(5).Infof("DeleteVolume: Failed to parse volumeID: %v, err: %v, returning success", volId, err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	//TODO: Add Delete File System when FS provisioning is implemented
	if accessPointId != "" {

		// Delete access point root directory if delete-access-point-root-dir is set.
		if d.deleteAccessPointRootDir {
			// Check if Access point exists.
			// If access point exists, retrieve its root directory and delete it/
			accessPoint, err := localCloud.DescribeAccessPoint(ctx, accessPointId)
			if err != nil {
				if err == cloud.ErrAccessDenied {
					return nil, status.Errorf(codes.Unauthenticated, "Access Denied. Please ensure you have the right AWS permissions: %v", err)
				}
				if err == cloud.ErrNotFound {
					klog.V(5).Infof("DeleteVolume: Access Point %v not found, returning success", accessPointId)
					return &csi.DeleteVolumeResponse{}, nil
				}
				return nil, status.Errorf(codes.Internal, "Could not get describe Access Point: %v , error: %v", accessPointId, err)
			}

			//Mount File System at it root and delete access point root directory
			mountOptions := []string{"tls", "iam"}
			if roleArn != "" {
				mountTarget, err := localCloud.DescribeMountTargets(ctx, fileSystemId, "")

				if err == nil {
					mountOptions = append(mountOptions, MountTargetIp+"="+mountTarget.IPAddress)
				} else {
					klog.Warningf("Failed to describe mount targets for file system %v. Skip using `mounttargetip` mount option: %v", fileSystemId, err)
				}
			}

			target := TempMountPathPrefix + "/" + accessPointId
			if err := d.mounter.MakeDir(target); err != nil {
				return nil, status.Errorf(codes.Internal, "Could not create dir %q: %v", target, err)
			}
			if err := d.mounter.Mount(fileSystemId, target, "efs", mountOptions); err != nil {
				os.Remove(target)
				return nil, status.Errorf(codes.Internal, "Could not mount %q at %q: %v", fileSystemId, target, err)
			}
			err = os.RemoveAll(target + accessPoint.AccessPointRootDir)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not delete access point root directory %q: %v", accessPoint.AccessPointRootDir, err)
			}
			err = d.mounter.Unmount(target)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not unmount %q: %v", target, err)
			}
			err = os.RemoveAll(target)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not delete %q: %v", target, err)
			}
		}

		// Delete access point
		if err = localCloud.DeleteAccessPoint(ctx, accessPointId); err != nil {
			if err == cloud.ErrAccessDenied {
				return nil, status.Errorf(codes.Unauthenticated, "Access Denied. Please ensure you have the right AWS permissions: %v", err)
			}
			if err == cloud.ErrNotFound {
				klog.V(5).Infof("DeleteVolume: Access Point not found, returning success")
				return &csi.DeleteVolumeResponse{}, nil
			}
			return nil, status.Errorf(codes.Internal, "Failed to Delete volume %v: %v", volId, err)
		}
	} else {
		return nil, status.Errorf(codes.NotFound, "Failed to find access point for volume: %v", volId)
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.V(4).Infof("ValidateVolumeCapabilities: called with args %+v", *req)
	volId := req.GetVolumeId()
	if volId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}

	_, _, _, err := parseVolumeId(volId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Volume not found, err: %v", err)
	}

	var confirmed *csi.ValidateVolumeCapabilitiesResponse_Confirmed
	if d.isValidVolumeCapabilities(volCaps) {
		confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: volCaps}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: confirmed,
	}, nil
}

func (d *Driver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(4).Infof("ControllerGetCapabilities: called with args %+v", *req)
	var caps []*csi.ControllerServiceCapability
	for _, cap := range controllerCaps {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func getCloud(secrets map[string]string, driver *Driver) (cloud.Cloud, string, error) {

	var localCloud cloud.Cloud
	var roleArn string
	var err error

	// Fetch aws role ARN for cross account mount from CSI secrets. Link to CSI secrets below
	// https://kubernetes-csi.github.io/docs/secrets-and-credentials.html#csi-operation-secrets
	if value, ok := secrets[RoleArn]; ok {
		roleArn = value
	}

	if roleArn != "" {
		localCloud, err = cloud.NewCloudWithRole(roleArn)
		if err != nil {
			return nil, "", status.Errorf(codes.Unauthenticated, "Unable to initialize aws cloud: %v. Please verify role has the correct AWS permissions for cross account mount", err)
		}
	} else {
		localCloud = driver.cloud
	}

	return localCloud, roleArn, nil
}
