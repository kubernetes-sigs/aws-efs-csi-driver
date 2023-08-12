package driver

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
)

type Provisioner interface {
	Provision(ctx context.Context, req *csi.CreateVolumeRequest, uid, gid int) (*csi.Volume, error)
	Delete(ctx context.Context, req *csi.DeleteVolumeRequest) error
}

func getProvisioners(tags map[string]string, cloud cloud.Cloud, deleteAccessPointRootDir bool, mounter Mounter, osClient OsClient, deleteProvisionedDir bool) map[string]Provisioner {
	return map[string]Provisioner{
		AccessPointMode: AccessPointProvisioner{
			tags:                     tags,
			cloud:                    cloud,
			deleteAccessPointRootDir: deleteAccessPointRootDir,
			mounter:                  mounter,
		},
		DirectoryMode: DirectoryProvisioner{
			mounter:              mounter,
			cloud:                cloud,
			osClient:             osClient,
			deleteProvisionedDir: deleteProvisionedDir,
		},
	}
}

func getCloud(originalCloud cloud.Cloud, secrets map[string]string) (cloud.Cloud, string, error) {

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
		localCloud = originalCloud
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
