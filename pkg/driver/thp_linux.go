//go:build linux

/*
Copyright 2026 The Kubernetes Authors.

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

import "golang.org/x/sys/unix"

// DisableTransparentHugePages turns transparent huge pages (THP) off for the calling process with
// prctl(PR_SET_THP_DISABLE). The setting belongs to the whole process, not to one thread, and the
// kernel copies it to every child on fork and keeps it across execve. Called before the driver
// starts any child, it therefore also applies to mount.efs, amazon-efs-mount-watchdog and every
// efs-proxy or stunnel process they start.
//
// On a node where THP is set to "always", each efs-proxy otherwise backs every touched 2 MiB
// range of its large allocator regions and thread stacks with a whole huge page.
func DisableTransparentHugePages() error {
	return unix.Prctl(unix.PR_SET_THP_DISABLE, 1, 0, 0, 0)
}

// transparentHugePagesDisabled reports whether PR_SET_THP_DISABLE is set for the calling process.
func transparentHugePagesDisabled() (bool, error) {
	v, err := unix.PrctlRetInt(unix.PR_GET_THP_DISABLE, 0, 0, 0, 0)
	return v != 0, err
}
