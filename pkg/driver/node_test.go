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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

const (
	volumeId   = "fs-abcd1234"
	targetPath = "/target/path"
)

type errtyp struct {
	code    string
	message string
}

func setup(mockCtrl *gomock.Controller, volStatter VolStatter, volMetricsOptIn bool, maxInflightCalls int64) (*mocks.MockMounter, *Driver, context.Context) {
	mockMounter := mocks.NewMockMounter(mockCtrl)
	nodeCaps := SetNodeCapOptInFeatures(volMetricsOptIn)
	driver := &Driver{
		endpoint:             "endpoint",
		nodeID:               "nodeID",
		mounter:              mockMounter,
		volStatter:           volStatter,
		volMetricsOptIn:      true,
		nodeCaps:             nodeCaps,
		inFlightMountTracker: NewInFlightMountTracker(maxInflightCalls),
	}
	ctx := context.Background()
	return mockMounter, driver, ctx
}

func testResult(t *testing.T, funcName string, ret interface{}, err error, expectError errtyp) {
	if expectError.code == "" {
		if err != nil {
			t.Fatalf("%s is failed: %v", funcName, err)
		}
		if ret == nil {
			t.Fatal("Expected non-nil return value")
		}
	} else {
		if err == nil {
			t.Fatalf("%s is not failed", funcName)
		}
		// Sure would be nice if grpc.statusError was exported :(
		// The error string looks like:
		// "rpc error: code = {code} desc = {desc}"
		tokens := strings.SplitN(err.Error(), " = ", 3)
		expCode := strings.Split(tokens[1], " ")[0]
		if expCode != expectError.code {
			t.Fatalf("Expected error code %q but got %q", expCode, expectError.code)
		}
		if tokens[2] != expectError.message {
			t.Fatalf("\nExpected error message: %s\nActual error message:   %s", expectError.message, tokens[2])
		}
	}
}

