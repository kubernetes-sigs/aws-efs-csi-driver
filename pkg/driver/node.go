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
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
)

var (
	volumeCapAccessModes = []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	}
	volumeIdCounter  = make(map[string]int)
	supportedFSTypes = []string{"efs", ""}
)

const (
	maxInflightMountCallsReached = "The number of concurrent mount calls is %v, which has reached the limit"
)

func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(4).Infof("NodePublishVolume: called with args %+v", util.SanitizeRequest(*req))
	mountOptions := []string{}

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	if err := d.isValidVolumeCapabilities([]*csi.VolumeCapability{volCap}); err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume capability not supported: %s", err))
	}

	if volCap.GetMount() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability access type must be mount")
	}

	if d.inFlightMountTracker != nil {
		if ok := d.inFlightMountTracker.increment(); !ok {
			return nil, status.Errorf(codes.Aborted, maxInflightMountCallsReached, d.inFlightMountTracker.maxCount)
		}

		defer func() {
			klog.V(4).Infof("NodePublishVolume: volume operation finished for volumeId: %s with %d inflight count before decrementing", req.GetVolumeId(), d.inFlightMountTracker.count)
			d.inFlightMountTracker.decrement()
		}()
	}

	// TODO when CreateVolume is implemented, it must use the same key names
	subpath := "/"
	encryptInTransit := true
	crossAccountDNSEnabled := false
	volContext := req.GetVolumeContext()
	for k, v := range volContext {
		switch strings.ToLower(k) {
		//Deprecated
		case "path":
			klog.Warning("Use of path under volumeAttributes is deprecated. This field will be removed in future release")
			if !filepath.IsAbs(v) {
				return nil, status.Errorf(codes.InvalidArgument, "Volume context property %q must be an absolute path", k)
			}
			subpath = filepath.Join(subpath, v)
		case "storage.kubernetes.io/csiprovisioneridentity":
			continue
		case "encryptintransit":
			var err error
			encryptInTransit, err = strconv.ParseBool(v)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume context property %q must be a boolean value: %v", k, err))
			}
		case MountTargetIp:
			ipAddr := volContext[MountTargetIp]
			mountOptions = append(mountOptions, MountTargetIp+"="+ipAddr)
		case CrossAccount:
			var err error
			crossAccountDNSEnabled, err = strconv.ParseBool(v)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Volume context property %q must be a boolean value: %v", k, err))
			}
		default:
			return nil, status.Errorf(codes.InvalidArgument, "Volume context property %s not supported.", k)
		}
	}

	fsid, vpath, apid, err := parseVolumeId(req.GetVolumeId())
	if err != nil {
		// parseVolumeId returns the appropriate error
		return nil, err
	}
	// The `vpath` takes precedence if specified. If not specified, we'll either use the
	// (deprecated) `path` from the volContext, or default to "/" from above.
	if vpath != "" {
		subpath = vpath
	}
	source := fmt.Sprintf("%s:%s", fsid, subpath)

	// If an access point was specified, we need to include two things in the mountOptions:
	// - The access point ID, properly prefixed. (Below, we'll check whether an access point was
	//   also specified in the incoming mount options and react appropriately.)
	// - The TLS option. Access point mounts won't work without it. (For ease of use, we won't
	//   require this to be present in the mountOptions already, but we won't complain if it is.)
	if apid != "" {
		mountOptions = append(mountOptions, fmt.Sprintf("accesspoint=%s", apid), "tls")
	}

	if encryptInTransit {
		// The TLS option may have been added above if apid was set
		// TODO: mountOptions should be a set to avoid all this hasOption checking
		if !hasOption(mountOptions, "tls") {
			mountOptions = append(mountOptions, "tls")
		}
	}

	if crossAccountDNSEnabled {
		mountOptions = append(mountOptions, CrossAccount)
	}

	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	if m := volCap.GetMount(); m != nil {
		for _, f := range m.MountFlags {
			// Special-case check for access point
			// Not sure if `accesspoint` is allowed to have mixed case, but this shouldn't hurt,
			// and it simplifies both matches (HasPrefix, hasOption) below.
			f = strings.ToLower(f)
			if strings.HasPrefix(f, "accesspoint=") {
				// The MountOptions Access Point ID
				moapid := f[12:]
				// No matter what, warn that this is not the right way to specify an access point
				klog.Warning(fmt.Sprintf(
					"Use of 'accesspoint' under mountOptions is deprecated with this driver. "+
						"Specify the access point in the volumeHandle instead, e.g. 'volumeHandle: %s:%s:%s'",
					fsid, subpath, moapid))
				// If they specified the same access point in both places, let it slide; otherwise, fail.
				if apid != "" && moapid != apid {
					return nil, status.Errorf(codes.InvalidArgument,
						"Found conflicting access point IDs in mountOptions (%s) and volumeHandle (%s)", moapid, apid)
				}
				// Fall through; the code below will uniq for us.
			}

			if f == "tls" {
				klog.Warning(
					"Use of 'tls' under mountOptions is deprecated with this driver since tls is enabled by default. " +
						"To disable it, set encrypt in transit in the volumeContext, e.g. 'encryptInTransit: true'")
				// If they set tls and encryptInTransit is true, let it slide; otherwise, fail.
				if !encryptInTransit {
					return nil, status.Errorf(codes.InvalidArgument,
						"Found tls in mountOptions but encryptInTransit is false")
				}
			}

			if strings.HasPrefix(f, "awscredsuri") {
				klog.Warning("awscredsuri mount option is not supported by efs-csi-driver.")
				continue
			}

			if !hasOption(mountOptions, f) {
				mountOptions = append(mountOptions, f)
			}
		}
	}
	klog.V(5).Infof("NodePublishVolume: creating dir %s", target)
	if err := d.mounter.MakeDir(target); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not create dir %q: %v", target, err)
	}

	klog.V(5).Infof("NodePublishVolume: mounting %s at %s with options %v", source, target, mountOptions)
	if err := d.mounter.Mount(source, target, "efs", mountOptions); err != nil {
		os.Remove(target)
		return nil, status.Errorf(codes.Internal, "Could not mount %q at %q: %v", source, target, err)
	}
	klog.V(5).Infof("NodePublishVolume: %s was mounted", target)

	//Increment volume Id counter
	if d.volMetricsOptIn {
		if value, ok := volumeIdCounter[req.GetVolumeId()]; ok {
			volumeIdCounter[req.GetVolumeId()] = value + 1
		} else {
			volumeIdCounter[req.GetVolumeId()] = 1
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume: called with args %+v", util.SanitizeRequest(*req))

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	if d.forceUnmountAfterTimeout {
		klog.V(5).Infof("NodeUnpublishVolume: will retry unmount %s with force after timeout %v", target, d.unmountTimeout)
		err := d.mounter.UnmountWithForce(target, d.unmountTimeout)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not unmountWithForce %q: %v", target, err)
		}
	} else {
		// Check if target directory is a mount point. GetDeviceNameFromMount
		// given a mnt point, finds the device from /proc/mounts
		// returns the device name, reference count, and error code
		_, refCount, err := d.mounter.GetDeviceName(target)
		if err != nil {
			format := "failed to check if volume is mounted: %v"
			return nil, status.Errorf(codes.Internal, format, err)
		}

		// From the spec: If the volume corresponding to the volume_id
		// is not staged to the staging_target_path, the Plugin MUST
		// reply 0 OK.
		if refCount == 0 {
			klog.V(5).Infof("NodeUnpublishVolume: %s target not mounted", target)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}

		klog.V(5).Infof("NodeUnpublishVolume: unmounting %s", target)
		err = d.mounter.Unmount(target)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not unmount %q: %v", target, err)
		}
	}

	klog.V(5).Infof("NodeUnpublishVolume: %s unmounted", target)

	//TODO: If `du` is running on a volume, unmount waits for it to complete. We should stop `du` on unmount in the future for NodeUnpublish
	//Decrement Volume ID counter and evict cache if counter is 0.
	if d.volMetricsOptIn {
		if value, ok := volumeIdCounter[req.GetVolumeId()]; ok {
			value -= 1
			if value < 1 {
				klog.V(4).Infof("Evicting vol ID: %v, vol path : %v from cache", req.VolumeId, target)
				d.volStatter.removeFromCache(req.VolumeId)
				delete(volumeIdCounter, req.GetVolumeId())
			} else {
				volumeIdCounter[req.GetVolumeId()] = value
			}
		}
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *Driver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	klog.V(4).Infof("NodeGetVolumeStats: called with args %+v", util.SanitizeRequest(*req))

	volId := req.GetVolumeId()
	if volId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetVolumePath()
	if target == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume Path not provided")
	}

	_, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "Volume Path %s does not exist", target)
		}

		return nil, status.Errorf(codes.Internal, "Failed to invoke stat on volume path %s: %v", target, err)
	}

	volMetrics, err := d.volStatter.computeVolumeMetrics(volId, target, d.volMetricsRefreshPeriod, d.volMetricsFsRateLimit)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get metrics: %v ", err)
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: volMetrics.volUsage,
	}, nil
}

