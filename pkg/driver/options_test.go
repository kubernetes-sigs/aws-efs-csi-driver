package driver

import (
	"flag"
	"os"
	"testing"
	"time"
)

func TestParseFlagsWithDefaultValue(t *testing.T) {
	// Reset flag package for clean test
	flag.CommandLine = flag.NewFlagSet("test", flag.ExitOnError)

	// Test with default values
	os.Args = []string{"test"}
	opts := NewOptions()

	if opts == nil {
		t.Fatal("NewOptions() returned nil")
	}

	if *opts.Endpoint != "unix://tmp/csi.sock" {
		t.Errorf("Expected endpoint 'unix://tmp/csi.sock', got '%s'", *opts.Endpoint)
	}
	if *opts.Version != false {
		t.Errorf("Expected version false, got %v", *opts.Version)
	}
	if *opts.EfsUtilsCfgDirPath != "/var/amazon/efs" {
		t.Errorf("Expected efs-utils-config-dir-path '/var/amazon/efs', got '%s'", *opts.EfsUtilsCfgDirPath)
	}
	if *opts.EfsUtilsCfgLegacyDirPath != "/etc/amazon/efs-legacy" {
		t.Errorf("Expected efs-utils-config-legacy-dir-path '/etc/amazon/efs-legacy', got '%s'", *opts.EfsUtilsCfgLegacyDirPath)
	}
	if *opts.EfsUtilsStaticFilesPath != "/etc/amazon/efs-static-files/" {
		t.Errorf("Expected efs-utils-static-files-path '/etc/amazon/efs-static-files/', got '%s'", *opts.EfsUtilsStaticFilesPath)
	}
	if *opts.VolMetricsOptIn != false {
		t.Errorf("Expected vol-metrics-opt-in false, got %v", *opts.VolMetricsOptIn)
	}
	if *opts.VolMetricsRefreshPeriod != 240 {
		t.Errorf("Expected vol-metrics-refresh-period 240, got %f", *opts.VolMetricsRefreshPeriod)
	}
	if *opts.VolMetricsFsRateLimit != 5 {
		t.Errorf("Expected vol-metrics-fs-rate-limit 5, got %d", *opts.VolMetricsFsRateLimit)
	}
	if *opts.DeleteAccessPointRootDir != false {
		t.Errorf("Expected delete-access-point-root-dir false, got %v", *opts.DeleteAccessPointRootDir)
	}
	if *opts.AdaptiveRetryMode != true {
		t.Errorf("Expected adaptive-retry-mode true, got %v", *opts.AdaptiveRetryMode)
	}
	if *opts.Tags != "" {
		t.Errorf("Expected tags empty string, got '%s'", *opts.Tags)
	}
	if *opts.MaxInflightMountCallsOptIn != false {
		t.Errorf("Expected max-inflight-mount-calls-opt-in false, got %v", *opts.MaxInflightMountCallsOptIn)
	}
	if *opts.MaxInflightMountCalls != UnsetMaxInflightMountCounts {
		t.Errorf("Expected max-inflight-mount-calls %d, got %d", UnsetMaxInflightMountCounts, *opts.MaxInflightMountCalls)
	}
	if *opts.VolumeAttachLimitOptIn != false {
		t.Errorf("Expected volume-attach-limit-opt-in false, got %v", *opts.VolumeAttachLimitOptIn)
	}
	if *opts.VolumeAttachLimit != UnsetVolumeAttachLimit {
		t.Errorf("Expected volume-attach-limit %d, got %d", UnsetVolumeAttachLimit, *opts.VolumeAttachLimit)
	}

	if *opts.ForceUnmountAfterTimeout != false {
		t.Errorf("Expected force-unmount-after-timeout false, got %v", *opts.ForceUnmountAfterTimeout)
	}

	if *opts.UnmountTimeout != DefaultUnmountTimeout {
		t.Errorf("Expected unmount-timeout %d, got %d", DefaultUnmountTimeout, *opts.UnmountTimeout)
	}
}