func TestNodePublishVolume(t *testing.T) {

	var (
		accessPointID = "fsap-abcd1234"
		stdVolCap     = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			},
		}
	)

	testCases := []struct {
		name                  string
		req                   *csi.NodePublishVolumeRequest
		expectMakeDir         bool
		mountArgs             []interface{}
		mountSuccess          bool
		volMetricsOptIn       bool
		expectError           errtyp
		maxInflightMountCalls int64
		csiNodeMemoryLimit    string
	}{
		{
			name: "success: normal",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			volMetricsOptIn:       true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			// DNS-name volumeHandle (static PV): the fsid is passed through
			// unchanged as the efs-utils mount source, asserted via mountArgs[0].
			name: "success: efs dns-name volume handle",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "fs-9919e11b.efs.us-east-1.amazonaws.com",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{"fs-9919e11b.efs.us-east-1.amazonaws.com:/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			volMetricsOptIn:       true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			// AZ-prefixed DNS-name volumeHandle (static PV): the fsid, including
			// the leading AZ label, is passed through unchanged as the mount source.
			name: "success: efs dns-name az-prefixed volume handle",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "us-east-1a.fs-9919e11b.efs.us-east-1.amazonaws.com",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{"us-east-1a.fs-9919e11b.efs.us-east-1.amazonaws.com:/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			volMetricsOptIn:       true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: empty path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId + ":",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			volMetricsOptIn:       true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: empty path and access point",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId + "::",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			volMetricsOptIn:       true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with read only mount",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				Readonly:         true,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls", "ro"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with tls mount options",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: volumeId,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"tls"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				TargetPath: targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with path in volume handle",
			req: &csi.NodePublishVolumeRequest{
				// This also shows that the path gets cleaned
				VolumeId:         volumeId + ":/a/b/",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/a/b", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with path in volume handle, empty access point",
			req: &csi.NodePublishVolumeRequest{
				// This also shows that relative paths are allowed when specified via volume handle
				VolumeId:         volumeId + ":a/b/:",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":a/b", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: access point in volume handle, no path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId + "::" + accessPointID,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"accesspoint=" + accessPointID, "tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: path and access point in volume handle",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId + ":/a/b:" + accessPointID,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/a/b", targetPath, "efs", []string{"accesspoint=" + accessPointID, "tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			// TODO: Validate deprecation warning
			name: "success: same access point in volume handle and mount options",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: volumeId + "::" + accessPointID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							// This also shows we allow the `tls` option to exist already
							MountFlags: []string{"tls", "accesspoint=" + accessPointID},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				TargetPath: targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"accesspoint=" + accessPointID, "tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with encryptInTransit true volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"encryptInTransit": "true"},
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with encryptInTransit false volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"encryptInTransit": "false"},
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with crossaccount true volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"crossaccount": "true"},
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls", "crossaccount"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with crossaccount false volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"crossaccount": "false"},
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: normal with volume context populated from dynamic provisioning",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext: map[string]string{"storage.kubernetes.io/csiprovisioneridentity": "efs.csi.aws.com",
					"mounttargetip": "127.0.0.1"},
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"mounttargetip=127.0.0.1", "tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "success: supported volume fstype capability",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: volumeId,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "efs",
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				TargetPath: targetPath,
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: conflicting access point in volume handle and mount options",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: volumeId + "::" + accessPointID,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"tls", "accesspoint=fsap-deadbeef"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				TargetPath: targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Found conflicting access point IDs in mountOptions (fsap-deadbeef) and volumeHandle (fsap-abcd1234)",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: too many fields in volume handle",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId + ":/a/b/::four!",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 'fs-abcd1234:/a/b/::four!' is invalid: Expected at most three fields separated by ':'",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: missing target path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Target path not provided",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: missing volume capability",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:   volumeId,
				TargetPath: targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume capability not provided",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: unsupported volume capability",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: volumeId,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
					},
				},
				TargetPath: targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume capability not supported: invalid access mode: SINGLE_NODE_READER_ONLY",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: unsupported volume access type",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: volumeId,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				TargetPath: targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume capability not supported: only filesystem volumes are supported",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: multiple unsupported volume capabilities",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: volumeId,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "abc"},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
					},
				},

				TargetPath: targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume capability not supported: invalid access mode: SINGLE_NODE_READER_ONLY",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: mounter failed to MakeDir",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{}, // Signal MakeDir failure
			expectError: errtyp{
				code:    "Internal",
				message: `Could not create dir "/target/path": failed to MakeDir`,
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: mounter failed to Mount",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:  false,
			expectError: errtyp{
				code:    "Internal",
				message: `Could not mount "fs-abcd1234:/" at "/target/path": failed to Mount`,
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: unsupported volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"asdf": "qwer"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume context property asdf not supported.",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			// A comma would otherwise be joined into the -o list as extra mount options.
			name: "fail: mounttargetip carrying additional mount options",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"mounttargetip": "10.0.0.1,uid=0,gid=0,port=12345"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: `Volume context property "mounttargetip"="10.0.0.1,uid=0,gid=0,port=12345" is not a valid IP address`,
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: mounttargetip that is not an IP address",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"mounttargetip": "not-an-ip"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: `Volume context property "mounttargetip"="not-an-ip" is not a valid IP address`,
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: 'path' is a deprecated and unsupported volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"path": "a/b"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume context property path not supported.",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: invalid filesystem ID",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "invalid-id",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 'invalid-id' is invalid: Expected a file system ID of the form 'fs-[0-9a-f]{8,40}' or a mount-target DNS name (e.g. 'fs-abcd1234.efs.<region>.amazonaws.com')",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: invalid access point ID",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId + "::invalid-id",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 'fs-abcd1234::invalid-id' has an invalid access point ID 'invalid-id': Expected it to be of the form 'fsap-[0-9a-f]{8,40}'",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: tls in mount options and encryptInTransit false volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: volumeId,
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"tls"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				TargetPath:    targetPath,
				VolumeContext: map[string]string{"encryptInTransit": "false"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Found tls in mountOptions but encryptInTransit is false",
			},
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: encryptInTransit invalid boolean value volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"encryptInTransit": "asdf"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume context property \"encryptInTransit\" must be a boolean value: strconv.ParseBool: parsing \"asdf\": invalid syntax",
			},
		},
		{
			name: "success: EFS with access point (new format)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "efs:fs-abcd1234::fsap-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{"fs-abcd1234:/", targetPath, "efs", []string{"accesspoint=fsap-abcd1234", "tls"}},
			mountSuccess:  true,
		},
		{
			name: "success: EFS without access point (new format)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "efs:fs-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{"fs-abcd1234:/", targetPath, "efs", []string{"tls"}},
			mountSuccess:  true,
		},
		{
			name: "success: EFS with path and access point (new format)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "efs:fs-abcd1234:/data/shared:fsap-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{"fs-abcd1234:/data/shared", targetPath, "efs", []string{"accesspoint=fsap-abcd1234", "tls"}},
			mountSuccess:  true,
		},
		{
			name: "fail: EFS with invalid filesystem ID (new format)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "efs:invalid-id",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 'efs:invalid-id' is invalid: Expected a file system ID of the form 'fs-[0-9a-f]{8,40}' or a mount-target DNS name (e.g. 'fs-abcd1234.efs.<region>.amazonaws.com')",
			},
		},
		{
			name: "fail: EFS with invalid access point ID (new format)",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "efs:fs-abcd1234::invalid-id",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 'efs:fs-abcd1234::invalid-id' has an invalid access point ID 'invalid-id': Expected it to be of the form 'fsap-[0-9a-f]{8,40}'",
			},
		},
		{
			name: "success: S3Files with access point",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "s3files:fs-abcd1234::fsap-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{"fs-abcd1234:/", targetPath, "s3files", []string{"accesspoint=fsap-abcd1234", "tls"}},
			mountSuccess:  true,
		},
		{
			name: "success: S3Files without access point",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "s3files:fs-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{"fs-abcd1234:/", targetPath, "s3files", []string{"tls"}},
			mountSuccess:  true,
		},
		{
			name: "success: S3Files with path and access point",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "s3files:fs-abcd1234:/data/shared:fsap-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{"fs-abcd1234:/data/shared", targetPath, "s3files", []string{"accesspoint=fsap-abcd1234", "tls"}},
			mountSuccess:  true,
		},
		{
			name: "success: S3Files with nos3readcache when memory limit is low",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "s3files:fs-abcd1234::fsap-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			csiNodeMemoryLimit: "1073741824",
			expectMakeDir:      true,
			mountArgs:          []interface{}{"fs-abcd1234:/", targetPath, "s3files", []string{"accesspoint=fsap-abcd1234", "tls", "nos3readcache"}},
			mountSuccess:       true,
		},
		{
			name: "fail: S3Files with invalid filesystem ID",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "s3files:invalid-id",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 's3files:invalid-id' is invalid: Expected a file system ID of the form 'fs-[0-9a-f]{8,40}' or a mount-target DNS name (e.g. 'fs-abcd1234.efs.<region>.amazonaws.com')",
			},
		},
		{
			name: "fail: S3Files with invalid access point ID",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "s3files:fs-abcd1234::invalid-id",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 's3files:fs-abcd1234::invalid-id' has an invalid access point ID 'invalid-id': Expected it to be of the form 'fsap-[0-9a-f]{8,40}'",
			},
		},
		{
			name: "fail: with invalid fsType in volume id",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "nfs:fs-abcd1234::fsap-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 'nfs:fs-abcd1234::fsap-abcd1234' is invalid: Expected at most three fields separated by ':'",
			},
		},
		{
			name: "fail: S3Files with encryptInTransit false",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "s3files:fs-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"encryptInTransit": "false"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Encryption in transit cannot be disabled for S3 Files file system. Remove 'encryptInTransit: false' from your volume configuration or omit the encryptInTransit parameter (encryption is enabled by default).",
			},
		},
		{
			name: "fail: S3Files with stunnel mount option",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: "s3files:fs-abcd1234",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"stunnel"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				TargetPath: targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "stunnel mount option is not supported by S3 Files file system.",
			},
		},
		{
			name: "fail: S3Files with crossaccount true volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "s3files:fs-abcd1234",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"crossaccount": "true"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Cross-account mounting is not supported for S3 Files file system. Remove 'crossaccount: true' from your volume configuration.",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.csiNodeMemoryLimit != "" {
				t.Setenv("CSI_NODE_MEMORY_LIMIT", tc.csiNodeMemoryLimit)
			} else {
				t.Setenv("CSI_NODE_MEMORY_LIMIT", strconv.Itoa(minMemoryInBytesToEnableS3ReadCache*2))
			}
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockMounter, driver, ctx := setup(mockCtrl, NewVolStatter(), tc.volMetricsOptIn, tc.maxInflightMountCalls)

			if tc.expectMakeDir {
				var err error
				// If not expecting mount, it's because mkdir errored
				if len(tc.mountArgs) == 0 {
					err = fmt.Errorf("failed to MakeDir")
				}
				mockMounter.EXPECT().MakeDir(gomock.Eq(targetPath)).Return(err)
			}
			if len(tc.mountArgs) != 0 {
				var err error
				if !tc.mountSuccess {
					err = fmt.Errorf("failed to Mount")
				}
				mockMounter.EXPECT().Mount(tc.mountArgs[0], tc.mountArgs[1], tc.mountArgs[2], tc.mountArgs[3]).Return(err)
			}

			ret, err := driver.NodePublishVolume(ctx, tc.req)
			testResult(t, "NodePublishVolume", ret, err, tc.expectError)
		})
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	var metrics = &volMetrics{
		volPath:   targetPath,
		timeStamp: time.Now().Add(time.Duration(-10) * time.Minute),
		volUsage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Available: 1,
				Used:      1,
				Total:     2,
			},
		},
	}

	testCases := []struct {
		name                string
		req                 *csi.NodeUnpublishVolumeRequest
		expectGetDeviceName bool
		getDeviceNameReturn []interface{}
		expectUnmount       bool
		setupVolUsageCache  bool
		unmountReturn       error
		expectError         errtyp
	}{
		{
			name: "success: normal",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   volumeId,
				TargetPath: targetPath,
			},
			expectGetDeviceName: true,
			getDeviceNameReturn: []interface{}{"", 1, nil},
			expectUnmount:       true,
			unmountReturn:       nil,
		},
		{
			name: "success: test volume usage cache eviction",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   volumeId,
				TargetPath: targetPath,
			},
			expectGetDeviceName: true,
			getDeviceNameReturn: []interface{}{"", 1, nil},
			expectUnmount:       true,
			setupVolUsageCache:  true,
			unmountReturn:       nil,
		},
		{
			name: "success: unpublish with already unmounted target",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   volumeId,
				TargetPath: targetPath,
			},
			expectGetDeviceName: true,
			getDeviceNameReturn: []interface{}{"", 0, nil},
			// NUV returns early if the refcount is zero
			expectUnmount: false,
		},
		{
			name: "fail: targetPath is missing",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId: volumeId,
			},
			expectGetDeviceName: false,
			expectUnmount:       false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Target path not provided",
			},
		},
		{
			name: "fail: mounter failed to umount",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   volumeId,
				TargetPath: targetPath,
			},
			expectGetDeviceName: true,
			getDeviceNameReturn: []interface{}{"", 1, nil},
			expectUnmount:       true,
			unmountReturn:       fmt.Errorf("Unmount failed"),
			expectError: errtyp{
				code:    "Internal",
				message: `Could not unmount "/target/path": Unmount failed`,
			},
		},
		{
			name: "fail: mounter failed to GetDeviceName",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   volumeId,
				TargetPath: targetPath,
			},
			expectGetDeviceName: true,
			getDeviceNameReturn: []interface{}{"", 1, fmt.Errorf("GetDeviceName failed")},
			expectUnmount:       false,
			expectError: errtyp{
				code:    "Internal",
				message: "failed to check if volume is mounted: GetDeviceName failed",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockMounter, driver, ctx := setup(mockCtrl, NewVolStatter(), true, UnsetMaxInflightMountCounts)

			if tc.expectGetDeviceName {
				mockMounter.EXPECT().
					GetDeviceName(targetPath).
					Return(tc.getDeviceNameReturn[0], tc.getDeviceNameReturn[1], tc.getDeviceNameReturn[2])
			}
			if tc.expectUnmount {
				mockMounter.EXPECT().Unmount(targetPath).Return(tc.unmountReturn)
			}

			if tc.setupVolUsageCache {
				volUsageCache = make(map[string]*volMetrics)
				volUsageCache[targetPath] = metrics
			}

			ret, err := driver.NodeUnpublishVolume(ctx, tc.req)
			testResult(t, "NodeUnpublishVolume", ret, err, tc.expectError)
		})
	}
}

