/*
Copyright 2021 The Kubernetes Authors.

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

package sanity

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/klog/v2"
)

// resourceInfo represents a resource (i.e., a volume or a snapshot).
type resourceInfo struct {
	id   string
	data interface{}
}

// volumeInfo keeps track of the information needed to delete a volume.
type volumeInfo struct {
	// Node on which the volume was published, empty if none
	// or publishing is not supported.
	NodeID string
}

// snapshotInfo keeps track of the information needed to delete a snapshot.
type snapshotInfo struct{}

// Resources keeps track of resources, in particular volumes and snapshots, that
// need to be freed when testing is done. It implements both ControllerClient
// and NodeClient and should be used as the only interaction point to either
// APIs. That way, Resources can ensure that resources are marked for cleanup as
// necessary.
// All methods can be called concurrently.
type Resources struct {
	Context *TestContext
	// ControllerClient is meant for struct-internal usage. It should only be
	// invoked directly if automatic cleanup is not desired and cannot be
	// avoided otherwise.
	csi.ControllerClient
	// NodeClient is meant for struct-internal usage. It should only be invoked
	// directly if automatic cleanup is not desired and cannot be avoided
	// otherwise.
	csi.NodeClient

	// mutex protects access to managedResourceInfos.
	mutex                sync.Mutex
	managedResourceInfos []resourceInfo
}

// ControllerClient interface wrappers

// CreateVolume proxies to a Controller service implementation and registers the
// volume for cleanup.
func (cl *Resources) CreateVolume(ctx context.Context, in *csi.CreateVolumeRequest, _ ...grpc.CallOption) (*csi.CreateVolumeResponse, error) {
	return cl.createVolume(ctx, 2, in)
}

// DeleteVolume proxies to a Controller service implementation and unregisters
// the volume from cleanup.
func (cl *Resources) DeleteVolume(ctx context.Context, in *csi.DeleteVolumeRequest, _ ...grpc.CallOption) (*csi.DeleteVolumeResponse, error) {
	return cl.deleteVolume(ctx, 2, in)
}

// ControllerPublishVolume proxies to a Controller service implementation and
// adds the node ID to the corresponding volume for cleanup.
func (cl *Resources) ControllerPublishVolume(ctx context.Context, in *csi.ControllerPublishVolumeRequest, _ ...grpc.CallOption) (*csi.ControllerPublishVolumeResponse, error) {
	return cl.controllerPublishVolume(ctx, 2, in)
}

// CreateSnapshot proxies to a Controller service implementation and registers
// the snapshot for cleanup.
func (cl *Resources) CreateSnapshot(ctx context.Context, in *csi.CreateSnapshotRequest, _ ...grpc.CallOption) (*csi.CreateSnapshotResponse, error) {
	return cl.createSnapshot(ctx, 2, in)
}

// DeleteSnapshot proxies to a Controller service implementation and unregisters
// the snapshot from cleanup.
func (cl *Resources) DeleteSnapshot(ctx context.Context, in *csi.DeleteSnapshotRequest, _ ...grpc.CallOption) (*csi.DeleteSnapshotResponse, error) {
	return cl.deleteSnapshot(ctx, 2, in)
}

// MustCreateVolume is like CreateVolume but asserts that the volume was
// successfully created.
func (cl *Resources) MustCreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) *csi.CreateVolumeResponse {
	return cl.mustCreateVolumeWithOffset(ctx, 2, req)
}

func (cl *Resources) mustCreateVolumeWithOffset(ctx context.Context, offset int, req *csi.CreateVolumeRequest) *csi.CreateVolumeResponse {
	vol, err := cl.createVolume(ctx, offset+1, req)
	ExpectWithOffset(offset, err).NotTo(HaveOccurred(), "volume create failed")
	ExpectWithOffset(offset, vol).NotTo(BeNil(), "volume response is nil")
	ExpectWithOffset(offset, vol.GetVolume()).NotTo(BeNil(), "volume in response is nil")
	ExpectWithOffset(offset, vol.GetVolume().GetVolumeId()).NotTo(BeEmpty(), "volume ID in response is missing")
	return vol
}

func (cl *Resources) createVolume(ctx context.Context, offset int, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	vol, err := cl.ControllerClient.CreateVolume(ctx, req)
	if err == nil && vol != nil && vol.GetVolume().GetVolumeId() != "" {
		cl.registerVolume(offset+1, vol.GetVolume().GetVolumeId(), volumeInfo{})
	}
	return vol, err
}

func (cl *Resources) deleteVolume(ctx context.Context, offset int, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	vol, err := cl.ControllerClient.DeleteVolume(ctx, req)
	if err == nil {
		cl.unregisterResource(offset+1, req.VolumeId)
	}
	return vol, err
}

// MustControllerPublishVolume is like ControllerPublishVolume but asserts that
// the volume was successfully controller-published.
func (cl *Resources) MustControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) *csi.ControllerPublishVolumeResponse {
	conpubvol, err := cl.controllerPublishVolume(ctx, 2, req)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "controller publish volume failed")
	ExpectWithOffset(1, conpubvol).NotTo(BeNil(), "controller publish volume response is nil")
	return conpubvol
}

func (cl *Resources) controllerPublishVolume(ctx context.Context, offset int, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	conpubvol, err := cl.ControllerClient.ControllerPublishVolume(ctx, req)
	if err == nil && req.VolumeId != "" && req.NodeId != "" {
		cl.registerVolume(offset+1, req.VolumeId, volumeInfo{NodeID: req.NodeId})
	}
	return conpubvol, err
}

// registerVolume adds or updates an entry for given volume.
func (cl *Resources) registerVolume(offset int, id string, info volumeInfo) {
	ExpectWithOffset(offset, id).NotTo(BeEmpty(), "volume ID is empty")
	ExpectWithOffset(offset, info).NotTo(BeNil(), "volume info is nil")
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	klog.V(4).Infof("registering volume ID %s", id)
	cl.managedResourceInfos = append(cl.managedResourceInfos, resourceInfo{
		id:   id,
		data: info,
	})
}

// MustCreateSnapshot is like CreateSnapshot but asserts that the snapshot was
// successfully created.
func (cl *Resources) MustCreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) *csi.CreateSnapshotResponse {
	return cl.mustCreateSnapshotWithOffset(ctx, 2, req)
}

func (cl *Resources) mustCreateSnapshotWithOffset(ctx context.Context, offset int, req *csi.CreateSnapshotRequest) *csi.CreateSnapshotResponse {
	snap, err := cl.createSnapshot(ctx, offset+1, req)
	ExpectWithOffset(offset, err).NotTo(HaveOccurred(), "create snapshot failed")
	ExpectWithOffset(offset, snap).NotTo(BeNil(), "create snasphot response is nil")
	verifySnapshotInfoWithOffset(offset+1, snap.GetSnapshot())
	return snap
}

// MustCreateSnapshotFromVolumeRequest creates a volume from the given
// CreateVolumeRequest and a snapshot subsequently. It registers the volume and
// snapshot and asserts that both were created successfully.
func (cl *Resources) MustCreateSnapshotFromVolumeRequest(ctx context.Context, req *csi.CreateVolumeRequest, snapshotName string) (*csi.CreateSnapshotResponse, *csi.CreateVolumeResponse) {
	vol := cl.mustCreateVolumeWithOffset(ctx, 2, req)
	snap := cl.mustCreateSnapshotWithOffset(ctx, 2, MakeCreateSnapshotReq(cl.Context, snapshotName, vol.Volume.VolumeId))
	return snap, vol
}

func (cl *Resources) createSnapshot(ctx context.Context, offset int, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	snap, err := cl.ControllerClient.CreateSnapshot(ctx, req)
	if err == nil && snap.GetSnapshot().GetSnapshotId() != "" {
		cl.registerSnapshot(offset+1, snap.Snapshot.SnapshotId)
	}
	return snap, err
}

func (cl *Resources) deleteSnapshot(ctx context.Context, offset int, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	snap, err := cl.ControllerClient.DeleteSnapshot(ctx, req)
	if err == nil && req.SnapshotId != "" {
		cl.unregisterResource(offset+1, req.SnapshotId)
	}
	return snap, err
}

func (cl *Resources) registerSnapshot(offset int, id string) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	cl.registerSnapshotNoLock(offset+1, id)
}

func (cl *Resources) registerSnapshotNoLock(offset int, id string) {
	ExpectWithOffset(offset, id).NotTo(BeEmpty(), "ID for register snapshot is missing")
	klog.V(4).Infof("registering snapshot ID %s", id)
	cl.managedResourceInfos = append(cl.managedResourceInfos, resourceInfo{
		id:   id,
		data: snapshotInfo{},
	})
}

func (cl *Resources) unregisterResource(offset int, id string) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	cl.unregisterResourceNoLock(offset+1, id)
}

func (cl *Resources) unregisterResourceNoLock(offset int, id string) {
	ExpectWithOffset(offset, id).NotTo(BeEmpty(), "ID for unregister resource is missing")
	// Find resource info with the given ID and remove it.
	for i, resInfo := range cl.managedResourceInfos {
		if resInfo.id == id {
			klog.V(4).Infof("unregistering resource ID %s", id)
			cl.managedResourceInfos = append(cl.managedResourceInfos[:i], cl.managedResourceInfos[i+1:]...)
			return
		}
	}
}

// Cleanup calls unpublish methods as needed and deletes all managed resources.
func (cl *Resources) Cleanup() {
	klog.V(4).Info("cleaning up all registered resources")
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	ctx := context.Background()

	// Clean up resources in LIFO order to account for dependency order.
	var errs []error
	for i := len(cl.managedResourceInfos) - 1; i >= 0; i-- {
		resInfo := cl.managedResourceInfos[i]
		id := resInfo.id
		switch resType := resInfo.data.(type) {
		case volumeInfo:
			errs = append(errs, cl.cleanupVolume(ctx, 2, id, resType)...)
		case snapshotInfo:
			errs = append(errs, cl.cleanupSnapshot(ctx, 2, id)...)
		default:
			Fail(fmt.Sprintf("unknown resource type: %T", resType), 1)
		}
	}

	ExpectWithOffset(2, errs).To(BeEmpty(), "resource cleanup failed")

	klog.V(4).Info("clearing managed resources list")
	cl.managedResourceInfos = []resourceInfo{}
}

func (cl *Resources) cleanupVolume(ctx context.Context, offset int, volumeID string, info volumeInfo) (errs []error) {
	klog.V(4).Infof("deleting volume ID %s", volumeID)
	if cl.NodeClient != nil {
		if _, err := cl.NodeUnpublishVolume(
			ctx,
			&csi.NodeUnpublishVolumeRequest{
				VolumeId:   volumeID,
				TargetPath: cl.Context.TargetPath + "/target",
			},
		); isRelevantError(err) {
			errs = append(errs, fmt.Errorf("NodeUnpublishVolume for volume ID %s failed: %s", volumeID, err))
		}

		if isNodeCapabilitySupported(cl, csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME) {
			if _, err := cl.NodeUnstageVolume(
				ctx,
				&csi.NodeUnstageVolumeRequest{
					VolumeId:          volumeID,
					StagingTargetPath: cl.Context.StagingPath,
				},
			); isRelevantError(err) {
				errs = append(errs, fmt.Errorf("NodeUnstageVolume for volume ID %s failed: %s", volumeID, err))
			}
		}
	}

	if info.NodeID != "" {
		if _, err := cl.ControllerClient.ControllerUnpublishVolume(
			ctx,
			&csi.ControllerUnpublishVolumeRequest{
				VolumeId: volumeID,
				NodeId:   info.NodeID,
				Secrets:  cl.Context.Secrets.ControllerUnpublishVolumeSecret,
			},
		); err != nil {
			errs = append(errs, fmt.Errorf("ControllerUnpublishVolume for volume ID %s failed: %s", volumeID, err))
		}
	}

	if _, err := cl.ControllerClient.DeleteVolume(
		ctx,
		&csi.DeleteVolumeRequest{
			VolumeId: volumeID,
			Secrets:  cl.Context.Secrets.DeleteVolumeSecret,
		},
	); err != nil {
		errs = append(errs, fmt.Errorf("DeleteVolume for volume ID %s failed: %s", volumeID, err))
	}

	return errs
}

func (cl *Resources) cleanupSnapshot(ctx context.Context, offset int, snapshotID string) []error {
	klog.Infof("deleting snapshot ID %s", snapshotID)
	if _, err := cl.ControllerClient.DeleteSnapshot(
		ctx,
		&csi.DeleteSnapshotRequest{
			SnapshotId: snapshotID,
			Secrets:    cl.Context.Secrets.DeleteSnapshotSecret,
		},
	); err != nil {
		return []error{fmt.Errorf("DeleteSnapshot for snapshot ID %s failed: %s", snapshotID, err)}
	}

	return nil
}

func isRelevantError(err error) bool {
	return err != nil && status.Code(err) != codes.NotFound
}
