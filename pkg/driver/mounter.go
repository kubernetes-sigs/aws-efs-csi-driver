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
	"path/filepath"

	mount_utils "k8s.io/mount-utils"
)

// Mounter is an interface for mount operations
type Mounter interface {
	mount_utils.MounterForceUnmounter
	MakeDir(pathname string) error
	Stat(pathname string) (os.FileInfo, error)
	GetDeviceName(mountPath string) (string, int, error)
	IsLikelyNotMountPoint(target string) (bool, error)
}

type NodeMounter struct {
	mount_utils.MounterForceUnmounter
}

func newNodeMounter() Mounter {
	return &NodeMounter{
		MounterForceUnmounter: mount_utils.New("").(mount_utils.MounterForceUnmounter),
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

func (m *NodeMounter) Stat(pathname string) (os.FileInfo, error) {
	return os.Stat(pathname)
}

// GetDeviceName returns the device mounted at mountPath and the number of
// /proc/mounts entries referring to it.
//
// It avoids mount_utils.GetDeviceNameFromMount, which resolves mountPath with
// EvalSymlinks and so stats the mountpoint. That blocks uninterruptibly on an
// unresponsive hard NFS mount. Symlinks are resolved on the parent only, and an
// unmatched target yields refCount 0, which the caller reads as not mounted.
func (m *NodeMounter) GetDeviceName(mountPath string) (string, int, error) {
	mps, err := m.List()
	if err != nil {
		return "", 0, err
	}

	// Spellings that may appear in /proc/mounts. mountPath itself is never resolved.
	cleanPath := filepath.Clean(mountPath)
	candidates := []string{cleanPath}
	if resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(cleanPath)); err == nil {
		if resolved := filepath.Join(resolvedParent, filepath.Base(cleanPath)); resolved != cleanPath {
			candidates = append(candidates, resolved)
		}
	}

	device := ""
	for i := range mps {
		for _, candidate := range candidates {
			if mps[i].Path == candidate {
				device = mps[i].Device
				break
			}
		}
		if device != "" {
			break
		}
	}
	if device == "" {
		// Not mounted. Return 0 rather than counting entries with an empty Device.
		return "", 0, nil
	}

	refCount := 0
	for i := range mps {
		if mps[i].Device == device {
			refCount++
		}
	}
	return device, refCount, nil
}

func (m *NodeMounter) IsLikelyNotMountPoint(target string) (bool, error) {
	notMnt, err := m.MounterForceUnmounter.IsLikelyNotMountPoint(target)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return notMnt, nil
}
