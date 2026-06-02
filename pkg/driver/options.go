package driver

import (
	"errors"
	"flag"
	"fmt"
	"time"
)

const (
	UnsetMaxInflightMountCounts = -1
	UnsetVolumeAttachLimit      = -1
	DefaultUnmountTimeout       = 30 * time.Second
)

type Options struct {
	Endpoint                        *string
	Version                         *bool
	EfsUtilsCfgDirPath              *string
	EfsUtilsCfgLegacyDirPath        *string
	EfsUtilsStaticFilesPath         *string
	VolMetricsOptIn                 *bool
	VolMetricsRefreshPeriod         *float64
	VolMetricsFsRateLimit           *int
	DeleteAccessPointRootDir        *bool
	AdaptiveRetryMode               *bool
	Tags                            *string
	MaxInflightMountCallsOptIn      *bool
	MaxInflightMountCalls           *int64
	VolumeAttachLimitOptIn          *bool
	VolumeAttachLimit               *int64
	ForceUnmountAfterTimeout        *bool
	UnmountTimeout                  *time.Duration
	DebugLogs                       *bool
	EfsCloudWatchLogEnabled         *bool
	S3FilesCloudWatchLogEnabled     *bool
	S3FilesCloudWatchMetricsEnabled *bool
	EfsUtilsConfOverrides           *string
	S3FilesUtilsConfOverrides       *string

	efsUtilsConfOverridesParsed     []ConfOverride
	s3filesUtilsConfOverridesParsed []ConfOverride
}

func NewOptions() *Options {
	return &Options{
		Endpoint:                 flag.String("endpoint", "unix://tmp/csi.sock", "CSI Endpoint"),
		Version:                  flag.Bool("version", false, "Print the version and exit"),
		EfsUtilsCfgDirPath:       flag.String("efs-utils-config-dir-path", "/var/amazon/efs", "The preferred path for the efs-utils config directory. efs-utils-config-legacy-dir-path will be used if it is not empty, otherwise efs-utils-config-dir-path will be used."),
		EfsUtilsCfgLegacyDirPath: flag.String("efs-utils-config-legacy-dir-path", "/etc/amazon/efs-legacy", "The path to the legacy efs-utils config directory mounted from the host path /etc/amazon/efs"),
		EfsUtilsStaticFilesPath:  flag.String("efs-utils-static-files-path", "/etc/amazon/efs-static-files/", "The path to efs-utils static files directory"),
		VolMetricsOptIn:          flag.Bool("vol-metrics-opt-in", false, "Opt in to emit volume metrics"),
		VolMetricsRefreshPeriod:  flag.Float64("vol-metrics-refresh-period", 240, "Refresh period for volume metrics in minutes"),
		VolMetricsFsRateLimit:    flag.Int("vol-metrics-fs-rate-limit", 5, "Volume metrics routines rate limiter per file system"),
		DeleteAccessPointRootDir: flag.Bool("delete-access-point-root-dir", false,
			"Opt in to delete access point root directory by DeleteVolume. By default, DeleteVolume will delete the access point behind Persistent Volume and deleting access point will not delete the access point root directory or its contents."),
		AdaptiveRetryMode:               flag.Bool("adaptive-retry-mode", true, "Opt out to use standard sdk retry configuration. By default, adaptive retry mode will be used to more heavily client side rate limit EFS API requests."),
		Tags:                            flag.String("tags", "", "Space separated key:value pairs which will be added as tags for EFS resources. For example, 'environment:prod region:us-east-1'"),
		MaxInflightMountCallsOptIn:      flag.Bool("max-inflight-mount-calls-opt-in", false, "Opt in to use max inflight mount calls limit."),
		MaxInflightMountCalls:           flag.Int64("max-inflight-mount-calls", UnsetMaxInflightMountCounts, "New NodePublishVolume operation will be blocked if maximum number of inflight calls is reached. If maxInflightMountCallsOptIn is true, it has to be set to a positive value."),
		VolumeAttachLimitOptIn:          flag.Bool("volume-attach-limit-opt-in", false, "Opt in to use volume attach limit."),
		VolumeAttachLimit:               flag.Int64("volume-attach-limit", UnsetVolumeAttachLimit, "Maximum number of volumes that can be attached to a node. If volumeAttachLimitOptIn is true, it has to be set to a positive value."),
		ForceUnmountAfterTimeout:        flag.Bool("force-unmount-after-timeout", false, "Enable force unmount if normal unmount times out during NodeUnpublishVolume."),
		UnmountTimeout:                  flag.Duration("unmount-timeout", DefaultUnmountTimeout, "Timeout for unmounting a volume during NodeUnpublishVolume when forceUnmountAfterTimeout is true. If the timeout is reached, the volume will be forcibly unmounted. The default value is 30 seconds."),
		DebugLogs:                       flag.Bool("debug-logs", false, "Set klog verbosity to level 5 and enable debug logging in efs-utils."),
		EfsCloudWatchLogEnabled:         flag.Bool("efs-cloudwatch-log-enabled", false, "Enable CloudWatch logging for EFS in efs-utils.conf."),
		S3FilesCloudWatchLogEnabled:     flag.Bool("s3files-cloudwatch-log-enabled", true, "Enable CloudWatch logging for S3Files in s3files-utils.conf."),
		S3FilesCloudWatchMetricsEnabled: flag.Bool("s3files-cloudwatch-metrics-enabled", true, "Enable CloudWatch metrics emission for S3 files in s3files-utils.conf."),
		EfsUtilsConfOverrides:           flag.String("efs-utils-conf-overrides", "", "Comma-separated section:key=value overrides applied to efs-utils.conf. These take precedence over other flags that control the same config (e.g., efs-cloudwatch-log-enabled)."),
		S3FilesUtilsConfOverrides:       flag.String("s3files-utils-conf-overrides", "", "Comma-separated section:key=value overrides applied to s3files-utils.conf. These take precedence over other flags that control the same config (e.g., s3files-cloudwatch-log-enabled, s3files-cloudwatch-metrics-enabled)."),
	}
}

func (o *Options) Validate() error {
	if *o.MaxInflightMountCallsOptIn && *o.MaxInflightMountCalls <= 0 {
		return errors.New("maxInflightMountCallsOptIn is true, but maxInflightMountCalls is not set to a positive value")
	}

	if *o.VolumeAttachLimitOptIn && *o.VolumeAttachLimit <= 0 {
		return errors.New("volumeAttachLimitOptIn is true, but volumeAttachLimit is not set to a positive value")
	}

	parsed, err := parseConfOverrides(*o.EfsUtilsConfOverrides)
	if err != nil {
		return fmt.Errorf("invalid efs-utils-conf-overrides: %w", err)
	}
	o.efsUtilsConfOverridesParsed = parsed

	parsed, err = parseConfOverrides(*o.S3FilesUtilsConfOverrides)
	if err != nil {
		return fmt.Errorf("invalid s3files-utils-conf-overrides: %w", err)
	}
	o.s3filesUtilsConfOverridesParsed = parsed

	return nil
}
