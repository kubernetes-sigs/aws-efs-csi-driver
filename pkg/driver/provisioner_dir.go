package driver

import (
	"context"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type DirectoryProvisioner struct {
	BaseProvisioner
	osClient             OsClient
	deleteProvisionedDir bool
}

func (d *DirectoryProvisioner) Provision(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.Volume, error) {
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

	localCloud, roleArn, crossAccountDNSEnabled, err := getCloud(req.GetSecrets(), d.cloud, d.adaptiveRetryMode)
	if err != nil {
		return nil, err
	}

	var azName string
	if value, ok := volumeParams[AzName]; ok {
		azName = value
	}

	mountOptions, mountTargetAddress, err := getMountOptions(ctx, localCloud, fileSystemId, roleArn, crossAccountDNSEnabled, azName)
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
			if mountTargetAddress != "" {
				volContext[MountTargetIp] = mountTargetAddress
			}

		}
	}

	return &csi.Volume{
		CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
		VolumeId:      fileSystemId + ":" + provisionedPath,
		VolumeContext: volContext,
	}, nil
}

func (d *DirectoryProvisioner) Delete(ctx context.Context, req *csi.DeleteVolumeRequest) (e error) {
	if !d.deleteProvisionedDir {
		return nil
	}
	fileSystemId, subpath, _, _ := parseVolumeId(req.GetVolumeId())
	klog.V(5).Infof("Running delete for EFS %s at subpath %s", fileSystemId, subpath)

	localCloud, roleArn, crossAccountDNSEnabled, err := getCloud(req.GetSecrets(), d.cloud, d.adaptiveRetryMode)
	if err != nil {
		return err
	}

	mountOptions, _, err := getMountOptions(ctx, localCloud, fileSystemId, roleArn, crossAccountDNSEnabled, "")
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

func getMountOptions(ctx context.Context, cloud cloud.Cloud, fileSystemId string, roleArn string, crossAccountDNSEnabled bool, azName string) ([]string, string, error) {
	mountOptions := []string{"tls", "iam"}
	mountTargetAddress := ""
	if roleArn != "" {
		if crossAccountDNSEnabled {
			// Connect via dns rather than mounttargetip
			mountOptions = append(mountOptions, CrossAccount)
		} else {
			mountTarget, err := cloud.DescribeMountTargets(ctx, fileSystemId, azName)
			if err == nil {
				mountTargetAddress = mountTarget.IPAddress
				mountOptions = append(mountOptions, MountTargetIp+"="+mountTargetAddress)
			} else {
				klog.Warningf("Failed to describe mount targets for file system %v. Skip using `mounttargetip` mount option: %v", fileSystemId, err)
			}
		}
	}
	return mountOptions, mountTargetAddress, nil
}
