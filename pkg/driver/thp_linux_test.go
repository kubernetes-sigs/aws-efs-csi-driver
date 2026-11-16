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

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// thpHelperEnv makes the test binary run TestDisableTransparentHugePagesHelper as a child, so
// that PR_SET_THP_DISABLE is set in a separate process and not in the test binary itself.
const thpHelperEnv = "EFS_CSI_THP_TEST_HELPER"

// thpEnabledField returns the THP_enabled field of a /proc/<pid>/status text ("0" or "1"),
// or "" if the kernel does not report it (older than Linux 5.0).
func thpEnabledField(status string) string {
	for _, line := range strings.Split(status, "\n") {
		if v, ok := strings.CutPrefix(line, "THP_enabled:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func TestThpEnabledField(t *testing.T) {
	if got := thpEnabledField("Name:\tcat\nTHP_enabled:\t0\nThreads:\t1\n"); got != "0" {
		t.Errorf("expected 0, got %q", got)
	}
	if got := thpEnabledField("Name:\tcat\n"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// TestDisableTransparentHugePages checks that the setting applies to the process and is inherited
// by a child started with fork and execve, as mount.efs, the watchdog and efs-proxy are.
func TestDisableTransparentHugePages(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestDisableTransparentHugePagesHelper$")
	cmd.Env = append(os.Environ(), thpHelperEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper process failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "SKIP:") {
		t.Skip(strings.TrimSpace(string(out)))
	}
}

func TestDisableTransparentHugePagesHelper(t *testing.T) {
	if os.Getenv(thpHelperEnv) != "1" {
		t.Skip("only runs as a child of TestDisableTransparentHugePages")
	}
	if disabled, err := transparentHugePagesDisabled(); err != nil || disabled {
		t.Fatalf("before: expected THP not disabled, got disabled=%v err=%v", disabled, err)
	}

	if err := DisableTransparentHugePages(); err != nil {
		t.Fatalf("DisableTransparentHugePages: %v", err)
	}
	if disabled, err := transparentHugePagesDisabled(); err != nil || !disabled {
		t.Fatalf("after: expected THP disabled, got disabled=%v err=%v", disabled, err)
	}

	// A child sees the setting in its own /proc/self/status.
	out, err := exec.Command("cat", "/proc/self/status").Output()
	if err != nil {
		t.Fatalf("starting child: %v", err)
	}
	switch thpEnabledField(string(out)) {
	case "0":
	case "":
		os.Stdout.WriteString("SKIP: kernel does not report THP_enabled in /proc/<pid>/status\n")
	default:
		t.Fatalf("child: expected THP_enabled 0, got status:\n%s", out)
	}
}
