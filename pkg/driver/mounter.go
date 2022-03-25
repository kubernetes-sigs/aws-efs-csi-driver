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
	"os"

	"k8s.io/mount-utils"
)

// Mounter is an interface for mount operations
type Mounter interface {
	mount.Interface
	IsCorruptedMnt(err error) bool
	MakeDir(pathname string) error
	PathExists(path string) (bool, error)
	GetDeviceName(mountPath string) (string, int, error)
}

type NodeMounter struct {
	mount.Interface
}

func newNodeMounter() Mounter {
	return &NodeMounter{
		Interface: mount.New(""),
	}
}

func (m *NodeMounter) MakeDir(pathname string) error {
	err := os.MkdirAll(pathname, os.FileMode(0755))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func (m *NodeMounter) GetDeviceName(mountPath string) (string, int, error) {
	return mount.GetDeviceNameFromMount(m, mountPath)
}

// IsCorruptedMnt return true if err is about corrupted mount point
func (m NodeMounter) IsCorruptedMnt(err error) bool {
	return mount.IsCorruptedMnt(err)
}

// This function is mirrored in ./sanity_test.go to make sure sanity test covered this block of code
// Please mirror the change to func MakeFile in ./sanity_test.go
func (m *NodeMounter) PathExists(path string) (bool, error) {
	return mount.PathExists(path)
}
