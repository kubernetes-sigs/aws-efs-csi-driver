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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	volumeId   = "fs-abc123"
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
			// TODO: Validate deprecation warning
			name: "success: normal with path in volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"path": "/a/b"},
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/a/b", targetPath, "efs", []string{"tls"}},
			mountSuccess:          true,
			maxInflightMountCalls: UnsetMaxInflightMountCounts,
		},
		{
			name: "fail: path in volume context must be absolute",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"path": "a/b"},
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: `Volume context property "path" must be an absolute path`,
			},
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
			name: "success: path in volume handle takes precedence",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId + ":/a/b/",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"path": "/c/d"},
			},
			expectMakeDir:         true,
			mountArgs:             []interface{}{volumeId + ":/a/b", targetPath, "efs", []string{"tls"}},
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
				message: "volume ID 'fs-abc123:/a/b/::four!' is invalid: Expected at most three fields separated by ':'",
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
				message: `Could not mount "fs-abc123:/" at "/target/path": failed to Mount`,
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
			name: "fail: invalid filesystem ID",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "invalid-id",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectError: errtyp{
				code:    "InvalidArgument",
				message: "volume ID 'invalid-id' is invalid: Expected a file system ID of the form 'fs-...'",
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
				message: "volume ID 'fs-abc123::invalid-id' has an invalid access point ID 'invalid-id': Expected it to be of the form 'fsap-...'",
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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
		name      string
		setup     func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error)
		expResult error
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
			name: "success",
			setup: func(t *testing.T, mockCtl *gomock.Controller) func() (kubernetes.Interface, error) {
				t.Setenv("CSI_NODE_NAME", nodeName)
				getNodeMock, mockNode := getNodeMock(mockCtl, nodeName, &corev1.Node{
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							{
								Key:    AgentNotReadyNodeTaintKey,
								Effect: "NoSchedule",
							},
						},
					},
				}, nil)
				mockNode.EXPECT().Patch(gomock.Any(), gomock.Eq(nodeName), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)

				return func() (kubernetes.Interface, error) {
					return getNodeMock, nil
				}
			},
			expResult: nil,
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
		i := 0
		tryRemoveNotReadyTaintUntilSucceed(time.Second, func() error {
			i++
			if i < 3 {
				return errors.New("test")
			}

			return nil
		})

		if i != 3 {
			t.Fatalf("unexpected result")
		}
	}
	{
		i := 0
		tryRemoveNotReadyTaintUntilSucceed(time.Second, func() error {
			i++
			return nil
		})

		if i != 1 {
			t.Fatalf("unexpected result")
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
		expectFatal                bool
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
		{
			name:                       "opt-in true with zero value should fatal",
			maxInflightMountCallsOptIn: true,
			maxInflightMountCalls:      0,
			expectFatal:                true,
		},
		{
			name:                       "opt-in true with negative value should fatal",
			maxInflightMountCallsOptIn: true,
			maxInflightMountCalls:      UnsetMaxInflightMountCounts,
			expectFatal:                true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectFatal {
				if os.Getenv("FORK") == "1" {
					// If it is in forked process, run the fatal code directly and let klog.Fatal exit
					getMaxInflightMountCalls(tc.maxInflightMountCallsOptIn, tc.maxInflightMountCalls)
					return
				}
				err := runForkFatalTest("TestGetMaxInflightMountCalls/" + tc.name)
				if err == nil {
					t.Fatal("expected process to exit with error")
				}
			} else {
				result := getMaxInflightMountCalls(tc.maxInflightMountCallsOptIn, tc.maxInflightMountCalls)
				if result != tc.expected {
					t.Errorf("Expected %d, got %d", tc.expected, result)
				}
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
		{
			name:                   "opt-in true with zero value should fatal",
			volumeAttachLimitOptIn: true,
			volumeAttachLimit:      0,
			expectFatal:            true,
		},
		{
			name:                   "opt-in true with negative value should fatal",
			volumeAttachLimitOptIn: true,
			volumeAttachLimit:      -1,
			expectFatal:            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectFatal {
				// If it is in forked process, run the fatal code directly and let klog.Fatal exit
				if os.Getenv("FORK") == "1" {
					getVolumeAttachLimit(tc.volumeAttachLimitOptIn, tc.volumeAttachLimit)
					return
				}
				err := runForkFatalTest("TestGetVolumeAttachLimit/" + tc.name)
				if err == nil {
					t.Fatal("expected process to exit with error")
				}
			} else {
				result := getVolumeAttachLimit(tc.volumeAttachLimitOptIn, tc.volumeAttachLimit)
				if result != tc.expected {
					t.Errorf("Expected %d, got %d", tc.expected, result)
				}
			}
		})
	}
}
