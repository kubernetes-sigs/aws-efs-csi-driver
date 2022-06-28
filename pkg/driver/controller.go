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
	"sort"
	"strconv"
	"strings"

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

	//Parse parameters
	if value, ok := volumeParams[ProvisioningMode]; ok {
		if _, ok = d.provisioners[value]; !ok {
			return nil, status.Errorf(codes.InvalidArgument, "Provisioning mode %s is not supported.", value)
		}
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "Missing %v parameter", ProvisioningMode)
	}

	mode := volumeParams[ProvisioningMode]
	provisioner := d.provisioners[mode]
	klog.V(5).Infof("CreateVolume: provisioning mode %s selected. Support modes are %s", mode,
		strings.Join(d.GetProvisioningModes(), ","))
	volume, err := provisioner.Provision(ctx, req)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not provision underlying storage: %v", err)
	}

	return &csi.CreateVolumeResponse{
		Volume: volume,
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

func getCloud(secrets map[string]string, driver *Driver) (cloud.Cloud, string, bool, error) {

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
		localCloud, err = cloud.NewCloudWithRole(roleArn)
		if err != nil {
			return nil, "", false, status.Errorf(codes.Unauthenticated, "Unable to initialize aws cloud: %v. Please verify role has the correct AWS permissions for cross account mount", err)
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
