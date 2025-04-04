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
	"sort"
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
	DirectoryMode         = "efs-dir"
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

	volumeParams := req.GetParameters()
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
		strings.Join(d.getProvisioningModes(), ","))

	volume, err := provisioner.Provision(ctx, req)
	if err != nil {
		return nil, err
	}

	return &csi.CreateVolumeResponse{
		Volume: volume,
	}, nil
}

func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(4).Infof("DeleteVolume: called with args %+v", *req)
	volId := req.GetVolumeId()
	if volId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	_, subpath, accessPointId, err := parseVolumeId(volId)
	if err != nil {
		//Returning success for an invalid volume ID. See here - https://github.com/kubernetes-csi/csi-test/blame/5deb83d58fea909b2895731d43e32400380aae3c/pkg/sanity/controller.go#L733
		klog.V(5).Infof("DeleteVolume: Failed to parse volumeID: %v, err: %v, returning success", volId, err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	var provisioningMode string
	if accessPointId != "" {
		provisioningMode = AccessPointMode
	} else if subpath != "" {
		provisioningMode = DirectoryMode
	} else {
		klog.V(5).Infof("DeleteVolume: No Access Point or subpath for volume %v, returning success", volId)
		return &csi.DeleteVolumeResponse{}, nil
	}

	klog.V(5).Infof("DeleteVolume: provisioningMode %v", provisioningMode)
	err = d.provisioners[provisioningMode].Delete(ctx, req)
	if err != nil {
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