func TestParseFlagsWithCustomValues(t *testing.T) {
	// Reset flag package for clean test
	flag.CommandLine = flag.NewFlagSet("test", flag.ExitOnError)

	// Test with custom values
	os.Args = []string{
		"test",
		"--endpoint=tcp://localhost:9000",
		"--version=true",
		"--efs-utils-config-dir-path=/custom/efs",
		"--efs-utils-config-legacy-dir-path=/custom/efs-legacy",
		"--efs-utils-static-files-path=/custom/efs-static/",
		"--vol-metrics-opt-in=true",
		"--vol-metrics-refresh-period=120",
		"--vol-metrics-fs-rate-limit=10",
		"--delete-access-point-root-dir=true",
		"--adaptive-retry-mode=false",
		"--tags=env:test region:us-west-2",
		"--max-inflight-mount-calls-opt-in=true",
		"--max-inflight-mount-calls=20",
		"--volume-attach-limit-opt-in=true",
		"--volume-attach-limit=15",
		"--force-unmount-after-timeout=true",
		"--unmount-timeout=60s",
	}

	opts := NewOptions()
	flag.Parse()

	if *opts.Endpoint != "tcp://localhost:9000" {
		t.Errorf("Expected endpoint 'tcp://localhost:9000', got '%s'", *opts.Endpoint)
	}
	if *opts.Version != true {
		t.Errorf("Expected version true, got %v", *opts.Version)
	}
	if *opts.EfsUtilsCfgDirPath != "/custom/efs" {
		t.Errorf("Expected efs-utils-config-dir-path '/custom/efs', got '%s'", *opts.EfsUtilsCfgDirPath)
	}
	if *opts.EfsUtilsCfgLegacyDirPath != "/custom/efs-legacy" {
		t.Errorf("Expected efs-utils-config-legacy-dir-path '/custom/efs-legacy', got '%s'", *opts.EfsUtilsCfgLegacyDirPath)
	}
	if *opts.EfsUtilsStaticFilesPath != "/custom/efs-static/" {
		t.Errorf("Expected efs-utils-static-files-path '/custom/efs-static/', got '%s'", *opts.EfsUtilsStaticFilesPath)
	}
	if *opts.VolMetricsOptIn != true {
		t.Errorf("Expected vol-metrics-opt-in true, got %v", *opts.VolMetricsOptIn)
	}
	if *opts.VolMetricsRefreshPeriod != 120 {
		t.Errorf("Expected vol-metrics-refresh-period 120, got %f", *opts.VolMetricsRefreshPeriod)
	}
	if *opts.VolMetricsFsRateLimit != 10 {
		t.Errorf("Expected vol-metrics-fs-rate-limit 10, got %d", *opts.VolMetricsFsRateLimit)
	}
	if *opts.DeleteAccessPointRootDir != true {
		t.Errorf("Expected delete-access-point-root-dir true, got %v", *opts.DeleteAccessPointRootDir)
	}
	if *opts.AdaptiveRetryMode != false {
		t.Errorf("Expected adaptive-retry-mode false, got %v", *opts.AdaptiveRetryMode)
	}
	if *opts.Tags != "env:test region:us-west-2" {
		t.Errorf("Expected tags 'env:test region:us-west-2', got '%s'", *opts.Tags)
	}
	if *opts.MaxInflightMountCallsOptIn != true {
		t.Errorf("Expected max-inflight-mount-calls-opt-in true, got %v", *opts.MaxInflightMountCallsOptIn)
	}
	if *opts.MaxInflightMountCalls != 20 {
		t.Errorf("Expected max-inflight-mount-calls 20, got %d", *opts.MaxInflightMountCalls)
	}
	if *opts.VolumeAttachLimitOptIn != true {
		t.Errorf("Expected volume-attach-limit-opt-in true, got %v", *opts.VolumeAttachLimitOptIn)
	}
	if *opts.VolumeAttachLimit != 15 {
		t.Errorf("Expected volume-attach-limit 15, got %d", *opts.VolumeAttachLimit)
	}

	if *opts.ForceUnmountAfterTimeout != true {
		t.Errorf("Expected force-unmount-after-timeout true, got %v", *opts.ForceUnmountAfterTimeout)
	}

	if *opts.UnmountTimeout != 60*time.Second {
		t.Errorf("Expected unmount-timeout 60 seconds, got %v", *opts.UnmountTimeout)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		opts        *Options
		expectError bool
	}{
		{
			name: "valid default options",
			opts: &Options{
				MaxInflightMountCallsOptIn: boolPtr(false),
				MaxInflightMountCalls:      int64Ptr(UnsetMaxInflightMountCounts),
				VolumeAttachLimitOptIn:     boolPtr(false),
				VolumeAttachLimit:          int64Ptr(UnsetVolumeAttachLimit),
			},
			expectError: false,
		},
		{
			name: "invalid max inflight mount calls while opt in",
			opts: &Options{
				MaxInflightMountCallsOptIn: boolPtr(true),
				MaxInflightMountCalls:      int64Ptr(0),
				VolumeAttachLimitOptIn:     boolPtr(false),
				VolumeAttachLimit:          int64Ptr(UnsetVolumeAttachLimit),
			},
			expectError: true,
		},
		{
			name: "invalid negative volume attach limit while opt in",
			opts: &Options{
				MaxInflightMountCallsOptIn: boolPtr(false),
				MaxInflightMountCalls:      int64Ptr(UnsetMaxInflightMountCounts),
				VolumeAttachLimitOptIn:     boolPtr(true),
				VolumeAttachLimit:          int64Ptr(-1),
			},
			expectError: true,
		},
		{
			name: "valid positive values while opt in",
			opts: &Options{
				MaxInflightMountCallsOptIn: boolPtr(true),
				MaxInflightMountCalls:      int64Ptr(10),
				VolumeAttachLimitOptIn:     boolPtr(true),
				VolumeAttachLimit:          int64Ptr(5),
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if (err != nil) != tc.expectError {
				t.Errorf("Validate() error = %v, expectError %v", err, tc.expectError)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}
