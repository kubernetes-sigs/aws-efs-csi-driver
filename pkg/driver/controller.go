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
	"fmt"
	"os"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
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
	DirectoryMode       = "efs-dir"
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
	klog.V(5).Infof("CreateVolume: provisioning mode %s selected. Supported modes are %s", mode,
		strings.Join(d.GetProvisioningModes(), ","))

	uid, gid, err := d.fsIdentityManager.GetUidAndGid(
		volumeParams[Uid], volumeParams[Gid], volumeParams[GidMin], volumeParams[GidMax], volumeParams[FsId])
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not assign UID or GID to access point")
	}
	volume, err := provisioner.Provision(ctx, req, uid, gid)

	if err != nil {
		d.fsIdentityManager.ReleaseGid(volumeParams[FsId], gid)
		return nil, status.Errorf(codes.Internal, "Could not provision underlying storage: %v", err)
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

	_, _, accessPointId, err := parseVolumeId(volId)
	if err != nil {
		//Returning success for an invalid volume ID. See here - https://github.com/kubernetes-csi/csi-test/blame/5deb83d58fea909b2895731d43e32400380aae3c/pkg/sanity/controller.go#L733
		klog.V(5).Infof("DeleteVolume: Failed to parse volumeID: %v, err: %v, returning success", volId, err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	//TODO: Add Delete File System when FS provisioning is implemented
	if accessPointId != "" {
		err := d.provisioners[AccessPointMode].Delete(ctx, req)
		if err != nil {
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

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {

	return nil, status.Error(codes.Unimplemented, "")
}