// TestNodePublishUnpublishVolumeConcurrent exercises concurrent
// NodePublishVolume and NodeUnpublishVolume calls (with volMetricsOptIn
// enabled) to guard against concurrent map read/write panics on the
// package-level volumeIdCounter map. Run with `go test -race` to verify
// there is no data race.
func TestNodePublishUnpublishVolumeConcurrent(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockMounter, driver, ctx := setup(mockCtrl, NewVolStatter(), true, UnsetMaxInflightMountCounts)

	stdVolCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}

	// The mounter calls happen for every goroutine with varying volume/target
	// paths, so allow them to be called any number of times with any args.
	mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil).AnyTimes()
	mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockMounter.EXPECT().GetDeviceName(gomock.Any()).Return("", 1, nil).AnyTimes()
	mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil).AnyTimes()

	const numGoroutines = 50
	const numVolumes = 5

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			// Reuse a small set of volume IDs across goroutines so that
			// concurrent Publish/Unpublish calls for the *same* volume ID
			// exercise the shared volumeIdCounter map entry as well.
			volID := fmt.Sprintf("fs-abcd%04d", i%numVolumes)
			target := fmt.Sprintf("/target/path/%d", i)

			publishReq := &csi.NodePublishVolumeRequest{
				VolumeId:         volID,
				VolumeCapability: stdVolCap,
				TargetPath:       target,
			}
			if _, err := driver.NodePublishVolume(ctx, publishReq); err != nil {
				t.Errorf("NodePublishVolume failed: %v", err)
				return
			}

			unpublishReq := &csi.NodeUnpublishVolumeRequest{
				VolumeId:   volID,
				TargetPath: target,
			}
			if _, err := driver.NodeUnpublishVolume(ctx, unpublishReq); err != nil {
				t.Errorf("NodeUnpublishVolume failed: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestNodeGetVolumeStats(t *testing.T) {
	var (
		validPath   = "/tmp/target"
		invalidPath = "/path/does/not/exist"
		volMetrics  = &volMetrics{
			volPath:   validPath,
			timeStamp: time.Now().Add(time.Duration(-10) * time.Minute),
			volUsage: []*csi.VolumeUsage{
				{
					Unit:      csi.VolumeUsage_BYTES,
					Available: 1,
					Used:      1,
					Total:     2,
				},
			},
		}
	)
	makeDir(validPath)

	//reset jitter to 0 for testing
	jitter = time.Duration(0)

	testCases := []struct {
		name             string
		req              *csi.NodeGetVolumeStatsRequest
		updateCache      bool
		expectError      errtyp
		expectedResponse *csi.NodeGetVolumeStatsResponse
	}{
		{
			name: "success: volume unknown",
			req: &csi.NodeGetVolumeStatsRequest{
				VolumeId:   volumeId,
				VolumePath: validPath,
			},
			expectedResponse: &csi.NodeGetVolumeStatsResponse{
				Usage: []*csi.VolumeUsage{
					{
						Unit: csi.VolumeUsage_UNKNOWN,
					},
				},
			},
		},
		{
			name: "success: volume known",
			req: &csi.NodeGetVolumeStatsRequest{
				VolumeId:   volumeId,
				VolumePath: validPath,
			},
			updateCache: true,
			expectedResponse: &csi.NodeGetVolumeStatsResponse{
				Usage: []*csi.VolumeUsage{
					{
						Unit:      csi.VolumeUsage_BYTES,
						Available: 1,
						Total:     2,
						Used:      1,
					},
				},
			},
		},
		{
			name: "Fail: Path does not exist",
			req: &csi.NodeGetVolumeStatsRequest{
				VolumeId:   volumeId,
				VolumePath: invalidPath,
			},
			expectError: errtyp{
				code:    "NotFound",
				message: "Volume Path /path/does/not/exist does not exist",
			},
		},
		{
			name: "Fail: Volume ID does not exist",
			req: &csi.NodeGetVolumeStatsRequest{
				VolumeId:   "",
				VolumePath: invalidPath,
			},
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume ID not provided",
			},
		},
		{
			name: "Fail: Volume Path does not exist",
			req: &csi.NodeGetVolumeStatsRequest{
				VolumeId:   volumeId,
				VolumePath: "",
			},
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "Volume Path not provided",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var driver *Driver
			var ctx context.Context
			var _ *mocks.MockMounter

			//setup
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			_, driver, ctx = setup(mockCtrl, NewVolStatter(), true, UnsetMaxInflightMountCounts)

			if tc.updateCache {
				mu.Lock()
				volUsageCache[volumeId] = volMetrics
				mu.Unlock()
			}

			//execute
			ret, err := driver.NodeGetVolumeStats(ctx, tc.req)

			//verify
			testResult(t, "NodeGetVolumeStats", ret, err, tc.expectError)
			if tc.expectedResponse != nil {
				testResponse(t, tc.expectedResponse, ret)
			}
			mu.Lock()
			delete(volUsageCache, volumeId)
			mu.Unlock()
		})
	}

	os.RemoveAll(validPath)
}

type mockMetadata struct {
	availabilityZone string
}

func (m *mockMetadata) GetInstanceID() string       { return "test-instance-id" }
func (m *mockMetadata) GetRegion() string           { return "us-east-1" }
func (m *mockMetadata) GetAvailabilityZone() string { return m.availabilityZone }

func TestNodeGetInfo(t *testing.T) {
	testCases := []struct {
		name              string
		volumeAttachLimit int64
		availabilityZone  string
		needsCloudMock    bool
		expectedResponse  *csi.NodeGetInfoResponse
	}{
		{
			name:              "returns nodeID and volumeAttachLimit",
			volumeAttachLimit: 100,
			availabilityZone:  "",
			expectedResponse: &csi.NodeGetInfoResponse{
				NodeId:            "test-node-id",
				MaxVolumesPerNode: 100,
			},
		},
		{
			name:              "zero volume attach limit",
			volumeAttachLimit: 0,
			availabilityZone:  "",
			expectedResponse: &csi.NodeGetInfoResponse{
				NodeId:            "test-node-id",
				MaxVolumesPerNode: 0,
			},
		},
		{
			name:              "returns topology when availability zone present",
			volumeAttachLimit: 100,
			availabilityZone:  "us-east-1b",
			expectedResponse: &csi.NodeGetInfoResponse{
				NodeId:            "test-node-id",
				MaxVolumesPerNode: 100,
				AccessibleTopology: &csi.Topology{
					Segments: map[string]string{
						"topology.kubernetes.io/zone": "us-east-1b",
					},
				},
			},
		},
		{
			name:              "returns topology for different zone",
			volumeAttachLimit: 50,
			availabilityZone:  "us-west-2a",
			expectedResponse: &csi.NodeGetInfoResponse{
				NodeId:            "test-node-id",
				MaxVolumesPerNode: 50,
				AccessibleTopology: &csi.Topology{
					Segments: map[string]string{
						"topology.kubernetes.io/zone": "us-west-2a",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockCloud := mocks.NewMockCloud(mockCtrl)
			mockCloud.EXPECT().GetMetadata().Return(&mockMetadata{availabilityZone: tc.availabilityZone}).AnyTimes()

			driver := &Driver{
				nodeID:            "test-node-id",
				volumeAttachLimit: tc.volumeAttachLimit,
				cloud:             mockCloud,
			}

			req := &csi.NodeGetInfoRequest{}
			ctx := context.Background()

			ret, err := driver.NodeGetInfo(ctx, req)

			testResult(t, "NodeGetInfo", ret, err, errtyp{})
			if !reflect.DeepEqual(tc.expectedResponse, ret) {
				t.Errorf("Expected: %v, Actual: %v", tc.expectedResponse, ret)
			}
		})
	}
}

func testResponse(t *testing.T, expected, actual *csi.NodeGetVolumeStatsResponse) {
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Expected: %v, Actual: %v", expected, actual)
	}
}

func makeDir(path string) error {
	err := os.MkdirAll(path, os.FileMode(0777))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func TestRemoveNotReadyTaint(t *testing.T) {
	nodeName := "test-node-123"
	testCases := []struct {
		name       string
		setup      func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error)
		expResult  error
		customTest func(t *testing.T, k8sClientGetter func() (kubernetes.Interface, error))
	}{
		{
			name: "missing CSI_NODE_NAME",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				return func() (kubernetes.Interface, error) {
					t.Fatalf("Unexpected call to k8s client getter")
					return nil, nil
				}
			},
			expResult: nil,
		},
		{
			name: "failed to setup k8s client",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				return func() (kubernetes.Interface, error) {
					return nil, fmt.Errorf("Failed setup!")
				}
			},
			expResult: nil,
		},
		{
			name: "failed to get node",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				getNodeMock, _ := getNodeMock(mockCtl, nodeName, nil, fmt.Errorf("Failed to get node!"))

				return func() (kubernetes.Interface, error) {
					return getNodeMock, nil
				}
			},
			expResult: fmt.Errorf("Failed to get node!"),
		},
		{
			name: "no taints to remove",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				getNodeMock, _ := getNodeMock(mockCtl, nodeName, &corev1.Node{}, nil)

				return func() (kubernetes.Interface, error) {
					return getNodeMock, nil
				}
			},
			expResult: nil,
		},
		{
			name: "failed to patch node",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				getNodeMock, mockNode := getNodeMock(mockCtl, nodeName, &corev1.Node{
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key:    AgentNotReadyNodeTaintKey,
								Effect: "NoExecute",
							},
						},
					},
				}, nil)
				mockNode.EXPECT().Patch(gomock.Any(), gomock.Eq(nodeName), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("Failed to patch node!"))

				return func() (kubernetes.Interface, error) {
					return getNodeMock, nil
				}
			},
			expResult: fmt.Errorf("Failed to patch node!"),
		},
		{
			name: "successful taint removal",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)

				nodeWithTaint := &corev1.Node{
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{{Key: AgentNotReadyNodeTaintKey, Effect: "NoSchedule"}},
					},
				}
				nodeWithoutTaint := &corev1.Node{
					Spec: corev1.NodeSpec{Taints: []corev1.Taint{}},
				}

				mockClient := mocks.NewMockKubernetesClient(mockCtl)
				mockCoreV1 := mocks.NewMockCoreV1Interface(mockCtl)
				mockNode := mocks.NewMockNodeInterface(mockCtl)

				mockClient.EXPECT().CoreV1().Return(mockCoreV1).MinTimes(1)
				mockCoreV1.EXPECT().Nodes().Return(mockNode).MinTimes(1)

				mockNode.EXPECT().Get(gomock.Any(), gomock.Eq(nodeName), gomock.Any()).Return(nodeWithTaint, nil)
				mockNode.EXPECT().Patch(gomock.Any(), gomock.Eq(nodeName), gomock.Any(), gomock.Any(), gomock.Any()).Return(nodeWithoutTaint, nil)

				return func() (kubernetes.Interface, error) {
					return mockClient, nil
				}
			},
			expResult: nil,
		},
		{
			name: "patch succeeds with no error but taint still present (verification fails)",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)

				nodeWithTaint := &corev1.Node{
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{{Key: AgentNotReadyNodeTaintKey, Effect: "NoSchedule"}},
					},
				}

				mockClient := mocks.NewMockKubernetesClient(mockCtl)
				mockCoreV1 := mocks.NewMockCoreV1Interface(mockCtl)
				mockNode := mocks.NewMockNodeInterface(mockCtl)

				mockClient.EXPECT().CoreV1().Return(mockCoreV1).MinTimes(1)
				mockCoreV1.EXPECT().Nodes().Return(mockNode).MinTimes(1)

				mockNode.EXPECT().Get(gomock.Any(), gomock.Eq(nodeName), gomock.Any()).Return(nodeWithTaint, nil)
				mockNode.EXPECT().Patch(gomock.Any(), gomock.Eq(nodeName), gomock.Any(), gomock.Any(), gomock.Any()).Return(nodeWithTaint, nil)

				return func() (kubernetes.Interface, error) {
					return mockClient, nil
				}
			},
			// expect verification to show failed startup taint removal
			expResult: fmt.Errorf("taint %s still present after patch", AgentNotReadyNodeTaintKey),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()

			k8sClientGetter := tc.setup(t, mockCtl)
			result := removeNotReadyTaint(k8sClientGetter)
			if !reflect.DeepEqual(result, tc.expResult) {
				t.Fatalf("Expected result `%v`, got result `%v`", tc.expResult, result)
			}
		})
	}
}

