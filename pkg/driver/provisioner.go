package driver

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
)

type Provisioner interface {
	Provision(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.Volume, error)
	Delete(ctx context.Context, req *csi.DeleteVolumeRequest) error
}

type AccessPointProvisioner struct {
	tags                     map[string]string
	cloud                    cloud.Cloud
	gidAllocator             *GidAllocator
	deleteAccessPointRootDir bool
	mounter                  Mounter
}

func getProvisioners(tags map[string]string, cloud cloud.Cloud, gidAllocator *GidAllocator, deleteAccessPointRootDir bool, mounter Mounter) map[string]Provisioner {
	return map[string]Provisioner{
		AccessPointMode: AccessPointProvisioner{
			tags:                     tags,
			cloud:                    cloud,
			gidAllocator:             gidAllocator,
			deleteAccessPointRootDir: deleteAccessPointRootDir,
			mounter:                  mounter,
		},
		DirectoryMode: DirectoryProvisioner{
			osClient: NewOsClient(),
			mounter:  mounter,
			cloud:    cloud,
		},
	}
}

func (a AccessPointProvisioner) Provision(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.Volume, error) {
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

	var (
		azName                 string
		basePath               string
		gid                    int64
		gidMin                 int64
		gidMax                 int64
		localCloud             cloud.Cloud
		roleArn                string
		uid                    int64
		crossAccountDNSEnabled bool
	)

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

	localCloud, roleArn, crossAccountDNSEnabled, err = getCloud(req.GetSecrets(), a.cloud)
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
		}
	}

	if accessPoint == nil {
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
			if uid < 0 {
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
			allocatedGid, err = a.gidAllocator.getNextGid(accessPointsOptions.FileSystemId, accessPoints, gidMin, gidMax)
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
			}
			if err == cloud.ErrAlreadyExists {
				return nil, status.Errorf(codes.AlreadyExists, "Access Point already exists")
			}
			return nil, status.Errorf(codes.Internal, "Failed to create Access point in File System %v : %v", accessPointsOptions.FileSystemId, err)
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

	return &csi.Volume{
		CapacityBytes: volSize,
		VolumeId:      accessPointsOptions.FileSystemId + "::" + accessPoint.AccessPointId,
		VolumeContext: volContext,
	}, nil
}

func (a AccessPointProvisioner) Delete(ctx context.Context, req *csi.DeleteVolumeRequest) error {
	var (
		localCloud             cloud.Cloud
		roleArn                string
		crossAccountDNSEnabled bool
		err                    error
	)

	localCloud, roleArn, crossAccountDNSEnabled, err = getCloud(req.GetSecrets(), a.cloud)
	if err != nil {
		return err
	}

	volId := req.GetVolumeId()
	if volId == "" {
		return status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	fileSystemId, _, accessPointId, err := parseVolumeId(volId)
	if err != nil {
		//Returning success for an invalid volume ID. See here - https://github.com/kubernetes-csi/csi-test/blame/5deb83d58fea909b2895731d43e32400380aae3c/pkg/sanity/controller.go#L733
		klog.V(5).Infof("DeleteVolume: Failed to parse volumeID: %v, err: %v, returning success", volId, err)
		return nil
	}

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
		return status.Errorf(codes.Internal, "Failed to Delete volume %v: %v", volId, err)
	}

	return nil
}

func (a AccessPointProvisioner) getCloud(secrets map[string]string) (cloud.Cloud, string, error) {

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
		localCloud = a.cloud
	}

	return localCloud, roleArn, nil
}

type DirectoryProvisioner struct {
	mounter              Mounter
	cloud                cloud.Cloud
	osClient             OsClient
	deleteProvisionedDir bool
}

