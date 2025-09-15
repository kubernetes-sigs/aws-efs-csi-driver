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
	"crypto/sha256"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	AccessPointMode       = "efs-ap"
	AzName                = "az"
	BasePath              = "basePath"
	DefaultGidMin         = int64(50000)
	DefaultGidMax         = DefaultGidMin + cloud.AccessPointPerFsLimit
	DefaultTagKey         = "efs.csi.aws.com/cluster"
	DefaultTagValue       = "true"
	DirectoryPerms        = "directoryPerms"
	EnsureUniqueDirectory = "ensureUniqueDirectory"
	ExternalId            = "externalId"
	FsId                  = "fileSystemId"
	Gid                   = "gid"
	GidMin                = "gidRangeStart"
	GidMax                = "gidRangeEnd"
	MountTargetIp         = "mounttargetip"
	ProvisioningMode      = "provisioningMode"
	PvName                = "csi.storage.k8s.io/pv/name"
	PvcName               = "csi.storage.k8s.io/pvc/name"
	PvcNamespace          = "csi.storage.k8s.io/pvc/namespace"
	RoleArn               = "awsRoleArn"
	SubPathPattern        = "subPathPattern"
	TempMountPathPrefix   = "/var/lib/csi/pv"
	Uid                   = "uid"
	ReuseAccessPointKey   = "reuseAccessPoint"
	PvcNameKey            = "csi.storage.k8s.io/pvc/name"
	CrossAccount          = "crossaccount"
	ApLockWaitTimeSec     = 3
)

var (
	// controllerCaps represents the capability of controller service
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	}
	// subPathPatternComponents shows the elements that we allow to be in the construction of the root directory
	// of the access point, as well as the values we need to extract them from the Volume Parameters.
	subPathPatternComponents = map[string]string{
		".PVC.name":      PvcName,
		".PVC.namespace": PvcNamespace,
		".PV.name":       PvName,
	}
)