func getNodeMock(mockCtl *gomock.Controller, nodeName string, returnNode *corev1.Node, returnError error) (kubernetes.Interface, *mocks.MockNodeInterface) {
	mockClient := mocks.NewMockKubernetesClient(mockCtl)
	mockCoreV1 := mocks.NewMockCoreV1Interface(mockCtl)
	mockNode := mocks.NewMockNodeInterface(mockCtl)

	mockClient.EXPECT().CoreV1().Return(mockCoreV1).MinTimes(1)
	mockCoreV1.EXPECT().Nodes().Return(mockNode).MinTimes(1)
	mockNode.EXPECT().Get(gomock.Any(), gomock.Eq(nodeName), gomock.Any()).Return(returnNode, returnError).MinTimes(1)

	return mockClient, mockNode
}

func TestTryRemoveNotReadyTaintUntilSucceed(t *testing.T) {
	{
		// Registration always passes, taint removal fails then succeeds
		i := 0
		tryRemoveNotReadyTaintUntilSucceed(0, time.Millisecond, 10*time.Millisecond,
			func() error { return nil },
			func() error {
				i++
				if i < 3 {
					return errors.New("test")
				}
				return nil
			})

		if i != 3 {
			t.Fatalf("unexpected result: got %d, want 3", i)
		}
	}
	{
		// Registration always passes, taint removal succeeds immediately
		i := 0
		tryRemoveNotReadyTaintUntilSucceed(0, time.Millisecond, 10*time.Millisecond,
			func() error { return nil },
			func() error {
				i++
				return nil
			})

		if i != 1 {
			t.Fatalf("unexpected result: got %d, want 1", i)
		}
	}
	{
		// Registration fails N times then succeeds, taint removal called once and succeeds
		regCount := 0
		removeCount := 0
		tryRemoveNotReadyTaintUntilSucceed(0, time.Millisecond, 10*time.Millisecond,
			func() error {
				regCount++
				if regCount < 4 {
					return errors.New("not registered yet")
				}
				return nil
			},
			func() error {
				removeCount++
				return nil
			})

		if regCount != 4 {
			t.Fatalf("unexpected registration check count: got %d, want 4", regCount)
		}
		if removeCount != 1 {
			t.Fatalf("unexpected remove count: got %d, want 1", removeCount)
		}
	}
	{
		// Registration fails then succeeds, taint removal fails then succeeds
		regCount := 0
		removeCount := 0
		tryRemoveNotReadyTaintUntilSucceed(0, time.Millisecond, 10*time.Millisecond,
			func() error {
				regCount++
				if regCount < 3 {
					return errors.New("not registered yet")
				}
				return nil
			},
			func() error {
				removeCount++
				if removeCount < 2 {
					return errors.New("patch conflict")
				}
				return nil
			})

		// Registration should be called: 2 fails + 1 success for first remove attempt + 1 success for second remove attempt = 4
		if regCount < 3 {
			t.Fatalf("unexpected registration check count: got %d, want >= 3", regCount)
		}
		if removeCount != 2 {
			t.Fatalf("unexpected remove count: got %d, want 2", removeCount)
		}
	}
}

// Run a test in subprocess that may call os.Exit or klog.Fatal.
func runForkFatalTest(testName string) error {
	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%v", testName))
	// Fork off the process
	cmd.Env = append(os.Environ(), "FORK=1")
	err := cmd.Run()
	return err
}

