/*
Copyright 2018 The Kubernetes Authors.

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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

func TestNodePublishVolume(t *testing.T) {

	var (
		endpoint   = "endpoint"
		nodeID     = "nodeID"
		volumeId   = "volumeId"
		targetPath = "/target/path"
		stdVolCap  = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			},
		}
	)

	testCases := []struct {
		name          string
		readOnly      bool
		volCap        *csi.VolumeCapability
		targetPath    string
		expectMakeDir bool
		makeDirErr    error
		expectMount   bool
		mountErr      error
		expectFail    bool
	}{
		{
			name:          "success: normal",
			readOnly:      false,
			volCap:        stdVolCap,
			targetPath:    targetPath,
			expectMakeDir: true,
			makeDirErr:    nil,
			expectMount:   true,
			mountErr:      nil,
			expectFail:    false,
		},
		{
			name:          "success: normal with read only mount",
			readOnly:      true,
			volCap:        stdVolCap,
			targetPath:    targetPath,
			expectMakeDir: true,
			makeDirErr:    nil,
			expectMount:   true,
			mountErr:      nil,
			expectFail:    false,
		},
		{
			name:     "success: normal with tls mount options",
			readOnly: false,
			volCap: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						MountFlags: []string{"tls"},
					},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
			},
			targetPath:    targetPath,
			expectMakeDir: true,
			makeDirErr:    nil,
			expectMount:   true,
			mountErr:      nil,
			expectFail:    false,
		},
		{
			name:          "fail: missing target path",
			readOnly:      false,
			volCap:        stdVolCap,
			targetPath:    "",
			expectMakeDir: false,
			makeDirErr:    nil,
			expectMount:   false,
			mountErr:      nil,
			expectFail:    true,
		},
		{
			name:          "fail: missing volume capability",
			readOnly:      false,
			volCap:        nil,
			targetPath:    targetPath,
			expectMakeDir: false,
			makeDirErr:    nil,
			expectMount:   false,
			mountErr:      nil,
			expectFail:    true,
		},
		{
			name:     "fail: unsupported volume capability",
			readOnly: false,
			volCap: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
				},
			},
			targetPath:    targetPath,
			expectMakeDir: false,
			makeDirErr:    nil,
			expectMount:   false,
			mountErr:      nil,
			expectFail:    true,
		},
		{
			name:          "fail: mounter failed to MakeDir",
			readOnly:      false,
			volCap:        stdVolCap,
			targetPath:    targetPath,
			expectMakeDir: true,
			makeDirErr:    fmt.Errorf("failed to MakeDir"),
			expectMount:   false,
			mountErr:      nil,
			expectFail:    true,
		},
		{
			name:          "fail: mounter failed to Mount",
			readOnly:      false,
			volCap:        stdVolCap,
			targetPath:    targetPath,
			expectMakeDir: true,
			makeDirErr:    nil,
			expectMount:   true,
			mountErr:      fmt.Errorf("failed to Mount"),
			expectFail:    true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockMounter := mocks.NewMockInterface(mockCtrl)
			driver := &Driver{
				endpoint: endpoint,
				nodeID:   nodeID,
				mounter:  mockMounter,
			}
			source := volumeId + ":/"

			ctx := context.Background()
			req := &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: tc.volCap,
				TargetPath:       tc.targetPath,
				Readonly:         tc.readOnly,
			}

			if tc.expectMakeDir {
				mockMounter.EXPECT().MakeDir(gomock.Eq(tc.targetPath)).Return(tc.makeDirErr)
			}

			mountFlags := []string{}
			if tc.readOnly {
				mountFlags = append(mountFlags, "ro")
			}
			if tc.volCap != nil {
				mountFlags = append(mountFlags, tc.volCap.AccessType.(*csi.VolumeCapability_Mount).Mount.MountFlags...)
			}
			if tc.expectMount {
				mockMounter.EXPECT().Mount(gomock.Eq(source), gomock.Eq(tc.targetPath), gomock.Eq("efs"), gomock.Eq(mountFlags)).Return(tc.mountErr)
			}

			_, err := driver.NodePublishVolume(ctx, req)
			if err != nil && !tc.expectFail {
				t.Fatalf("NodePublishVolume is failed: %v", err)
			} else if err == nil && tc.expectFail {
				t.Fatalf("NodePublishVolume is not failed: %v", err)
			}

			mockCtrl.Finish()
		})
	}
}

func TestNodeUnpublishVolume(t *testing.T) {

	var (
		endpoint   = "endpoint"
		nodeID     = "nodeID"
		volumeId   = "volumeId"
		targetPath = "/target/path"
	)

	testCases := []struct {
		name          string
		targetPath    string
		expectUnmount bool
		unmountErr    error
		expectFail    bool
	}{
		{
			name:          "success: normal",
			targetPath:    targetPath,
			expectUnmount: true,
			unmountErr:    nil,
			expectFail:    false,
		},
		{
			name:          "fail: targetPath is missing",
			targetPath:    "",
			expectUnmount: false,
			unmountErr:    nil,
			expectFail:    true,
		},
		{
			name:          "fail: mounter failed to umount",
			targetPath:    targetPath,
			expectUnmount: true,
			unmountErr:    fmt.Errorf("Unmount failed"),
			expectFail:    true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockMounter := mocks.NewMockInterface(mockCtrl)
			driver := &Driver{
				endpoint: endpoint,
				nodeID:   nodeID,
				mounter:  mockMounter,
			}

			ctx := context.Background()
			req := &csi.NodeUnpublishVolumeRequest{
				VolumeId:   volumeId,
				TargetPath: tc.targetPath,
			}

			if tc.expectUnmount {
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(tc.unmountErr)
			}

			_, err := driver.NodeUnpublishVolume(ctx, req)
			if err != nil && !tc.expectFail {
				t.Fatalf("NodeUnpublishVolume is failed: %v", err)
			} else if err == nil && tc.expectFail {
				t.Fatalf("NodeUnpublishVolume is not failed: %v", err)
			}
		})
	}
}