func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(4).Infof("CreateVolume: called with args %+v", util.SanitizeRequest(*req))

	var reuseAccessPoint bool
	var err error
	volumeParams := req.GetParameters()
	volName := req.GetName()
	clientToken := volName

	// if true, then use sha256 hash of pvcName as clientToken instead of PVC Id
	// This allows users to reconnect to the same AP from different k8s cluster
	if reuseAccessPointStr, ok := volumeParams[ReuseAccessPointKey]; ok {
		reuseAccessPoint, err = strconv.ParseBool(reuseAccessPointStr)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "Invalid value for reuseAccessPoint parameter")
		}
		if reuseAccessPoint {
			clientToken = get64LenHash(volumeParams[PvcNameKey])
			klog.V(5).Infof("Client token : %s", clientToken)
		}
	}
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

	if err := d.isValidVolumeCapabilities(volCaps); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume capabilities not supported: %s", err))
	}
	if err := d.validateFStype(volCaps); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume fstype not supported: %s", err))
	}

	var (
		azName                 string
		basePath               string
		gid                    int64
		gidMin                 int64
		gidMax                 int64
		localCloud             cloud.Cloud
		provisioningMode       string
		roleArn                string
		uid                    int64
		crossAccountDNSEnabled bool
	)

	//Parse parameters
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

	accessPointsOptions := &cloud.AccessPointOptions{
		CapacityGiB: volSize,
	}

	if value, ok := volumeParams[FsId]; ok {
		if strings.TrimSpace(value) == "" {
			return nil, status.Errorf(codes.InvalidArgument, "Parameter %v cannot be empty", FsId)
		}
		accessPointsOptions.FileSystemId = value
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "Missing %v parameter", FsId)
	}

	localCloud, roleArn, crossAccountDNSEnabled, err = getCloud(req.GetSecrets(), d)
	if err != nil {
		return nil, err
	}

	var accessPoint *cloud.AccessPoint
	//if reuseAccessPoint is true, check for AP with same Root Directory exists in efs
	// if found reuse that AP
	if reuseAccessPoint {
		existingAP, err := localCloud.FindAccessPointByClientToken(ctx, clientToken, accessPointsOptions.FileSystemId)
		if err != nil {
			return nil, fmt.Errorf("failed to find access point: %v", err)
		}
		if existingAP != nil {
			//AP path already exists
			klog.V(2).Infof("Existing AccessPoint found : %+v", existingAP)
			accessPoint = &cloud.AccessPoint{
				AccessPointId: existingAP.AccessPointId,
				FileSystemId:  existingAP.FileSystemId,
				CapacityGiB:   accessPointsOptions.CapacityGiB,
			}

			// Take the lock to prevent this access point from being deleted while creating volume
			if d.lockManager.lockMutex(accessPoint.AccessPointId, ApLockWaitTimeSec*time.Second) {
				defer d.lockManager.unlockMutex(accessPoint.AccessPointId)
			} else {
				return nil, status.Errorf(codes.Internal, "Could not take the lock for existing access point: %v", accessPoint.AccessPointId)
			}
		}
	}

	if accessPoint == nil {
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

		accessPointsOptions.Tags = tags

		uid = -1
		if value, ok := volumeParams[Uid]; ok {
			uid, err = strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "Failed to parse invalid %v: %v", Uid, err)
			}
			if uid < 0 {
				return nil, status.Errorf(codes.InvalidArgument, "%v must be greater or equal than 0", Uid)
			}
		}

		gid = -1
		if value, ok := volumeParams[Gid]; ok {
			gid, err = strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "Failed to parse invalid %v: %v", Gid, err)
			}
			if gid < 0 {
				return nil, status.Errorf(codes.InvalidArgument, "%v must be greater or equal than 0", Gid)
			}
		}

		if value, ok := volumeParams[GidMin]; ok {
			gidMin, err = strconv.ParseInt(value, 10, 64)
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
			gidMax, err = strconv.ParseInt(value, 10, 64)
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

		// Storage class parameter `az` will be used to fetch preferred mount target for cross account mount.
		// If the `az` storage class parameter is not provided, a random mount target will be picked for mounting.
		// This storage class parameter different from `az` mount option provided by efs-utils https://github.com/aws/efs-utils/blob/v1.31.1/src/mount_efs/__init__.py#L195
		// The `az` mount option provided by efs-utils is used for cross az mount or to provide az of efs one zone file system mount within the same aws-account.
		// To make use of the `az` mount option, add it under storage class's `mountOptions` section. https://kubernetes.io/docs/concepts/storage/storage-classes/#mount-options
		if value, ok := volumeParams[AzName]; ok {
			azName = value
		}

		// Check if file system exists. Describe FS or List APs handle appropriate error codes
		// With dynamic uid/gid provisioning we can save a call to describe FS, as list APs fails if FS ID does not exist
		var accessPoints []*cloud.AccessPoint
		if uid == -1 || gid == -1 {
			accessPoints, err = localCloud.ListAccessPoints(ctx, accessPointsOptions.FileSystemId)
		} else {
			_, err = localCloud.DescribeFileSystem(ctx, accessPointsOptions.FileSystemId)
		}
		if err != nil {
			if err == cloud.ErrAccessDenied {
				return nil, status.Errorf(codes.Unauthenticated, "Access Denied. Please ensure you have the right AWS permissions: %v", err)
			}
			if err == cloud.ErrNotFound {
				return nil, status.Errorf(codes.InvalidArgument, "File System does not exist: %v", err)
			}
			return nil, status.Errorf(codes.Internal, "Failed to fetch Access Points or Describe File System: %v", err)
		}

		var allocatedGid int64
		if uid == -1 || gid == -1 {
			allocatedGid, err = d.gidAllocator.getNextGid(accessPointsOptions.FileSystemId, accessPoints, gidMin, gidMax)
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

		if value, ok := volumeParams[BasePath]; ok {
			basePath = value
		}

		rootDirName := volName
		// Check if a custom structure should be imposed on the access point directory
		if value, ok := volumeParams[SubPathPattern]; ok {
			// Try and construct the root directory and check it only contains supported components
			val, err := interpolateRootDirectoryName(value, volumeParams)
			if err == nil {
				klog.Infof("Using user-specified structure for access point directory.")
				rootDirName = val
				if value, ok := volumeParams[EnsureUniqueDirectory]; ok {
					if ensureUniqueDirectory, err := strconv.ParseBool(value); !ensureUniqueDirectory && err == nil {
						klog.Infof("Not appending PVC UID to path.")
					} else {
						klog.Infof("Appending PVC UID to path.")
						rootDirName = fmt.Sprintf("%s-%s", val, uuid.New().String())
					}
				} else {
					klog.Infof("Appending PVC UID to path.")
					rootDirName = fmt.Sprintf("%s-%s", val, uuid.New().String())
				}
			} else {
				return nil, err
			}
		} else {
			klog.Infof("Using PV name for access point directory.")
		}

		rootDir := path.Join("/", basePath, rootDirName)
		if ok, err := validateEfsPathRequirements(rootDir); !ok {
			return nil, err
		}
		klog.Infof("Using %v as the access point directory.", rootDir)

		accessPointsOptions.Uid = uid
		accessPointsOptions.Gid = gid
		accessPointsOptions.DirectoryPath = rootDir

		accessPoint, err = localCloud.CreateAccessPoint(ctx, clientToken, accessPointsOptions)
		if err != nil {
			if err == cloud.ErrAccessDenied {
				return nil, status.Errorf(codes.Unauthenticated, "Access Denied. Please ensure you have the right AWS permissions: %v", err)
			} else if err == cloud.ErrAlreadyExists {
				klog.V(4).Infof("Access point already exists for client token %s. Retrieving existing access point details.", clientToken)
				existingAccessPoint, err := localCloud.FindAccessPointByClientToken(ctx, clientToken, accessPointsOptions.FileSystemId)
				if err != nil {
					return nil, fmt.Errorf("Error attempting to retrieve existing access point for client token %s: %v", clientToken, err)
				}
				if existingAccessPoint == nil {
					return nil, fmt.Errorf("No access point for client token %s was returned: %v", clientToken, err)
				}
				err = validateExistingAccessPoint(existingAccessPoint, basePath, gidMin, gidMax)
				if err != nil {
					return nil, status.Errorf(codes.AlreadyExists, "Invalid existing access point: %v", err)
				}
				accessPoint = existingAccessPoint
			} else {
				return nil, status.Errorf(codes.Internal, "Failed to create Access point in File System %v : %v", accessPointsOptions.FileSystemId, err)
			}
		}

		// Lock on the new access point to prevent accidental deletion before creation is done
		if d.lockManager.lockMutex(accessPoint.AccessPointId, ApLockWaitTimeSec*time.Second) {
			defer d.lockManager.unlockMutex(accessPoint.AccessPointId)
		} else {
			return nil, status.Errorf(codes.Internal, "Could not take the lock after creating access point: %v", accessPoint.AccessPointId)
		}
	}

	volContext := map[string]string{}

	// Enable cross-account dns resolution or fetch mount target Ip for cross-account mount
	if roleArn != "" {
		if crossAccountDNSEnabled {
			// This option indicates the customer would like to use DNS to resolve
			// the cross-account mount target ip address (in order to mount to
			// the same AZ-ID as the client instance); mounttargetip should
			// not be used as a mount option in this case.
			volContext[CrossAccount] = strconv.FormatBool(true)
		} else {
			mountTarget, err := localCloud.DescribeMountTargets(ctx, accessPointsOptions.FileSystemId, azName)
			if err != nil {
				klog.Warningf("Failed to describe mount targets for file system %v. Skip using `mounttargetip` mount option: %v", accessPointsOptions.FileSystemId, err)
			} else {
				volContext[MountTargetIp] = mountTarget.IPAddress
			}

		}
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: volSize,
			VolumeId:      accessPointsOptions.FileSystemId + "::" + accessPoint.AccessPointId,
			VolumeContext: volContext,
		},
	}, nil
}

