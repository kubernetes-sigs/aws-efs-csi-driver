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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/mount-utils"

	"github.com/golang/mock/gomock"
	"github.com/kubernetes-csi/csi-test/pkg/sanity"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
)

type mockWatchdog struct {
}

func (w *mockWatchdog) start() error {
	return nil
}

func (w *mockWatchdog) stop() {
}

func TestSanityEFSCSI(t *testing.T) {
	// Setup the full driver and its environment
	dir, err := ioutil.TempDir("", "sanity-efs-csi")
	if err != nil {
		t.Fatalf("error creating directory %v", err)
	}
	defer os.RemoveAll(dir)

	targetPath := filepath.Join(dir, "target")
	stagingPath := filepath.Join(dir, "staging")
	endpoint := "unix:" + filepath.Join(dir, "csi.sock")
	parameters := make(map[string]string)
	//Access Point Parameters
	parameters[FsId] = "fs-1234abcd"
	parameters[ProvisioningMode] = "efs-ap"
	parameters[DirectoryPerms] = "777"

	config := &sanity.Config{
		TargetPath:           targetPath,
		StagingPath:          stagingPath,
		Address:              endpoint,
		TestVolumeParameters: parameters,
	}

	nodeCaps := SetNodeCapOptInFeatures(true)

	mockCtrl := gomock.NewController(t)
	drv := Driver{
		endpoint:        endpoint,
		nodeID:          "sanity",
		mounter:         NewFakeMounter(),
		efsWatchdog:     &mockWatchdog{},
		cloud:           cloud.NewFakeCloudProvider(),
		nodeCaps:        nodeCaps,
		volMetricsOptIn: true,
		volStatter:      NewVolStatter(),
		gidAllocator:    NewGidAllocator(),
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recover: %v", r)
		}
	}()
	go func() {
		if err := drv.Run(); err != nil {
			panic(fmt.Sprintf("%v", err))
		}
	}()

	// Now call the test suite
	sanity.Test(t, config)

	mockCtrl.Finish()
}

func NewFakeMounter() Mounter {
	return &NodeMounter{
		Interface: &mount.FakeMounter{
			MountPoints: []mount.MountPoint{},
		},
	}
}