func (d DirectoryProvisioner) Provision(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.Volume, error) {
	var provisionedPath string

	var fileSystemId string
	volumeParams := req.GetParameters()
	if value, ok := volumeParams[FsId]; ok {
		if strings.TrimSpace(value) == "" {
			return nil, status.Errorf(codes.InvalidArgument, "Parameter %v cannot be empty", FsId)
		}
		fileSystemId = value
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "Missing %v parameter", FsId)
	}
	klog.V(5).Infof("Provisioning directory on FileSystem %s...", fileSystemId)

	localCloud, roleArn, _, err := getCloud(req.GetSecrets(), d.cloud)
	if err != nil {
		return nil, err
	}

	mountOptions, err := getMountOptions(ctx, localCloud, fileSystemId, roleArn)
	if err != nil {
		return nil, err
	}
	target := TempMountPathPrefix + "/" + uuid.New().String()
	if err := d.mounter.MakeDir(target); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not create dir %q: %v", target, err)
	}
	if err := d.mounter.Mount(fileSystemId, target, "efs", mountOptions); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not mount %q at %q: %v", fileSystemId, target, err)
	}
	// Extract the basePath
	var basePath string
	if value, ok := volumeParams[BasePath]; ok {
		basePath = value
	}

	rootDirName := req.Name
	provisionedPath = path.Join(basePath, rootDirName)

	klog.V(5).Infof("Provisioning directory at path %s", provisionedPath)

	// Grab the required permissions
	perms := os.FileMode(0777)
	if value, ok := volumeParams[DirectoryPerms]; ok {
		parsedPerms, err := strconv.ParseUint(value, 8, 32)
		if err == nil {
			perms = os.FileMode(parsedPerms)
		}
	}

	klog.V(5).Infof("Provisioning directory with permissions %s", perms)

	provisionedDirectory := path.Join(target, provisionedPath)
	err = d.osClient.MkDirAllWithPermsNoOwnership(provisionedDirectory, perms)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not provision directory: %v", err)
	}

	// Check the permissions that actually got created
	actualPerms, err := d.osClient.GetPerms(provisionedDirectory)
	if err != nil {
		klog.V(5).Infof("Could not load file info for '%s'", provisionedDirectory)
	}
	klog.V(5).Infof("Permissions of folder '%s' are '%s'", provisionedDirectory, actualPerms)

	err = d.mounter.Unmount(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not unmount %q: %v", target, err)
	}
	err = d.osClient.RemoveAll(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not delete %q: %v", target, err)
	}

	return &csi.Volume{
		CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
		VolumeId:      fileSystemId + ":" + provisionedPath,
		VolumeContext: map[string]string{},
	}, nil
}

func (d DirectoryProvisioner) Delete(ctx context.Context, req *csi.DeleteVolumeRequest) (e error) {
	if !d.deleteProvisionedDir {
		return nil
	}
	fileSystemId, subpath, _, _ := parseVolumeId(req.GetVolumeId())
	klog.V(5).Infof("Running delete for EFS %s at subpath %s", fileSystemId, subpath)

	localCloud, roleArn, _, err := getCloud(req.GetSecrets(), d.cloud)
	if err != nil {
		return err
	}

	mountOptions, err := getMountOptions(ctx, localCloud, fileSystemId, roleArn)
	if err != nil {
		return err
	}

	target := TempMountPathPrefix + "/" + uuid.New().String()
	klog.V(5).Infof("Making temporary directory at '%s' to temporarily mount EFS folder in", target)
	if err := d.mounter.MakeDir(target); err != nil {
		return status.Errorf(codes.Internal, "Could not create dir %q: %v", target, err)
	}

	defer func() {
		// Try and unmount the directory
		klog.V(5).Infof("Unmounting directory mounted at '%s'", target)
		unmountErr := d.mounter.Unmount(target)
		// If that fails then track the error but don't do anything else
		if unmountErr != nil {
			klog.V(5).Infof("Unmount failed at '%s'", target)
			e = status.Errorf(codes.Internal, "Could not unmount %q: %v", target, err)
		} else {
			// If it is nil then it's safe to try and delete the directory as it should now be empty
			klog.V(5).Infof("Deleting temporary directory at '%s'", target)
			if err := d.osClient.RemoveAll(target); err != nil {
				e = status.Errorf(codes.Internal, "Could not delete %q: %v", target, err)
			}
		}
	}()

	klog.V(5).Infof("Mounting EFS '%s' into temporary directory at '%s'", fileSystemId, target)
	if err := d.mounter.Mount(fileSystemId, target, "efs", mountOptions); err != nil {
		// If this call throws an error we're about to return anyway and the mount has failed, so it's more
		// important we return with that information than worry about the folder not being deleted
		_ = d.osClient.Remove(target)
		return status.Errorf(codes.Internal, "Could not mount %q at %q: %v", fileSystemId, target, err)
	}

	pathToRemove := path.Join(target, subpath)
	klog.V(5).Infof("Delete all files at %s, stored on EFS %s", pathToRemove, fileSystemId)
	if err := d.osClient.RemoveAll(pathToRemove); err != nil {
		return status.Errorf(codes.Internal, "Could not delete directory %q: %v", subpath, err)
	}

	return nil
}

func (d DirectoryProvisioner) getCloud(secrets map[string]string) (cloud.Cloud, string, error) {

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
		localCloud = d.cloud
	}

	return localCloud, roleArn, nil
}

func getMountOptions(ctx context.Context, cloud cloud.Cloud, fileSystemId string, roleArn string) ([]string, error) {
	//Mount File System at it root and delete access point root directory
	mountOptions := []string{"tls", "iam"}
	if roleArn != "" {
		mountTarget, err := cloud.DescribeMountTargets(ctx, fileSystemId, "")

		if err == nil {
			mountOptions = append(mountOptions, MountTargetIp+"="+mountTarget.IPAddress)
		} else {
			klog.Warningf("Failed to describe mount targets for file system %v. Skip using `mounttargetip` mount option: %v", fileSystemId, err)
		}
	}
	return mountOptions, nil
}