func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	var (
		localCloud             cloud.Cloud
		roleArn                string
		crossAccountDNSEnabled bool
		err                    error
	)

	localCloud, roleArn, crossAccountDNSEnabled, err = getCloud(req.GetSecrets(), d)
	if err != nil {
		return nil, err
	}

	klog.V(4).Infof("DeleteVolume: called with args %+v", util.SanitizeRequest(*req))
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

	if accessPointId == "" {
		klog.V(5).Infof("DeleteVolume: No Access Point for volume %v, returning success", volId)
		return &csi.DeleteVolumeResponse{}, nil
	}

	// Lock on the access point ID to ensure a retry won't race with the in-progress deletion
	if d.lockManager.lockMutex(accessPointId, ApLockWaitTimeSec*time.Second) {
		defer d.lockManager.unlockMutex(accessPointId)
	} else {
		return nil, status.Errorf(codes.Internal, "Could not take the lock to delete access point: %v", accessPointId)
	}

	//TODO: Add Delete File System when FS provisioning is implemented
	// Delete access point root directory if delete-access-point-root-dir is set.
	if d.deleteAccessPointRootDir {
		fsRoot := TempMountPathPrefix + "/" + accessPointId
		deleteCompleted := false

		// Ensure the volume is cleaned up properly in case of an incomplete deletion
		defer func() {
			if !deleteCompleted {
				// Check if the FS is still mounted
				isNotMounted, err := d.mounter.IsLikelyNotMountPoint(fsRoot)
				if err != nil {
					return // Skip cleanup, we can't verify mount status
				}

				if !isNotMounted {
					if err := d.mounter.Unmount(fsRoot); err != nil {
						klog.Warningf("Failed to unmount %v: %v", fsRoot, err)
						return // Don't remove any data if the unmount fails
					}
				}

				// Only try folder removal if the unmount succeeded or wasn't mounted
				// If the directory already doesn't exist it will be treated as success
				if err := os.Remove(fsRoot); err != nil && !os.IsNotExist(err) {
					klog.Warningf("Failed to remove %v: %v", fsRoot, err)
				}
			}
		}()

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
			if crossAccountDNSEnabled {
				// Connect via dns rather than mounttargetip
				mountOptions = append(mountOptions, CrossAccount)
			} else {
				mountTarget, err := localCloud.DescribeMountTargets(ctx, fileSystemId, "")
				if err == nil {
					mountOptions = append(mountOptions, MountTargetIp+"="+mountTarget.IPAddress)
				} else {
					klog.Warningf("Failed to describe mount targets for file system %v. Skip using `mounttargetip` mount option: %v", fileSystemId, err)
				}
			}
		}

		// Create the target directory, This won't fail if it already exists
		if err := d.mounter.MakeDir(fsRoot); err != nil {
			return nil, status.Errorf(codes.Internal, "Could not create dir %q: %v", fsRoot, err)
		}

		// Only attempt to mount the target filesystem if its not already mounted
		isNotMounted, err := d.mounter.IsLikelyNotMountPoint(fsRoot)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not check if %q is mounted: %v", fsRoot, err)
		}
		if isNotMounted {
			if err := d.mounter.Mount(fileSystemId, fsRoot, "efs", mountOptions); err != nil {
				return nil, status.Errorf(codes.Internal, "Could not mount %q at %q: %v", fileSystemId, fsRoot, err)
			}
		}

		// Before removing, ensure the removal path exists and is a directory
		apRootPath := fsRoot + accessPoint.AccessPointRootDir
		if pathInfo, err := d.mounter.Stat(apRootPath); err == nil && !os.IsNotExist(err) && pathInfo.IsDir() {
			err = os.RemoveAll(apRootPath)
		}
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not delete access point root directory %q: %v", accessPoint.AccessPointRootDir, err)
		}
		err = d.mounter.Unmount(fsRoot)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not unmount %q: %v", fsRoot, err)
		}
		err = os.Remove(fsRoot)
		if err != nil && !os.IsNotExist(err) {
			return nil, status.Errorf(codes.Internal, "Could not delete %q: %v", fsRoot, err)
		}

		//Mark the delete as complete, Nothing needs cleanup in the deferred function
		deleteCompleted = true
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

	return &csi.DeleteVolumeResponse{}, nil
}

