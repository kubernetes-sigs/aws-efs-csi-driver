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
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

func TestNodePublishVolume(t *testing.T) {

	var (
		endpoint   = "endpoint"
		nodeID     = "nodeID"
		volumeId   = "fs-volumeId"
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
		req           *csi.NodePublishVolumeRequest
		expectMakeDir bool
		mountArgs     []interface{}
		mountSuccess  bool
		expectSuccess bool
	}{
		{
			name: "success: normal",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{volumeId + ":/", targetPath, "efs", gomock.Any()},
			mountSuccess:  true,
			expectSuccess: true,
		},
		{
			name: "success: normal with read only mount",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				Readonly:         true,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{volumeId + ":/", targetPath, "efs", []string{"ro"}},
			mountSuccess:  true,
			expectSuccess: true,
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
			expectMakeDir: true,
			mountArgs:     []interface{}{volumeId + ":/", targetPath, "efs", []string{"tls"}},
			mountSuccess:  true,
			expectSuccess: true,
		},
		{
			name: "success: normal with path volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"path": "/a/b"},
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{volumeId + ":/a/b", targetPath, "efs", gomock.Any()},
			mountSuccess:  true,
			expectSuccess: true,
		},
		{
			name: "success: normal with path in volume handle",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId + ":/a/b/",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{volumeId + ":/a/b", targetPath, "efs", gomock.Any()},
			mountSuccess:  false,
			expectSuccess: false,
		},
		{
			name: "fail: missing target path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
			},
			expectMakeDir: false,
			expectSuccess: false,
		},
		{
			name: "fail: missing volume capability",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:   volumeId,
				TargetPath: targetPath,
			},
			expectMakeDir: false,
			expectSuccess: false,
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
			expectSuccess: false,
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
			expectSuccess: false,
		},
		{
			name: "fail: mounter failed to Mount",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: true,
			mountArgs:     []interface{}{volumeId + ":/", targetPath, "efs", gomock.Any()},
			mountSuccess:  false,
			expectSuccess: false,
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
			expectSuccess: false,
		},
		{
			name: "fail: relative path volume context",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         volumeId,
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
				VolumeContext:    map[string]string{"path": "a/b"},
			},
			expectMakeDir: false,
			expectSuccess: false,
		},
		{
			name: "fail: invalid filesystem ID",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:         "invalid-id",
				VolumeCapability: stdVolCap,
				TargetPath:       targetPath,
			},
			expectMakeDir: false,
			expectSuccess: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockMounter := mocks.NewMockMounter(mockCtrl)
			driver := &Driver{
				endpoint: endpoint,
				nodeID:   nodeID,
				mounter:  mockMounter,
			}

			ctx := context.Background()

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

			_, err := driver.NodePublishVolume(ctx, tc.req)
			if !tc.expectSuccess && err == nil {
				t.Fatalf("NodePublishVolume is not failed")
			}
			if tc.expectSuccess && err != nil {
				t.Fatalf("NodePublishVolume is failed: %v", err)
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
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success: normal",
			testFunc: func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtrl)
				driver := &Driver{
					endpoint: endpoint,
					nodeID:   nodeID,
					mounter:  mockMounter,
				}

				ctx := context.Background()
				req := &csi.NodeUnpublishVolumeRequest{
					VolumeId:   volumeId,
					TargetPath: targetPath,
				}

				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return("", 1, nil)
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(nil)

				_, err := driver.NodeUnpublishVolume(ctx, req)
				if err != nil {
					t.Fatalf("NodeUnpublishVolume is failed: %v", err)
				}
			},
		},
		{
			name: "success: unpublish with already unmounted target",
			testFunc: func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtrl)
				driver := &Driver{
					endpoint: endpoint,
					nodeID:   nodeID,
					mounter:  mockMounter,
				}

				ctx := context.Background()
				req := &csi.NodeUnpublishVolumeRequest{
					VolumeId:   volumeId,
					TargetPath: targetPath,
				}

				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return("", 0, nil)
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(nil)

				_, err := driver.NodeUnpublishVolume(ctx, req)
				if err != nil {
					t.Fatalf("NodeUnpublishVolume is failed: %v", err)
				}
			},
		},
		{
			name: "fail: targetPath is missing",
			testFunc: func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtrl)
				driver := &Driver{
					endpoint: endpoint,
					nodeID:   nodeID,
					mounter:  mockMounter,
				}

				ctx := context.Background()
				req := &csi.NodeUnpublishVolumeRequest{
					VolumeId: volumeId,
				}

				_, err := driver.NodeUnpublishVolume(ctx, req)
				if err == nil {
					t.Fatalf("NodeUnpublishVolume is not failed")
				}
			},
		},
		{
			name: "fail: mounter failed to umount",
			testFunc: func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtrl)
				driver := &Driver{
					endpoint: endpoint,
					nodeID:   nodeID,
					mounter:  mockMounter,
				}

				ctx := context.Background()
				req := &csi.NodeUnpublishVolumeRequest{
					VolumeId:   volumeId,
					TargetPath: targetPath,
				}

				mountErr := fmt.Errorf("Unmount failed")
				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return("", 1, nil)
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(mountErr)

				_, err := driver.NodeUnpublishVolume(ctx, req)
				if err == nil {
					t.Fatalf("NodeUnpublishVolume is not failed")
				}
			},
		},
		{
			name: "fail: mounter failed to GetDeviceName",
			testFunc: func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				mockMounter := mocks.NewMockMounter(mockCtrl)
				driver := &Driver{
					endpoint: endpoint,
					nodeID:   nodeID,
					mounter:  mockMounter,
				}

				ctx := context.Background()
				req := &csi.NodeUnpublishVolumeRequest{
					VolumeId:   volumeId,
					TargetPath: targetPath,
				}

				mounterErr := fmt.Errorf("Unmount failed")
				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return("", 1, mounterErr)
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(nil)

				_, err := driver.NodeUnpublishVolume(ctx, req)
				if err == nil {
					t.Fatalf("NodeUnpublishVolume is not failed")
				}
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}
