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
	"testing"

	mount_utils "k8s.io/mount-utils"
)

// newTestNodeMounter backs a NodeMounter with a fixed mount listing, reusing the
// FakeMounterForceUnmounter adapter already defined in sanity_test.go.
func newTestNodeMounter(mps []mount_utils.MountPoint) *NodeMounter {
	return &NodeMounter{
		MounterForceUnmounter: &FakeMounterForceUnmounter{
			FakeMounter: mount_utils.NewFakeMounter(mps),
		},
	}
}

// TestGetDeviceName pins the property that makes the unmount pre-check
// hang-safe: mount status is determined from the mount listing alone, never by
// stat'ing the mountpoint. A stat against an unresponsive `hard` NFS mount
// blocks in the kernel forever, which is what pinned ~9,800 OS threads on one
// node. The subtest that pins that property is "the target itself is never
// resolved"; a target path merely absent from the test host does not pin it,
// because resolving a nonexistent path fails cheaply and harmlessly.
func TestGetDeviceName(t *testing.T) {
	const csiTarget = "/var/lib/kubelet/pods/uid-1/volumes/kubernetes.io~csi/pv-a/mount"

	t.Run("mounted target is found and refcount counts entries sharing the device", func(t *testing.T) {
		m := newTestNodeMounter([]mount_utils.MountPoint{
			{Device: "127.0.0.1:/", Path: csiTarget, Type: "nfs4"},
			{Device: "127.0.0.1:/", Path: "/var/lib/kubelet/pods/uid-2/volumes/kubernetes.io~csi/pv-a/mount", Type: "nfs4"},
			{Device: "tmpfs", Path: "/run", Type: "tmpfs"},
		})

		device, refCount, err := m.GetDeviceName(csiTarget)
		if err != nil {
			t.Fatalf("GetDeviceName returned an error: %v", err)
		}
		if device != "127.0.0.1:/" {
			t.Errorf("device = %q, expected %q", device, "127.0.0.1:/")
		}
		if refCount != 2 {
			t.Errorf("refCount = %d, expected 2", refCount)
		}
	})

	t.Run("unmounted target reports refcount zero", func(t *testing.T) {
		m := newTestNodeMounter([]mount_utils.MountPoint{
			{Device: "tmpfs", Path: "/run", Type: "tmpfs"},
			// An entry with an empty Device pins the early return: without it, an
			// unmatched target would count these and report a non-zero refCount.
			{Device: "", Path: "/sys/fs/cgroup", Type: "cgroup2"},
		})

		device, refCount, err := m.GetDeviceName(csiTarget)
		if err != nil {
			t.Fatalf("GetDeviceName returned an error: %v", err)
		}
		if device != "" || refCount != 0 {
			t.Errorf("got (%q, %d), expected (\"\", 0)", device, refCount)
		}
	})

	t.Run("non-canonical spelling of the target still matches", func(t *testing.T) {
		m := newTestNodeMounter([]mount_utils.MountPoint{
			{Device: "127.0.0.1:/", Path: csiTarget, Type: "nfs4"},
		})

		_, refCount, err := m.GetDeviceName(csiTarget + "/")
		if err != nil {
			t.Fatalf("GetDeviceName returned an error: %v", err)
		}
		if refCount != 1 {
			t.Errorf("refCount = %d, expected 1 for a trailing-slash spelling", refCount)
		}
	})

	// A kubelet directory reached through a symlink (Bottlerocket layouts) must
	// still match the resolved path in /proc/mounts. Only the parent may be
	// resolved: "mount" deliberately does not exist under the real parent, so
	// this case fails if the implementation ever resolves or stats the target.
	t.Run("symlinked parent is resolved without touching the mountpoint", func(t *testing.T) {
		realParent := t.TempDir()
		linkParent := filepath.Join(t.TempDir(), "kubelet-link")
		if err := os.Symlink(realParent, linkParent); err != nil {
			t.Fatalf("failed to create symlink: %v", err)
		}

		m := newTestNodeMounter([]mount_utils.MountPoint{
			{Device: "127.0.0.1:/", Path: filepath.Join(realParent, "mount"), Type: "nfs4"},
		})

		device, refCount, err := m.GetDeviceName(filepath.Join(linkParent, "mount"))
		if err != nil {
			t.Fatalf("GetDeviceName returned an error: %v", err)
		}
		if device != "127.0.0.1:/" || refCount != 1 {
			t.Errorf("got (%q, %d), expected (%q, 1)", device, refCount, "127.0.0.1:/")
		}
	})

	// The property that makes this hang-safe is that the target itself is never
	// resolved. Here the target IS a symlink pointing elsewhere, and both the
	// symlink and its destination appear in the listing with different devices. An
	// implementation that resolved the target would return the destination's
	// device, so this fails if the mountpoint is ever stat'ed again.
	t.Run("the target itself is never resolved", func(t *testing.T) {
		tmp := t.TempDir()
		elsewhere := filepath.Join(tmp, "elsewhere")
		if err := os.MkdirAll(elsewhere, 0o755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		target := filepath.Join(tmp, "mount")
		if err := os.Symlink(elsewhere, target); err != nil {
			t.Fatalf("failed to create symlink: %v", err)
		}

		m := newTestNodeMounter([]mount_utils.MountPoint{
			{Device: "resolved-away", Path: elsewhere, Type: "nfs4"},
			{Device: "127.0.0.1:/", Path: target, Type: "nfs4"},
		})

		device, refCount, err := m.GetDeviceName(target)
		if err != nil {
			t.Fatalf("GetDeviceName returned an error: %v", err)
		}
		if device != "127.0.0.1:/" || refCount != 1 {
			t.Errorf("got (%q, %d), expected (%q, 1): the target path was resolved", device, refCount, "127.0.0.1:/")
		}
	})

	t.Run("device shared with a bind-propagated entry is counted once per entry", func(t *testing.T) {
		m := newTestNodeMounter([]mount_utils.MountPoint{
			{Device: "127.0.0.1:/", Path: csiTarget, Type: "nfs4"},
			{Device: "127.0.0.1:/", Path: "/mnt/data/kubelet/pods/uid-1/volumes/kubernetes.io~csi/pv-a/mount", Type: "nfs4"},
		})

		_, refCount, err := m.GetDeviceName(csiTarget)
		if err != nil {
			t.Fatalf("GetDeviceName returned an error: %v", err)
		}
		if refCount != 2 {
			t.Errorf("refCount = %d, expected 2", refCount)
		}
	})
}
