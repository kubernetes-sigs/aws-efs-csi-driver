package driver

import (
	"errors"
	"flag"
	"time"
)

const (
	UnsetMaxInflightMountCounts = -1
	UnsetVolumeAttachLimit      = -1
	DefaultUnmountTimeout       = 30 * time.Second
)

type Options struct {
	Endpoint                   *string
	Version                    *bool
	EfsUtilsCfgDirPath         *string
	EfsUtilsCfgLegacyDirPath   *string
	EfsUtilsStaticFilesPath    *string
	VolMetricsOptIn            *bool
	VolMetricsRefreshPeriod    *float64
	VolMetricsFsRateLimit      *int
	DeleteAccessPointRootDir   *bool
	AdaptiveRetryMode          *bool
	Tags                       *string
	MaxInflightMountCallsOptIn *bool
	MaxInflightMountCalls      *int64
	VolumeAttachLimitOptIn     *bool
	VolumeAttachLimit          *int64
	ForceUnmountAfterTimeout   *bool
	UnmountTimeout             *time.Duration
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
		AdaptiveRetryMode:          flag.Bool("adaptive-retry-mode", true, "Opt out to use standard sdk retry configuration. By default, adaptive retry mode will be used to more heavily client side rate limit EFS API requests."),
		Tags:                       flag.String("tags", "", "Space separated key:value pairs which will be added as tags for EFS resources. For example, 'environment:prod region:us-east-1'"),
		MaxInflightMountCallsOptIn: flag.Bool("max-inflight-mount-calls-opt-in", false, "Opt in to use max inflight mount calls limit."),
		MaxInflightMountCalls:      flag.Int64("max-inflight-mount-calls", UnsetMaxInflightMountCounts, "New NodePublishVolume operation will be blocked if maximum number of inflight calls is reached. If maxInflightMountCallsOptIn is true, it has to be set to a positive value."),
		VolumeAttachLimitOptIn:     flag.Bool("volume-attach-limit-opt-in", false, "Opt in to use volume attach limit."),
		VolumeAttachLimit:          flag.Int64("volume-attach-limit", UnsetVolumeAttachLimit, "Maximum number of volumes that can be attached to a node. If volumeAttachLimitOptIn is true, it has to be set to a positive value."),
		ForceUnmountAfterTimeout:   flag.Bool("force-unmount-after-timeout", false, "Enable force unmount if normal unmount times out during NodeUnpublishVolume."),
		UnmountTimeout:             flag.Duration("unmount-timeout", DefaultUnmountTimeout, "Timeout for unmounting a volume during NodePublishVolume when forceUnmountAfterTimeout is true. If the timeout is reached, the volume will be forcibly unmounted. The default value is 30 seconds."),
	}
}

func (o *Options) Validate() error {
	if *o.MaxInflightMountCallsOptIn && *o.MaxInflightMountCalls <= 0 {
		return errors.New("maxInflightMountCallsOptIn is true, but maxInflightMountCalls is not set to a positive value")
	}

	if *o.VolumeAttachLimitOptIn && *o.VolumeAttachLimit <= 0 {
		return errors.New("volumeAttachLimitOptIn is true, but volumeAttachLimit is not set to a positive value")
	}

	return nil
}