func TestGetMaxInflightMountCalls(t *testing.T) {
	testCases := []struct {
		name                       string
		maxInflightMountCallsOptIn bool
		maxInflightMountCalls      int64
		expected                   int64
	}{
		{
			name:                       "opt-in false returns unset",
			maxInflightMountCallsOptIn: false,
			maxInflightMountCalls:      10,
			expected:                   UnsetMaxInflightMountCounts,
		},
		{
			name:                       "opt-in true with valid value",
			maxInflightMountCallsOptIn: true,
			maxInflightMountCalls:      5,
			expected:                   5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getMaxInflightMountCalls(tc.maxInflightMountCallsOptIn, tc.maxInflightMountCalls)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestGetVolumeAttachLimit(t *testing.T) {
	testCases := []struct {
		name                   string
		volumeAttachLimitOptIn bool
		volumeAttachLimit      int64
		expected               int64
		expectFatal            bool
	}{
		{
			name:                   "opt-in false returns zero",
			volumeAttachLimitOptIn: false,
			volumeAttachLimit:      100,
			expected:               0,
		},
		{
			name:                   "opt-in true with valid value",
			volumeAttachLimitOptIn: true,
			volumeAttachLimit:      50,
			expected:               50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getVolumeAttachLimit(tc.volumeAttachLimitOptIn, tc.volumeAttachLimit)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestIsValidAccessPointId(t *testing.T) {
	testCases := []struct {
		name     string
		apid     string
		expected bool
	}{
		{"valid: 8 chars", "fsap-12345678", true},
		{"valid: 16 chars", "fsap-1234567890abcdef", true},
		{"valid: 40 chars", "fsap-1234567890abcdef1234567890abcdef1234", true},
		{"invalid: too short", "fsap-1234567", false},
		{"invalid: too long", "fsap-1234567890abcdef1234567890abcdef1234567890", false},
		{"invalid: no prefix", "1234567890abcdef", false},
		{"invalid: uppercase hex", "fsap-1234567G", false},
		{"invalid: non-hex chars", "fsap-123456zz", false},
		{"invalid: empty", "", false},
		{"invalid: prefix only", "fsap-", false},
		{"invalid: attack with mount options", "fsap-attacktest,exec,suid,dev", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidAccessPointId(tc.apid)
			if result != tc.expected {
				t.Errorf("isValidAccessPointId(%q) = %v, expected %v", tc.apid, result, tc.expected)
			}
		})
	}
}

func TestIsValidFileSystemId(t *testing.T) {
	testCases := []struct {
		name     string
		fsid     string
		expected bool
	}{
		{"valid: 8 chars", "fs-12345678", true},
		{"valid: 16 chars", "fs-1234567890abcdef", true},
		{"valid: 40 chars", "fs-1234567890abcdef1234567890abcdef1234", true},
		{"invalid: too short", "fs-1234567", false},
		{"invalid: too long", "fs-1234567890abcdef1234567890abcdef1234567890", false},
		{"invalid: no prefix", "1234567890abcdef", false},
		{"invalid: uppercase hex", "fs-1234567G", false},
		{"invalid: non-hex chars", "fs-123456zz", false},
		{"invalid: empty", "", false},
		{"invalid: prefix only", "fs-", false},
		{"invalid: attack with mount options", "fs-attacktest,exec,suid,dev", false},
		// DNS-name form used in static PV volumeHandles. Validation is structural
		// (optional AZ label, strict fs-<hex> id label, DNS-safe domain) rather
		// than enumerating known domains, so EFS bare, EFS AZ-prefixed, and S3
		// Files forms are all covered while still blocking mount-option injection.
		{"valid: efs dns-name", "fs-9919e11b.efs.us-east-1.amazonaws.com", true},
		{"valid: efs dns-name az-prefixed", "us-east-1a.fs-9919e11b.efs.us-east-1.amazonaws.com", true},
		{"valid: s3files dns-name", "use1-az1.fs-0123456abcdef0189.s3files.us-east-1.on.aws", true},
		{"valid: dns-name china", "fs-12345678.efs.cn-north-1.amazonaws.com.cn", true},
		{"valid: dns-name fips", "fs-12345678.efs-fips.us-east-1.amazonaws.com", true},
		{"invalid: injected mount options on s3files dns-name", "use1-az1.fs-0123456abcdef0189.s3files.us-east-1.on.aws,exec,suid,dev", false},
		{"invalid: dns-name az-prefixed id too short", "us-east-1a.fs-1234567.efs.us-east-1.amazonaws.com", false},
		{"invalid: dns-name az-prefixed non-hex id", "us-east-1a.fs-1234567G.efs.us-east-1.amazonaws.com", false},
		// DNS labels are case-insensitive, so an uppercase domain must be accepted,
		// but the fs-<hex> id label itself stays strictly lowercase hex: an
		// uppercase id label is rejected in the DNS branch just as the bare id
		// fs-ABCDEF12 is rejected in the bare-id branch.
		{"valid: efs dns-name uppercase domain", "fs-9919e11b.EFS.us-east-1.amazonaws.com", true},
		{"invalid: dns-name uppercase id label", "fs-ABCDEF12.efs.us-east-1.amazonaws.com", false},
		{"invalid: bare id uppercase hex", "fs-ABCDEF12", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidFileSystemId(tc.fsid)
			if result != tc.expected {
				t.Errorf("isValidFileSystemId(%q) = %v, expected %v", tc.fsid, result, tc.expected)
			}
		})
	}
}

func TestParseVolumeIdDnsName(t *testing.T) {
	// A DNS-name volumeHandle (used in static PVs) must parse successfully, keep
	// the fsid byte-for-byte, and produce an unchanged efs-utils mount source.
	// Both EFS (fs-<id> as the leading label) and S3 Files (fs-<id> as a middle
	// label with a different domain) forms are covered.
	testCases := []struct {
		name            string
		volumeId        string
		expectedFsid    string
		expectedFsType  util.FileSystemType
		expectedSubpath string
	}{
		{
			name:            "efs dns-name",
			volumeId:        "fs-9919e11b.efs.us-east-1.amazonaws.com",
			expectedFsid:    "fs-9919e11b.efs.us-east-1.amazonaws.com",
			expectedFsType:  util.FileSystemTypeEFS,
			expectedSubpath: "/",
		},
		{
			name:            "s3files dns-name",
			volumeId:        "s3files:use1-az1.fs-0123456abcdef0189.s3files.us-east-1.on.aws",
			expectedFsid:    "use1-az1.fs-0123456abcdef0189.s3files.us-east-1.on.aws",
			expectedFsType:  util.FileSystemTypeS3Files,
			expectedSubpath: "/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fsid, subpath, apid, fsType, err := parseVolumeId(tc.volumeId)
			if err != nil {
				t.Fatalf("parseVolumeId(%q) returned unexpected error: %v", tc.volumeId, err)
			}
			if fsid != tc.expectedFsid {
				t.Errorf("parseVolumeId(%q) fsid = %q, expected %q (must be unchanged)", tc.volumeId, fsid, tc.expectedFsid)
			}
			if apid != "" {
				t.Errorf("parseVolumeId(%q) apid = %q, expected empty", tc.volumeId, apid)
			}
			if fsType != tc.expectedFsType {
				t.Errorf("parseVolumeId(%q) fsType = %q, expected %q", tc.volumeId, fsType, tc.expectedFsType)
			}

			// The efs-utils mount source is built in NodePublishVolume (asserted
			// end-to-end via the gomock Mount expectation in TestNodePublishVolume).
			// Here we only assert that parseVolumeId keeps the fsid unchanged and
			// derives the subpath correctly, which are the inputs to that source.
			if subpath == "" {
				subpath = "/"
			}
			if subpath != tc.expectedSubpath {
				t.Errorf("parseVolumeId(%q) subpath = %q, expected %q", tc.volumeId, subpath, tc.expectedSubpath)
			}
		})
	}
}

func TestIsValidMountTargetIP(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected bool
	}{
		{"valid: IPv4", "10.0.1.1", true},
		{"valid: IPv6", "2600:1f14:abc:1234::1", true},
		{"valid: IPv6 loopback", "::1", true},
		{"invalid: trailing mount options", "10.0.1.1,uid=0,gid=0,port=12345", false},
		{"invalid: leading mount option", "uid=0,10.0.1.1", false},
		{"invalid: whitespace separated option", "10.0.1.1 uid=0", false},
		{"invalid: hostname", "fs-abcd1234.efs.us-east-1.amazonaws.com", false},
		{"invalid: not an IP", "not-an-ip", false},
		{"invalid: empty", "", false},
		{"invalid: IPv4 with port", "10.0.1.1:2049", false},
		{"invalid: CIDR", "10.0.1.0/24", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidMountTargetIP(tc.value)
			if result != tc.expected {
				t.Errorf("isValidMountTargetIP(%q) = %v, expected %v", tc.value, result, tc.expected)
			}
		})
	}
}

func TestBuildVolumeId(t *testing.T) {
	testCases := []struct {
		name     string
		fsType   util.FileSystemType
		fsid     string
		apid     string
		expected string
	}{
		{
			name:     "EFS omits the type prefix",
			fsType:   util.FileSystemTypeEFS,
			fsid:     "fs-abcd1234",
			apid:     "fsap-abcd1234",
			expected: "fs-abcd1234::fsap-abcd1234",
		},
		{
			name:     "S3 Files carries the type prefix",
			fsType:   util.FileSystemTypeS3Files,
			fsid:     "fs-abcd1234",
			apid:     "fsap-abcd1234",
			expected: "s3files:fs-abcd1234::fsap-abcd1234",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildVolumeId(tc.fsType, tc.fsid, tc.apid)
			if got != tc.expected {
				t.Fatalf("buildVolumeId(%q, %q, %q) = %q, expected %q", tc.fsType, tc.fsid, tc.apid, got, tc.expected)
			}

			// parseVolumeId is the inverse, so every handle this driver emits must
			// resolve back to the same file system, access point and type.
			fsid, subpath, apid, fsType, err := parseVolumeId(got)
			if err != nil {
				t.Fatalf("parseVolumeId(%q) returned an error: %v", got, err)
			}
			if fsid != tc.fsid || apid != tc.apid || fsType != tc.fsType || subpath != "" {
				t.Errorf("parseVolumeId(%q) = (%q, %q, %q, %q), expected (%q, \"\", %q, %q)",
					got, fsid, subpath, apid, fsType, tc.fsid, tc.apid, tc.fsType)
			}
		})
	}
}

