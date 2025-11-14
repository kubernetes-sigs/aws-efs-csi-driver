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
	"flag"
	"fmt"
	"os"

	"k8s.io/klog/v2"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver"
)

// etcAmazonEfs is the non-negotiable directory that the mount.efs will use for config files. We will create a symlink here.
const etcAmazonEfs = "/etc/amazon/efs"

func main() {
	klog.InitFlags(nil)

	options := driver.NewOptions()
	flag.Parse()

	if err := options.Validate(); err != nil {
		klog.ErrorS(err, "Invalid options")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if *options.Version {
		info, err := driver.GetVersionJSON()
		if err != nil {
			klog.Fatalln(err)
		}
		fmt.Println(info)
		os.Exit(0)
	}

	// chose which configuration directory we will use and create a symlink to it
	err := driver.InitConfigDir(*options.EfsUtilsCfgLegacyDirPath, *options.EfsUtilsCfgDirPath, etcAmazonEfs)
	if err != nil {
		klog.Fatalln(err)
	}
	drv := driver.NewDriver(options, etcAmazonEfs)
	if err := drv.Run(); err != nil {
		klog.Fatalln(err)
	}
}
