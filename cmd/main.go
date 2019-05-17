/*
Copyright 2018 The Kubernetes Authors.

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
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/info"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver"
	"k8s.io/klog"
)

func main() {
	var endpoint = flag.String("endpoint", "unix://tmp/csi.sock", "CSI Endpoint")
	var version = flag.Bool("version", false, "Print version information")

	klog.InitFlags(nil)
	flag.Parse()

	if *version {
		info.Print()
		return
	}

	drv := driver.NewDriver(*endpoint)
	if err := drv.Run(); err != nil {
		klog.Fatalln(err)
	}
}