func TestGetCsiNodeEfsPluginContainerMemoryLimitInBytes(t *testing.T) {
	testCases := []struct {
		name     string
		envValue string
		expected int64
	}{
		{"env not set", "", -1},
		{"valid value", "1073741824", 1073741824},
		{"invalid value", "notanumber", -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envValue != "" {
				t.Setenv("CSI_NODE_MEMORY_LIMIT", tc.envValue)
			} else {
				os.Unsetenv("CSI_NODE_MEMORY_LIMIT")
			}
			result := getCsiNodeEfsPluginContainerMemoryLimitInBytes()
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestNodePublishVolumeMountTargetIpMap(t *testing.T) {
	stdVolCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}

	testCases := []struct {
		name      string
		nodeAZ    string
		ipMapJSON string
		// expected mounttargetip value in mount options, empty if none expected
		expectedIP  string
		expectError bool
		// substring the error must contain, checked only when set
		expectErrContains string
		// gRPC code the error must carry, checked only when set. kubelet retry
		// behavior depends on this, so a rejection is not enough on its own.
		expectErrCode codes.Code
	}{
		{
			name:       "node AZ found in map",
			nodeAZ:     "us-west-2b",
			ipMapJSON:  `{"us-west-2a":"10.0.1.1","us-west-2b":"10.0.2.1","us-west-2c":"10.0.3.1"}`,
			expectedIP: "10.0.2.1",
		},
		{
			name:       "node AZ not in map falls back to a random available AZ",
			nodeAZ:     "us-west-2d",
			ipMapJSON:  `{"us-west-2a":"10.0.1.1"}`,
			expectedIP: "10.0.1.1",
		},
		{
			name:        "invalid JSON returns error",
			nodeAZ:      "us-west-2a",
			ipMapJSON:   `{invalid`,
			expectError: true,
		},
		{
			name:              "comma in the node AZ value is rejected instead of injecting mount options",
			nodeAZ:            "us-west-2a",
			ipMapJSON:         `{"us-west-2a":"10.0.1.1,uid=0,gid=0,port=12345"}`,
			expectError:       true,
			expectErrCode:     codes.InvalidArgument,
			expectErrContains: `has an invalid IP address "10.0.1.1,uid=0,gid=0,port=12345" for availability zone "us-west-2a"`,
		},
		{
			// The entry this node would not have selected is still rejected, so a
			// tampered map cannot mount on some nodes and inject on others.
			name:              "comma in another AZ value is rejected even though this node resolves elsewhere",
			nodeAZ:            "us-west-2b",
			ipMapJSON:         `{"us-west-2a":"10.0.1.1,uid=0","us-west-2b":"10.0.2.1"}`,
			expectError:       true,
			expectErrCode:     codes.InvalidArgument,
			expectErrContains: `has an invalid IP address "10.0.1.1,uid=0" for availability zone "us-west-2a"`,
		},
		{
			// The escape is decoded before validation, so escaping the separator
			// does not get it past the check.
			name:              "unicode-escaped comma is rejected",
			nodeAZ:            "us-west-2a",
			ipMapJSON:         `{"us-west-2a":"10.0.1.1\u002cuid=0"}`,
			expectError:       true,
			expectErrCode:     codes.InvalidArgument,
			expectErrContains: `has an invalid IP address "10.0.1.1,uid=0"`,
		},
		{
			name:              "non-IP value is rejected",
			nodeAZ:            "us-west-2a",
			ipMapJSON:         `{"us-west-2a":"not-an-ip"}`,
			expectError:       true,
			expectErrCode:     codes.InvalidArgument,
			expectErrContains: `has an invalid IP address "not-an-ip"`,
		},
		{
			name:              "empty value is rejected",
			nodeAZ:            "us-west-2a",
			ipMapJSON:         `{"us-west-2a":""}`,
			expectError:       true,
			expectErrCode:     codes.InvalidArgument,
			expectErrContains: `has an invalid IP address ""`,
		},
		{
			// JSON null decodes to the empty string rather than failing to unmarshal.
			name:              "null value is rejected",
			nodeAZ:            "us-west-2a",
			ipMapJSON:         `{"us-west-2a":null}`,
			expectError:       true,
			expectErrCode:     codes.InvalidArgument,
			expectErrContains: `has an invalid IP address ""`,
		},
		{
			// A type error leaves the map partially populated, so the unmarshal error
			// must be returned before any entry is read.
			name:          "non-string value is rejected before the map is used",
			nodeAZ:        "us-west-2a",
			ipMapJSON:     `{"us-west-2a":"10.0.1.1","us-west-2b":123}`,
			expectError:   true,
			expectErrCode: codes.InvalidArgument,
		},
		{
			name:       "IPv6 address is accepted",
			nodeAZ:     "us-west-2a",
			ipMapJSON:  `{"us-west-2a":"2600:1f14:abc:1234::1"}`,
			expectedIP: "2600:1f14:abc:1234::1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			t.Setenv("CSI_NODE_MEMORY_LIMIT", strconv.Itoa(minMemoryInBytesToEnableS3ReadCache*2))

			mockMounter := mocks.NewMockMounter(mockCtrl)
			mockCloud := mocks.NewMockCloud(mockCtrl)
			mockCloud.EXPECT().GetMetadata().Return(&mockMetadata{availabilityZone: tc.nodeAZ}).AnyTimes()

			driver := &Driver{
				endpoint:             "endpoint",
				nodeID:               "nodeID",
				mounter:              mockMounter,
				cloud:                mockCloud,
				volMetricsOptIn:      true,
				nodeCaps:             SetNodeCapOptInFeatures(true),
				inFlightMountTracker: NewInFlightMountTracker(UnsetMaxInflightMountCounts),
			}
			ctx := context.Background()

			req := &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext: map[string]string{
					"mounttargetipmap": tc.ipMapJSON,
				},
			}

			if !tc.expectError {
				expectedMountOpts := []string{"mounttargetip=" + tc.expectedIP, "tls"}
				mockMounter.EXPECT().MakeDir(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().Mount(volumeId+":/", targetPath, "efs", expectedMountOpts).Return(nil)
			}

			ret, err := driver.NodePublishVolume(ctx, req)
			if tc.expectError {
				if err == nil {
					t.Fatal("Expected error but got nil")
				}
				if tc.expectErrCode != codes.OK && status.Code(err) != tc.expectErrCode {
					t.Fatalf("Expected gRPC code %v, got %v (%v)", tc.expectErrCode, status.Code(err), err)
				}
				if tc.expectErrContains != "" && !strings.Contains(err.Error(), tc.expectErrContains) {
					t.Fatalf("Expected error containing %q, got %q", tc.expectErrContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if ret == nil {
					t.Fatal("Expected non-nil return value")
				}
			}
		})
	}
}

func TestCheckDriverRegistration(t *testing.T) {
	nodeName := "test-node-123"
	testDriverName := "efs.csi.aws.com"
	int32Val := int32(10)

	testCases := []struct {
		name      string
		setup     func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error)
		expectErr bool
		errSubstr string
	}{
		{
			name: "CSI_NODE_NAME not set",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", "")
				return func() (kubernetes.Interface, error) {
					t.Fatalf("Unexpected call to k8s client getter")
					return nil, nil
				}
			},
			expectErr: true,
			errSubstr: "CSI_NODE_NAME not set",
		},
		{
			name: "k8s client creation fails",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				return func() (kubernetes.Interface, error) {
					return nil, fmt.Errorf("failed to create client")
				}
			},
			expectErr: true,
			errSubstr: "failed to create kubernetes client",
		},
		{
			name: "CSINode Get returns not-found",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				mockClient := mocks.NewMockKubernetesClient(mockCtl)
				mockStorageV1 := mocks.NewMockStorageV1Interface(mockCtl)
				mockCSINode := mocks.NewMockCSINodeInterface(mockCtl)

				mockClient.EXPECT().StorageV1().Return(mockStorageV1).MinTimes(1)
				mockStorageV1.EXPECT().CSINodes().Return(mockCSINode).MinTimes(1)
				mockCSINode.EXPECT().Get(gomock.Any(), gomock.Eq(nodeName), gomock.Any()).Return(nil,
					apierrors.NewNotFound(schema.GroupResource{Group: "storage.k8s.io", Resource: "csinodes"}, nodeName))

				return func() (kubernetes.Interface, error) {
					return mockClient, nil
				}
			},
			expectErr: true,
			errSubstr: "failed to get CSINode",
		},
		{
			name: "CSINode Get returns forbidden",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				mockClient := mocks.NewMockKubernetesClient(mockCtl)
				mockStorageV1 := mocks.NewMockStorageV1Interface(mockCtl)
				mockCSINode := mocks.NewMockCSINodeInterface(mockCtl)

				mockClient.EXPECT().StorageV1().Return(mockStorageV1).MinTimes(1)
				mockStorageV1.EXPECT().CSINodes().Return(mockCSINode).MinTimes(1)
				mockCSINode.EXPECT().Get(gomock.Any(), gomock.Eq(nodeName), gomock.Any()).Return(nil,
					apierrors.NewForbidden(schema.GroupResource{Group: "storage.k8s.io", Resource: "csinodes"}, nodeName, fmt.Errorf("forbidden")))

				return func() (kubernetes.Interface, error) {
					return mockClient, nil
				}
			},
			expectErr: true,
			errSubstr: "failed to get CSINode",
		},
		{
			name: "CSINode exists but driver not listed",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				mockClient := mocks.NewMockKubernetesClient(mockCtl)
				mockStorageV1 := mocks.NewMockStorageV1Interface(mockCtl)
				mockCSINode := mocks.NewMockCSINodeInterface(mockCtl)

				mockClient.EXPECT().StorageV1().Return(mockStorageV1).MinTimes(1)
				mockStorageV1.EXPECT().CSINodes().Return(mockCSINode).MinTimes(1)
				mockCSINode.EXPECT().Get(gomock.Any(), gomock.Eq(nodeName), gomock.Any()).Return(&storagev1.CSINode{
					Spec: storagev1.CSINodeSpec{
						Drivers: []storagev1.CSINodeDriver{
							{Name: "some.other.driver"},
						},
					},
				}, nil)

				return func() (kubernetes.Interface, error) {
					return mockClient, nil
				}
			},
			expectErr: true,
			errSubstr: "not yet listed in CSINode",
		},
		{
			name: "CSINode exists, driver listed, Allocatable nil",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				mockClient := mocks.NewMockKubernetesClient(mockCtl)
				mockStorageV1 := mocks.NewMockStorageV1Interface(mockCtl)
				mockCSINode := mocks.NewMockCSINodeInterface(mockCtl)

				mockClient.EXPECT().StorageV1().Return(mockStorageV1).MinTimes(1)
				mockStorageV1.EXPECT().CSINodes().Return(mockCSINode).MinTimes(1)
				mockCSINode.EXPECT().Get(gomock.Any(), gomock.Eq(nodeName), gomock.Any()).Return(&storagev1.CSINode{
					Spec: storagev1.CSINodeSpec{
						Drivers: []storagev1.CSINodeDriver{
							{
								Name:        testDriverName,
								Allocatable: nil,
							},
						},
					},
				}, nil)

				return func() (kubernetes.Interface, error) {
					return mockClient, nil
				}
			},
			expectErr: true,
			errSubstr: "Allocatable not yet set",
		},
		{
			name: "CSINode exists, driver listed, Allocatable set - success",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				mockClient := mocks.NewMockKubernetesClient(mockCtl)
				mockStorageV1 := mocks.NewMockStorageV1Interface(mockCtl)
				mockCSINode := mocks.NewMockCSINodeInterface(mockCtl)

				mockClient.EXPECT().StorageV1().Return(mockStorageV1).MinTimes(1)
				mockStorageV1.EXPECT().CSINodes().Return(mockCSINode).MinTimes(1)
				mockCSINode.EXPECT().Get(gomock.Any(), gomock.Eq(nodeName), gomock.Any()).Return(&storagev1.CSINode{
					Spec: storagev1.CSINodeSpec{
						Drivers: []storagev1.CSINodeDriver{
							{
								Name: testDriverName,
								Allocatable: &storagev1.VolumeNodeResources{
									Count: &int32Val,
								},
							},
						},
					},
				}, nil)

				return func() (kubernetes.Interface, error) {
					return mockClient, nil
				}
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()

			k8sClientGetter := tc.setup(t, mockCtl)
			err := checkDriverRegistration(k8sClientGetter, testDriverName)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("Expected error containing %q, got nil", tc.errSubstr)
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("Expected error containing %q, got: %v", tc.errSubstr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected nil error, got: %v", err)
				}
			}
		})
	}
}

