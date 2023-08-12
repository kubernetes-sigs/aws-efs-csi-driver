package driver

import (
	"context"
	"os"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
)

type AccessPointProvisioner struct {
	tags                     map[string]string
	cloud                    cloud.Cloud
	deleteAccessPointRootDir bool
	mounter                  Mounter
}

func (a AccessPointProvisioner) Provision(ctx context.Context, req *csi.CreateVolumeRequest, uid, gid int) (*csi.Volume, error) {
	volumeParams := req.GetParameters()
	volName := req.GetName()
	if volName == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume name not provided")
	}

	var (
		azName  string
		err     error
		roleArn string
	)

	// Volume size is required to match PV to PVC by k8s.
	// Volume size is not consumed by EFS for any purposes.
	volSize := req.GetCapacityRange().GetRequiredBytes()

	accessPointsOptions, err := a.deriveAccessPointOptions(req, uid, gid)

	localCloud, roleArn, err := getCloud(a.cloud, req.GetSecrets())
	if err != nil {
		return nil, err
	}

	// Storage class parameter `az` will be used to fetch preferred mount target for cross account mount.
	// If the `az` storage class parameter is not provided, a random mount target will be picked for mounting.
	// This storage class parameter different from `az` mount option provided by efs-utils https://github.com/aws/efs-utils/blob/v1.31.1/src/mount_efs/__init__.py#L195
	// The `az` mount option provided by efs-utils is used for cross az mount or to provide az of efs one zone file system mount within the same aws-account.
	// To make use of the `az` mount option, add it under storage class's `mountOptions` section. https://kubernetes.io/docs/concepts/storage/storage-classes/#mount-options
	if value, ok := volumeParams[AzName]; ok {
		azName = value
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

	accessPointId, err := localCloud.CreateAccessPoint(ctx, volName, accessPointsOptions)
	if err != nil {
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

	return &csi.Volume{
		CapacityBytes: volSize,
		VolumeId:      accessPointsOptions.FileSystemId + "::" + accessPointId.AccessPointId,
		VolumeContext: volContext,
	}, nil
}

func (a AccessPointProvisioner) deriveAccessPointOptions(req *csi.CreateVolumeRequest,
	uid int, gid int) (*cloud.AccessPointOptions, error) {

	accessPointsOptions := &cloud.AccessPointOptions{
		CapacityGiB: req.GetCapacityRange().GetRequiredBytes(),
		Tags:        a.getTags(),
		Uid:         int64(uid),
		Gid:         int64(gid),
	}

	volumeParams := req.Parameters

	if value, ok := volumeParams[FsId]; ok {
		if strings.TrimSpace(value) == "" {
			return nil, status.Errorf(codes.InvalidArgument, "Parameter %v cannot be empty", FsId)
		}
		accessPointsOptions.FileSystemId = value
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "Missing %v parameter", FsId)
	}

	if value, ok := volumeParams[DirectoryPerms]; ok {
		accessPointsOptions.DirectoryPerms = value
	}

	var basePath string
	if value, ok := volumeParams[BasePath]; ok {
		basePath = value
	}

	rootDirName := req.Name
	rootDir := basePath + "/" + rootDirName
	accessPointsOptions.DirectoryPath = rootDir

	return accessPointsOptions, nil
}

func (a AccessPointProvisioner) getTags() map[string]string {
	// Create tags
	tags := map[string]string{
		DefaultTagKey: DefaultTagValue,
	}

	// Append input tags to default tag
	if len(a.tags) != 0 {
		for k, v := range a.tags {
			tags[k] = v
		}
	}
	return tags
}

func (a AccessPointProvisioner) Delete(ctx context.Context, req *csi.DeleteVolumeRequest) error {
	localCloud, roleArn, err := getCloud(a.cloud, req.GetSecrets())
	if err != nil {
		return err
	}

	fileSystemId, _, accessPointId, _ := parseVolumeId(req.GetVolumeId())
	if accessPointId != "" {
		// Delete access point root directory if delete-access-point-root-dir is set.
		if a.deleteAccessPointRootDir {
			// Check if Access point exists.
			// If access point exists, retrieve its root directory and delete it/
			accessPoint, err := localCloud.DescribeAccessPoint(ctx, accessPointId)
			if err != nil {
				if err == cloud.ErrAccessDenied {
					return status.Errorf(codes.Unauthenticated, "Access Denied. Please ensure you have the right AWS permissions: %v", err)
				}
				if err == cloud.ErrNotFound {
					klog.V(5).Infof("DeleteVolume: Access Point %v not found, returning success", accessPointId)
					return nil
				}
				return status.Errorf(codes.Internal, "Could not get describe Access Point: %v , error: %v", accessPointId, err)
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
			if err := a.mounter.MakeDir(target); err != nil {
				return status.Errorf(codes.Internal, "Could not create dir %q: %v", target, err)
			}
			if err := a.mounter.Mount(fileSystemId, target, "efs", mountOptions); err != nil {
				os.Remove(target)
				return status.Errorf(codes.Internal, "Could not mount %q at %q: %v", fileSystemId, target, err)
			}
			err = os.RemoveAll(target + accessPoint.AccessPointRootDir)
			if err != nil {
				return status.Errorf(codes.Internal, "Could not delete access point root directory %q: %v", accessPoint.AccessPointRootDir, err)
			}
			err = a.mounter.Unmount(target)
			if err != nil {
				return status.Errorf(codes.Internal, "Could not unmount %q: %v", target, err)
			}
			err = os.RemoveAll(target)
			if err != nil {
				return status.Errorf(codes.Internal, "Could not delete %q: %v", target, err)
			}
		}

		// Delete access point
		if err = localCloud.DeleteAccessPoint(ctx, accessPointId); err != nil {
			if err == cloud.ErrAccessDenied {
				return status.Errorf(codes.Unauthenticated, "Access Denied. Please ensure you have the right AWS permissions: %v", err)
			}
			if err == cloud.ErrNotFound {
				klog.V(5).Infof("DeleteVolume: Access Point not found, returning success")
				return nil
			}
			return status.Errorf(codes.Internal, "Failed to Delete volume %v: %v", req.GetVolumeId(), err)
		}
	} else {
		return status.Errorf(codes.NotFound, "Failed to find access point for volume: %v", req.GetVolumeId())
	}

	return nil
}