func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.V(4).Infof("ValidateVolumeCapabilities: called with args %+v", util.SanitizeRequest(*req))
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
	if err := d.isValidVolumeCapabilities(volCaps); err == nil {
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
	klog.V(4).Infof("ControllerGetCapabilities: called with args %+v", util.SanitizeRequest(*req))
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

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func getCloud(secrets map[string]string, driver *Driver) (cloud.Cloud, string, bool, error) {

	var localCloud cloud.Cloud
	var roleArn string
	var externalId string
	var crossAccountDNSEnabled bool
	var err error

	// Fetch aws role ARN for cross account mount from CSI secrets. Link to CSI secrets below
	// https://kubernetes-csi.github.io/docs/secrets-and-credentials.html#csi-operation-secrets
	if value, ok := secrets[RoleArn]; ok {
		roleArn = value
	}
	if value, ok := secrets[ExternalId]; ok {
		externalId = value
	}

	if value, ok := secrets[CrossAccount]; ok {
		crossAccountDNSEnabled, err = strconv.ParseBool(value)
		if err != nil {
			return nil, "", false, status.Error(codes.InvalidArgument, "crossaccount parameter must have boolean value.")
		}
	} else {
		crossAccountDNSEnabled = false
	}

	if roleArn != "" {
		if externalId != "" {
			localCloud, err = cloud.NewCloudWithRole(roleArn, externalId, driver.adaptiveRetryMode)
			if err != nil {
				return nil, "", false, status.Errorf(codes.Unauthenticated, "Unable to initialize aws cloud: %v. Please verify role has the correct AWS permissions for cross account mount", err)
			}
		} else {
			localCloud, err = cloud.NewCloudWithRole(roleArn, "", driver.adaptiveRetryMode)
			if err != nil {
				return nil, "", false, status.Errorf(codes.Unauthenticated, "Unable to initialize aws cloud: %v. Please verify role has the correct AWS permissions for cross account mount", err)
			}
		}
	} else {
		localCloud = driver.cloud
	}

	return localCloud, roleArn, crossAccountDNSEnabled, nil
}

func interpolateRootDirectoryName(rootDirectoryPath string, volumeParams map[string]string) (string, error) {
	r := strings.NewReplacer(createListOfVariableSubstitutions(volumeParams)...)
	result := r.Replace(rootDirectoryPath)

	// Check if any templating characters still exist
	if strings.Contains(result, "${") || strings.Contains(result, "}") {
		return "", status.Errorf(codes.InvalidArgument,
			"Path specified \"%v\" contains invalid elements. Can only contain %v", rootDirectoryPath,
			getSupportedComponentNames())
	}
	return result, nil
}

func createListOfVariableSubstitutions(volumeParams map[string]string) []string {
	variableSubstitutions := make([]string, 2*len(subPathPatternComponents))
	i := 0
	for key, volumeParamsKey := range subPathPatternComponents {
		variableSubstitutions[i] = "${" + key + "}"
		variableSubstitutions[i+1] = volumeParams[volumeParamsKey]
		i += 2
	}
	return variableSubstitutions
}

func getSupportedComponentNames() []string {
	keys := make([]string, len(subPathPatternComponents))

	i := 0
	for key := range subPathPatternComponents {
		keys[i] = key
		i++
	}
	sort.Strings(keys)
	return keys
}

func validateEfsPathRequirements(proposedPath string) (bool, error) {
	if len(proposedPath) > 100 {
		// Check the proposed path is 100 characters or fewer
		return false, status.Errorf(codes.InvalidArgument, "Proposed path '%s' exceeds EFS limit of 100 characters", proposedPath)
	} else if strings.Count(proposedPath, "/") > 5 {
		// Check the proposed path contains at most 4 subdirectories
		return false, status.Errorf(codes.InvalidArgument, "Proposed path '%s' EFS limit of 4 subdirectories", proposedPath)
	} else {
		return true, nil
	}
}

func get64LenHash(text string) string {
	h := sha256.New()
	h.Write([]byte(text))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func validateExistingAccessPoint(existingAccessPoint *cloud.AccessPoint, basePath string, gidMin int64, gidMax int64) error {

	normalizedBasePath := strings.TrimPrefix(basePath, "/")
	normalizedAccessPointPath := strings.TrimPrefix(existingAccessPoint.AccessPointRootDir, "/")
	if !strings.HasPrefix(normalizedAccessPointPath, normalizedBasePath) {
		return fmt.Errorf("Access point found but has different base path than what's specified in storage class")
	}

	if existingAccessPoint.PosixUser == nil {
		return fmt.Errorf("Access point found but PosixUser is nil")
	}

	if existingAccessPoint.PosixUser.Gid < gidMin || existingAccessPoint.PosixUser.Gid > gidMax {
		return fmt.Errorf("Access point found but its GID is outside the specified range")
	}

	if existingAccessPoint.PosixUser.Uid < gidMin || existingAccessPoint.PosixUser.Uid > gidMax {
		return fmt.Errorf("Access point found but its UID is outside the specified range")
	}

	return nil
}
