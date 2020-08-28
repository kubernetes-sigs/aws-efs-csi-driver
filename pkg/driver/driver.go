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
	"context"
	"net"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
)

const (
	driverName = "efs.csi.aws.com"
)

type Driver struct {
	endpoint                 string
	nodeID                   string
	srv                      *grpc.Server
	mounter                  Mounter
	efsWatchdog              Watchdog
	cloud                    cloud.Cloud
	nodeCaps                 []csi.NodeServiceCapability_RPC_Type
	volMetricsOptIn          bool
	volMetricsRefreshPeriod  float64
	volMetricsFsRateLimit    int
	volStatter               VolStatter
	gidAllocator             GidAllocator
	deleteAccessPointRootDir bool
}

func NewDriver(endpoint, efsUtilsCfgPath, efsUtilsStaticFilesPath string, volMetricsOptIn bool, volMetricsRefreshPeriod float64, volMetricsFsRateLimit int, deleteAccessPointRootDir bool) *Driver {
	cloud, err := cloud.NewCloud()
	if err != nil {
		klog.Fatalln(err)
	}

	nodeCaps := SetNodeCapOptInFeatures(volMetricsOptIn)
	watchdog := newExecWatchdog(efsUtilsCfgPath, efsUtilsStaticFilesPath, "amazon-efs-mount-watchdog")
	return &Driver{
		endpoint:                 endpoint,
		nodeID:                   cloud.GetMetadata().GetInstanceID(),
		mounter:                  newNodeMounter(),
		efsWatchdog:              watchdog,
		cloud:                    cloud,
		nodeCaps:                 nodeCaps,
		volStatter:               NewVolStatter(),
		volMetricsOptIn:          volMetricsOptIn,
		volMetricsRefreshPeriod:  volMetricsRefreshPeriod,
		volMetricsFsRateLimit:    volMetricsFsRateLimit,
		gidAllocator:             NewGidAllocator(),
		deleteAccessPointRootDir: deleteAccessPointRootDir,
	}
}

func SetNodeCapOptInFeatures(volMetricsOptIn bool) []csi.NodeServiceCapability_RPC_Type {
	var nCaps = []csi.NodeServiceCapability_RPC_Type{}
	if volMetricsOptIn {
		klog.V(4).Infof("Enabling Node Service capability for Get Volume Stats")
		nCaps = append(nCaps, csi.NodeServiceCapability_RPC_GET_VOLUME_STATS)
	} else {
		klog.V(4).Infof("Node Service capability for Get Volume Stats Not enabled")
	}
	return nCaps
}

func (d *Driver) Run() error {
	scheme, addr, err := util.ParseEndpoint(d.endpoint)
	if err != nil {
		return err
	}

	listener, err := net.Listen(scheme, addr)
	if err != nil {
		return err
	}

	logErr := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			klog.Errorf("GRPC error: %v", err)
		}
		return resp, err
	}
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logErr),
	}
	d.srv = grpc.NewServer(opts...)

	csi.RegisterIdentityServer(d.srv, d)
	klog.Info("Registering Node Server")
	csi.RegisterNodeServer(d.srv, d)
	klog.Info("Registering Controller Server")
	csi.RegisterControllerServer(d.srv, d)

	klog.Info("Starting watchdog")
	if err := d.efsWatchdog.start(); err != nil {
		return err
	}

	reaper := newReaper()
	klog.Info("Staring subreaper")
	reaper.start()

	klog.Infof("Listening for connections on address: %#v", listener.Addr())
	return d.srv.Serve(listener)
}
