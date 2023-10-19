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

package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"k8s.io/component-base/featuregate"
	logsapi "k8s.io/component-base/logs/api/v1"
	json "k8s.io/component-base/logs/json"
	"k8s.io/klog/v2"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver"
)

// etcAmazonEfs is the non-negotiable directory that the mount.efs will use for config files. We will create a symlink here.
const etcAmazonEfs = "/etc/amazon/efs"

var featureGate = featuregate.NewFeatureGate()

func main() {
	var (
		fs                       = flag.NewFlagSet("aws-efs-csi-driver", flag.ExitOnError)
		endpoint                 = fs.String("endpoint", "unix://tmp/csi.sock", "CSI Endpoint")
		version                  = fs.Bool("version", false, "Print the version and exit")
		efsUtilsCfgDirPath       = fs.String("efs-utils-config-dir-path", "/var/amazon/efs", "The preferred path for the efs-utils config directory. efs-utils-config-legacy-dir-path will be used if it is not empty, otherwise efs-utils-config-dir-path will be used.")
		efsUtilsCfgLegacyDirPath = fs.String("efs-utils-config-legacy-dir-path", "/etc/amazon/efs-legacy", "The path to the legacy efs-utils config directory mounted from the host path /etc/amazon/efs")
		efsUtilsStaticFilesPath  = fs.String("efs-utils-static-files-path", "/etc/amazon/efs-static-files/", "The path to efs-utils static files directory")
		volMetricsOptIn          = fs.Bool("vol-metrics-opt-in", false, "Opt in to emit volume metrics")
		volMetricsRefreshPeriod  = fs.Float64("vol-metrics-refresh-period", 240, "Refresh period for volume metrics in minutes")
		volMetricsFsRateLimit    = fs.Int("vol-metrics-fs-rate-limit", 5, "Volume metrics routines rate limiter per file system")
		deleteAccessPointRootDir = fs.Bool("delete-access-point-root-dir", false, "Opt in to delete access point root directory by DeleteVolume. By default, DeleteVolume will delete the access point behind Persistent Volume and deleting access point will not delete the access point root directory or its contents.")
		tags                     = fs.String("tags", "", "Space separated key:value pairs which will be added as tags for EFS resources. For example, 'environment:prod region:us-east-1'")
	)
	if err := logsapi.RegisterLogFormat(logsapi.JSONLogFormat, json.Factory{}, logsapi.LoggingBetaOptions); err != nil {
		klog.ErrorS(err, "failed to register JSON log format")
	}

	c := logsapi.NewLoggingConfiguration()

	err := logsapi.AddFeatureGates(featureGate)
	if err != nil {
		klog.ErrorS(err, "failed to add feature gates")
	}

	logsapi.AddFlags(c, fs)
	fs.Parse(os.Args[1:])

	err = logsapi.ValidateAndApply(c, featureGate)
	if err != nil {
		klog.ErrorS(err, "failed to validate and apply logging configuration")
	}

	if *version {
		info, err := driver.GetVersionJSON()
		if err != nil {
			klog.Fatalln(err)
		}
		fmt.Println(info)
		os.Exit(0)
	}

	// chose which configuration directory we will use and create a symlink to it
	err = driver.InitConfigDir(*efsUtilsCfgLegacyDirPath, *efsUtilsCfgDirPath, etcAmazonEfs)
	if err != nil {
		klog.Fatalln(err)
	}
	drv := driver.NewDriver(*endpoint, etcAmazonEfs, *efsUtilsStaticFilesPath, *tags, *volMetricsOptIn, *volMetricsRefreshPeriod, *volMetricsFsRateLimit, *deleteAccessPointRootDir)
	if err := drv.Run(); err != nil {
		klog.Fatalln(err)
	}
}