func TestEfsMetaFile(t *testing.T) {
	stdVolCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}

	newDriver := func(t *testing.T, mounter Mounter) *Driver {
		t.Helper()
		return &Driver{
			endpoint:             "endpoint",
			nodeID:               "nodeID",
			mounter:              mounter,
			volMetricsOptIn:      true,
			volStatter:           NewVolStatter(),
			nodeCaps:             SetNodeCapOptInFeatures(true),
			inFlightMountTracker: NewInFlightMountTracker(UnsetMaxInflightMountCounts),
			metaDir:              filepath.Join(t.TempDir(), "mounts"),
		}
	}

	setupPublish := func(t *testing.T) (driver *Driver, ctrl *gomock.Controller, target string) {
		t.Helper()
		t.Setenv("CSI_NODE_MEMORY_LIMIT", strconv.Itoa(minMemoryInBytesToEnableS3ReadCache*2))
		ctrl = gomock.NewController(t)
		target = filepath.Join(t.TempDir(), "mount")

		mockMounter := mocks.NewMockMounter(ctrl)
		mockMounter.EXPECT().MakeDir(gomock.Eq(target)).Return(nil)
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		return newDriver(t, mockMounter), ctrl, target
	}

	publishReq := func(target string) *csi.NodePublishVolumeRequest {
		return &csi.NodePublishVolumeRequest{
			VolumeId:         volumeId,
			VolumeCapability: stdVolCap,
			TargetPath:       target,
		}
	}

	publish := func(t *testing.T, driver *Driver, req *csi.NodePublishVolumeRequest) {
		t.Helper()
		if _, err := driver.NodePublishVolume(context.Background(), req); err != nil {
			t.Fatalf("NodePublishVolume failed: %v", err)
		}
	}

	readMeta := func(t *testing.T, driver *Driver, target string) efsVolumeMeta {
		t.Helper()
		metaPath := driver.efsMetaPath(target)
		data, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatalf("expected meta file %s to exist: %v", metaPath, err)
		}
		var meta efsVolumeMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			t.Fatalf("failed to parse meta file: %v", err)
		}
		return meta
	}

	seedMeta := func(t *testing.T, driver *Driver, target string) {
		t.Helper()
		metaPath := driver.efsMetaPath(target)
		if err := os.MkdirAll(filepath.Dir(metaPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(metaPath, []byte(`{"schemaVersion":1}`), 0600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("publish writes meta file with all fields", func(t *testing.T) {
		driver, _, target := setupPublish(t)

		req := publishReq(target)
		req.VolumeId = "fs-abc12345:/data:fsap-deadbeef"
		req.Readonly = true
		req.VolumeContext = map[string]string{"mounttargetip": "10.0.0.5"}
		publish(t, driver, req)

		meta := readMeta(t, driver, target)
		if meta.SchemaVersion != 1 {
			t.Errorf("schemaVersion = %d, want 1", meta.SchemaVersion)
		}
		if meta.Target != target {
			t.Errorf("target = %q, want %q", meta.Target, target)
		}
		if meta.FsType != "efs" {
			t.Errorf("fsType = %q, want %q", meta.FsType, "efs")
		}
		if meta.VolumeHandle.FileSystemID != "fs-abc12345" {
			t.Errorf("volumeHandle.fileSystemId = %q, want %q", meta.VolumeHandle.FileSystemID, "fs-abc12345")
		}
		if meta.VolumeHandle.ExportPath != "/data" {
			t.Errorf("volumeHandle.exportPath = %q, want %q", meta.VolumeHandle.ExportPath, "/data")
		}
		if meta.VolumeHandle.AccessPointID != "fsap-deadbeef" {
			t.Errorf("volumeHandle.accessPointId = %q, want %q", meta.VolumeHandle.AccessPointID, "fsap-deadbeef")
		}
		if meta.VolumeContext.MountTargetIP != "10.0.0.5" {
			t.Errorf("volumeContext.mountTargetIp = %q, want %q", meta.VolumeContext.MountTargetIP, "10.0.0.5")
		}
		if !meta.VolumeContext.EncryptInTransit {
			t.Error("volumeContext.encryptInTransit = false, want true")
		}
		if !meta.ReadOnly {
			t.Error("readOnly = false, want true")
		}
		if !hasOption(meta.MountFlags, "tls") ||
			!hasOption(meta.MountFlags, "accesspoint=fsap-deadbeef") {
			t.Errorf("mountFlags missing expected entries: %v", meta.MountFlags)
		}
	})

	t.Run("publish without mounttargetip omits field", func(t *testing.T) {
		driver, _, target := setupPublish(t)
		publish(t, driver, publishReq(target))

		data, err := os.ReadFile(driver.efsMetaPath(target))
		if err != nil {
			t.Fatalf("expected meta file to exist: %v", err)
		}
		var raw struct {
			VolumeContext map[string]interface{} `json:"volumeContext"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("failed to parse meta: %v", err)
		}
		if _, ok := raw.VolumeContext["mountTargetIp"]; ok {
			t.Error("volumeContext.mountTargetIp should be omitted when no IP was resolved")
		}
	})

	t.Run("publish with trailing slash writes metadata outside the mount target", func(t *testing.T) {
		t.Setenv("CSI_NODE_MEMORY_LIMIT", strconv.Itoa(minMemoryInBytesToEnableS3ReadCache*2))
		target := filepath.Join(t.TempDir(), "mount") + string(os.PathSeparator)
		if err := os.MkdirAll(filepath.Clean(target), 0755); err != nil {
			t.Fatal(err)
		}

		mockMounter := mocks.NewMockMounter(gomock.NewController(t))
		mockMounter.EXPECT().MakeDir(target).Return(nil)
		mockMounter.EXPECT().Mount(gomock.Any(), target, gomock.Any(), gomock.Any()).Return(nil)

		driver := newDriver(t, mockMounter)
		publish(t, driver, publishReq(target))

		readMeta(t, driver, target)

		entries, err := os.ReadDir(filepath.Clean(target))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("mount target should be empty, found %d entries", len(entries))
		}
	})

	for _, tc := range []struct {
		name     string
		refCount int
		seedMeta bool
	}{
		{name: "unpublish removes meta file", refCount: 1, seedMeta: true},
		{name: "unpublish tolerates missing meta file", refCount: 1},
		{name: "unpublish removes metadata when target is already unmounted", seedMeta: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "mount")
			mockMounter := mocks.NewMockMounter(gomock.NewController(t))
			mockMounter.EXPECT().GetDeviceName(target).Return("", tc.refCount, nil)
			if tc.refCount > 0 {
				mockMounter.EXPECT().Unmount(target).Return(nil)
			}

			driver := newDriver(t, mockMounter)
			metaPath := driver.efsMetaPath(target)
			if tc.seedMeta {
				seedMeta(t, driver, target)
			}

			req := &csi.NodeUnpublishVolumeRequest{VolumeId: volumeId, TargetPath: target}
			if _, err := driver.NodeUnpublishVolume(context.Background(), req); err != nil {
				t.Fatalf("NodeUnpublishVolume failed: %v", err)
			}
			if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
				t.Errorf("expected meta file to be absent after unpublish, got err=%v", err)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		nodeAZ string
		ipMap  string
		wantIP string
	}{
		{
			name:   "publish with mounttargetipmap writes resolved IP for node AZ",
			nodeAZ: "us-west-2b",
			ipMap:  `{"us-west-2a":"10.0.1.1","us-west-2b":"10.0.2.1"}`,
			wantIP: "10.0.2.1",
		},
		{
			name:   "publish with mounttargetipmap falls back to any IP when node AZ absent",
			nodeAZ: "us-west-2c",
			ipMap:  `{"us-west-2a":"10.0.1.1"}`,
			wantIP: "10.0.1.1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			driver, ctrl, target := setupPublish(t)
			mockCloud := mocks.NewMockCloud(ctrl)
			mockCloud.EXPECT().GetMetadata().Return(&mockMetadata{availabilityZone: tc.nodeAZ}).AnyTimes()
			driver.cloud = mockCloud

			req := publishReq(target)
			req.VolumeContext = map[string]string{"mounttargetipmap": tc.ipMap}
			publish(t, driver, req)

			if got := readMeta(t, driver, target).VolumeContext.MountTargetIP; got != tc.wantIP {
				t.Errorf("volumeContext.mountTargetIp = %q, want %q", got, tc.wantIP)
			}
		})
	}

	t.Run("publish with crossaccount writes crossAccount field", func(t *testing.T) {
		driver, _, target := setupPublish(t)

		req := publishReq(target)
		req.VolumeContext = map[string]string{"crossaccount": "true"}
		publish(t, driver, req)

		if meta := readMeta(t, driver, target); !meta.VolumeContext.CrossAccount {
			t.Error("volumeContext.crossAccount = false, want true")
		}
	})

	for _, tc := range []struct {
		name       string
		volumeID   string
		mountFlags []string
		wantFsType string
		wantIAM    bool
	}{
		{
			name:       "publish with explicit iam records iam=true",
			volumeID:   volumeId,
			mountFlags: []string{"iam"},
			wantFsType: "efs",
			wantIAM:    true,
		},
		{
			name:       "publish with S3 Files implies iam=true",
			volumeID:   "s3files:fs-abcd1234::fsap-abcd1234",
			wantFsType: "s3files",
			wantIAM:    true,
		},
		{
			name:       "publish with access point but no iam option records iam=false",
			volumeID:   volumeId + "::fsap-deadbeef",
			wantFsType: "efs",
		},
		{
			name:       "publish plain EFS records iam=false",
			volumeID:   volumeId,
			wantFsType: "efs",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			driver, _, target := setupPublish(t)
			req := publishReq(target)
			req.VolumeId = tc.volumeID
			if tc.mountFlags != nil {
				req.VolumeCapability = &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{MountFlags: tc.mountFlags},
					},
					AccessMode: stdVolCap.AccessMode,
				}
			}
			publish(t, driver, req)

			meta := readMeta(t, driver, target)
			if meta.FsType != tc.wantFsType {
				t.Errorf("fsType = %q, want %q", meta.FsType, tc.wantFsType)
			}
			if meta.Iam != tc.wantIAM {
				t.Errorf("iam = %t, want %t", meta.Iam, tc.wantIAM)
			}
			if tc.wantIAM && len(tc.mountFlags) > 0 && !hasOption(meta.MountFlags, "iam") {
				t.Errorf("mountFlags missing iam: %v", meta.MountFlags)
			}
		})
	}

	t.Run("publish does not write meta file when mount fails", func(t *testing.T) {
		t.Setenv("CSI_NODE_MEMORY_LIMIT", strconv.Itoa(minMemoryInBytesToEnableS3ReadCache*2))
		target := filepath.Join(t.TempDir(), "mount")

		mockMounter := mocks.NewMockMounter(gomock.NewController(t))
		mockMounter.EXPECT().MakeDir(gomock.Eq(target)).Return(nil)
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("mount boom"))

		driver := newDriver(t, mockMounter)
		if _, err := driver.NodePublishVolume(context.Background(), publishReq(target)); err == nil {
			t.Fatal("NodePublishVolume: expected error on mount failure")
		}

		if _, err := os.Stat(driver.efsMetaPath(target)); !os.IsNotExist(err) {
			t.Errorf("meta file should not exist after mount failure, got err=%v", err)
		}
	})

	t.Run("publish succeeds when meta write fails", func(t *testing.T) {
		t.Setenv("CSI_NODE_MEMORY_LIMIT", strconv.Itoa(minMemoryInBytesToEnableS3ReadCache*2))
		target := filepath.Join(t.TempDir(), "mount")

		mockMounter := mocks.NewMockMounter(gomock.NewController(t))
		mockMounter.EXPECT().MakeDir(gomock.Eq(target)).Return(nil)
		mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		driver := newDriver(t, mockMounter)
		// Put a regular file where MkdirAll expects a parent directory. This
		// makes MkdirAll fail with ENOTDIR for any UID (including root),
		// which is portable across CI environments.
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, nil, 0600); err != nil {
			t.Fatal(err)
		}
		driver.metaDir = filepath.Join(blocker, "mounts")

		if _, err := driver.NodePublishVolume(context.Background(), publishReq(target)); err != nil {
			t.Fatalf("NodePublishVolume should succeed on meta-write failure: %v", err)
		}
		// Any stat error is fine here — the meta file must not have been
		// written. ENOENT is expected on non-root; ENOTDIR is what we get
		// via the blocker-file mechanism.
		if _, err := os.Stat(driver.efsMetaPath(target)); err == nil {
			t.Error("meta file should not exist after meta-write failure")
		}
	})

	t.Run("meta path is stable under filepath.Clean", func(t *testing.T) {
		driver := newDriver(t, nil)
		a := driver.efsMetaPath("/var/lib/kubelet/pods/uid/volumes/kubernetes.io~csi/pv/mount")
		b := driver.efsMetaPath("/var/lib/kubelet/pods/uid/volumes/kubernetes.io~csi/pv/mount/")
		c := driver.efsMetaPath("/var/lib/kubelet/pods/uid/volumes/kubernetes.io~csi//pv/mount")
		if a != b || a != c {
			t.Errorf("efsMetaPath should be stable under trailing slashes and duplicate separators:\n  a=%s\n  b=%s\n  c=%s", a, b, c)
		}
	})
}