func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(4).Infof("NodeGetCapabilities: called with args %+v", util.SanitizeRequest(*req))
	var caps []*csi.NodeServiceCapability
	for _, cap := range d.nodeCaps {
		c := &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.NodeGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (d *Driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(4).Infof("NodeGetInfo: called with args %+v", util.SanitizeRequest(*req))

	maxVolumesPerNode := d.volumeAttachLimit
	klog.V(4).Infof("NodeGetInfo: maxVolumesPerNode=%d", maxVolumesPerNode)

	availabilityZone := d.cloud.GetMetadata().GetAvailabilityZone()
	klog.V(4).Infof("NodeGetInfo: availabilityZone=%s", availabilityZone)

	topology := util.BuildTopology(availabilityZone)

	return &csi.NodeGetInfoResponse{
		NodeId:             d.nodeID,
		MaxVolumesPerNode:  maxVolumesPerNode,
		AccessibleTopology: topology,
	}, nil
}

func (d *Driver) isValidVolumeCapabilities(volCaps []*csi.VolumeCapability) error {
	if err := d.validateAccessMode(volCaps); err != nil {
		return err
	}

	if err := d.validateAccessType(volCaps); err != nil {
		return err
	}

	return nil
}

func (d *Driver) validateAccessMode(volCaps []*csi.VolumeCapability) error {
	isSupportedAccessMode := func(cap *csi.VolumeCapability) bool {
		for _, m := range volumeCapAccessModes {
			if m == cap.AccessMode.GetMode() {
				return true
			}
		}
		return false
	}

	var invalidModes []string
	for _, c := range volCaps {
		if !isSupportedAccessMode(c) {
			invalidModes = append(invalidModes, c.AccessMode.GetMode().String())
		}
	}
	if len(invalidModes) != 0 {
		return fmt.Errorf("invalid access mode: %s", strings.Join(invalidModes, ","))
	}
	return nil
}

func (d *Driver) validateAccessType(volCaps []*csi.VolumeCapability) error {
	for _, c := range volCaps {
		if c.GetMount() == nil {
			return fmt.Errorf("only filesystem volumes are supported")
		}
	}
	return nil
}

func (d *Driver) validateFStype(volCaps []*csi.VolumeCapability) error {
	isSupportedFStype := func(cap *csi.VolumeCapability) bool {
		for _, m := range supportedFSTypes {
			if m == cap.GetMount().FsType {
				return true
			}
		}
		return false
	}

	var invalidFStypes []string
	for _, c := range volCaps {
		if !isSupportedFStype(c) {
			invalidFStypes = append(invalidFStypes, c.GetMount().FsType)
		}
	}
	if len(invalidFStypes) != 0 {
		return fmt.Errorf("invalid fstype: %s", strings.Join(invalidFStypes, ","))
	}
	return nil
}

// parseVolumeId accepts a NodePublishVolumeRequest.VolumeId as a colon-delimited string of the
// form `{fileSystemID}:{mountPath}:{accessPointID}`.
//   - The `{fileSystemID}` is required, and expected to be of the form `fs-...`.
//   - The other two fields are optional -- they may be empty or omitted entirely. For example,
//     `fs-abcd1234::`, `fs-abcd1234:`, and `fs-abcd1234` are equivalent.
//   - The `{mountPath}`, if specified, is not required to be absolute.
//   - The `{accessPointID}` is expected to be of the form `fsap-...`.
//
// parseVolumeId returns the parsed values, of which `subpath` and `apid` may be empty; and an
// error, which will be a `status.Error` with `codes.InvalidArgument`, or `nil` if the `volumeId`
// was parsed successfully.
// See the following issues for some background:
// - https://github.com/kubernetes-sigs/aws-efs-csi-driver/issues/100
// - https://github.com/kubernetes-sigs/aws-efs-csi-driver/issues/167
func parseVolumeId(volumeId string) (fsid, subpath, apid string, err error) {
	// Might as well do this up front, since the FSID is required and first in the string
	if !isValidFileSystemId(volumeId) {
		err = status.Errorf(codes.InvalidArgument, "volume ID '%s' is invalid: Expected a file system ID of the form 'fs-...'", volumeId)
		return
	}

	tokens := strings.Split(volumeId, ":")
	if len(tokens) > 3 {
		err = status.Errorf(codes.InvalidArgument, "volume ID '%s' is invalid: Expected at most three fields separated by ':'", volumeId)
		return
	}

	// Okay, we know we have a FSID
	fsid = tokens[0]

	// Do we have a subpath?
	if len(tokens) >= 2 && tokens[1] != "" {
		subpath = path.Clean(tokens[1])
	}

	// Do we have an access point ID?
	if len(tokens) == 3 && tokens[2] != "" {
		apid = tokens[2]
		if !isValidAccessPointId(apid) {
			err = status.Errorf(codes.InvalidArgument, "volume ID '%s' has an invalid access point ID '%s': Expected it to be of the form 'fsap-...'", volumeId, apid)
			return
		}
	}

	return
}

// Check and avoid adding duplicate mount options
func hasOption(options []string, opt string) bool {
	for _, o := range options {
		if o == opt {
			return true
		}
	}
	return false
}

func isValidFileSystemId(filesystemId string) bool {
	return strings.HasPrefix(filesystemId, "fs-")
}

func isValidAccessPointId(accesspointId string) bool {
	return strings.HasPrefix(accesspointId, "fsap-")
}

// Struct for JSON patch operations
type JSONPatch struct {
	OP    string      `json:"op,omitempty"`
	Path  string      `json:"path,omitempty"`
	Value interface{} `json:"value"`
}

// removeNotReadyTaint removes the taint efs.csi.aws.com/agent-not-ready from the local node
// This taint can be optionally applied by users to prevent startup race conditions such as
// https://github.com/kubernetes/kubernetes/issues/95911
func removeNotReadyTaint(k8sClient cloud.KubernetesAPIClient) error {
	if os.Getenv("DISABLE_TAINT_WATCHER") != "" {
		klog.V(4).InfoS("DISABLE_TAINT_WATCHER set, skipping taint removal")
		return nil
	}

	nodeName := os.Getenv("CSI_NODE_NAME")
	if nodeName == "" {
		klog.V(4).InfoS("CSI_NODE_NAME missing, skipping taint removal")
		return nil
	}

	clientset, err := k8sClient()
	if err != nil {
		klog.V(4).InfoS("Failed to communicate with k8s API, skipping taint removal")
		return nil //lint:ignore nilerr Failing to communicate with k8s API is a soft failure
	}

	node, err := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	var taintsToKeep []corev1.Taint
	for _, taint := range node.Spec.Taints {
		if taint.Key != AgentNotReadyNodeTaintKey {
			taintsToKeep = append(taintsToKeep, taint)
		} else {
			klog.V(4).InfoS("Queued taint for removal", "key", taint.Key, "effect", taint.Effect)
		}
	}

	if len(taintsToKeep) == len(node.Spec.Taints) {
		klog.V(4).InfoS("No taints to remove on node, skipping taint removal")
		return nil
	}

	patchRemoveTaints := []JSONPatch{
		{
			OP:    "test",
			Path:  "/spec/taints",
			Value: node.Spec.Taints,
		},
		{
			OP:    "replace",
			Path:  "/spec/taints",
			Value: taintsToKeep,
		},
	}

	patch, err := json.Marshal(patchRemoveTaints)
	if err != nil {
		return err
	}

	_, err = clientset.CoreV1().Nodes().Patch(context.Background(), nodeName, k8stypes.JSONPatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return err
	}
	klog.InfoS("Removed taint(s) from local node", "node", nodeName)
	return nil
}

// remove taint may fail, this keeps retrying until it succeeds, make sure the taint will eventually be removed
func tryRemoveNotReadyTaintUntilSucceed(interval time.Duration, removeFn func() error) {
	for {
		err := removeFn()
		if err == nil {
			return
		}

		klog.ErrorS(err, "Unexpected failure when attempting to remove node taint(s)")
		time.Sleep(interval)
	}
}

func getMaxInflightMountCalls(maxInflightMountCallsOptIn bool, maxInflightMountCalls int64) int64 {
	if maxInflightMountCallsOptIn && maxInflightMountCalls <= 0 {
		klog.Errorf("Fatal error: maxInflightMountCalls must be greater than 0 when maxInflightMountCallsOptIn is true!")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if !maxInflightMountCallsOptIn {
		klog.V(4).Infof("MaxInflightMountCallsOptIn is false, setting maxInflightMountCalls to %d and inflight check is disabled", UnsetMaxInflightMountCounts)
		return UnsetMaxInflightMountCounts
	}

	klog.V(4).Infof("MaxInflightMountCalls is manually set to %d", maxInflightMountCalls)
	return maxInflightMountCalls
}

func getVolumeAttachLimit(volumeAttachLimitOptIn bool, volumeAttachLimit int64) int64 {
	if volumeAttachLimitOptIn && volumeAttachLimit <= 0 {
		klog.Errorf("Fatal error: volumeAttachLimit must be greater than 0 when volumeAttachLimitOptIn is true!")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if !volumeAttachLimitOptIn {
		klog.V(4).Infof("VolumeAttachLimitOptIn is false, setting maxVolumesPerNode to zero so that container orchestrator will decide the value")
		return 0
	}

	klog.V(4).Infof("VolumeAttachLimit is manually set to %d", volumeAttachLimit)
	return volumeAttachLimit
}
