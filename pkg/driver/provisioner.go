package driver

import (
	"context"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Provisioner interface {
	Provision(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.Volume, error)
	Delete(ctx context.Context, req *csi.DeleteVolumeRequest) error
}

type BaseProvisioner struct {
	cloud             cloud.Cloud
	mounter           Mounter
	adaptiveRetryMode bool
	lockManager       LockManagerMap
}

func getProvisioners(cloud cloud.Cloud, mounter Mounter, tags map[string]string, deleteAccessPointRootDir bool, adaptiveRetryMode bool, osClient OsClient, deleteProvisionedDir bool) map[string]Provisioner {
	return map[string]Provisioner{
		AccessPointMode: &AccessPointProvisioner{
			BaseProvisioner: BaseProvisioner{
				cloud:             cloud,
				mounter:           mounter,
				adaptiveRetryMode: adaptiveRetryMode,
				lockManager:       NewLockManagerMap(),
			},
			tags:                     tags,
			gidAllocator:             NewGidAllocator(),
			deleteAccessPointRootDir: deleteAccessPointRootDir,
		},
		DirectoryMode: &DirectoryProvisioner{
			BaseProvisioner: BaseProvisioner{
				cloud:   cloud,
				mounter: mounter,
			},
			osClient:             osClient,
			deleteProvisionedDir: deleteProvisionedDir,
		},
	}
}

func getCloud(secrets map[string]string, driverCloud cloud.Cloud, adaptiveRetryMode bool) (cloud.Cloud, string, bool, error) {

	var localCloud cloud.Cloud
	var roleArn string
	var crossAccountDNSEnabled bool
	var err error

	// Fetch aws role ARN for cross account mount from CSI secrets. Link to CSI secrets below
	// https://kubernetes-csi.github.io/docs/secrets-and-credentials.html#csi-operation-secrets
	if value, ok := secrets[RoleArn]; ok {
		roleArn = value
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
		localCloud, err = cloud.NewCloudWithRole(roleArn, adaptiveRetryMode)
		if err != nil {
			return nil, "", false, status.Errorf(codes.Unauthenticated, "Unable to initialize aws cloud: %v. Please verify role has the correct AWS permissions for cross account mount", err)
		}
	} else {
		localCloud = driverCloud
	}

	return localCloud, roleArn, crossAccountDNSEnabled, nil
}
