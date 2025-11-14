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
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

const (
	driverName = "efs.csi.aws.com"

	// AgentNotReadyTaintKey contains the key of taints to be removed on driver startup
	AgentNotReadyNodeTaintKey = "efs.csi.aws.com/agent-not-ready"
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
	adaptiveRetryMode        bool
	tags                     map[string]string
	lockManager              LockManagerMap
	inFlightMountTracker     *InFlightMountTracker
	volumeAttachLimit        int64
	forceUnmountAfterTimeout bool
	unmountTimeout           time.Duration
}

func NewDriver(options *Options, efsUtilsCfgPath string) *Driver {
	cloud, err := cloud.NewCloud(*options.AdaptiveRetryMode)
	if err != nil {
		klog.Fatalln(err)
	}

	nodeCaps := SetNodeCapOptInFeatures(*options.VolMetricsOptIn)
	watchdog := newExecWatchdog(efsUtilsCfgPath, *options.EfsUtilsStaticFilesPath, "amazon-efs-mount-watchdog")
	return &Driver{
		endpoint:                 *options.Endpoint,
		nodeID:                   cloud.GetMetadata().GetInstanceID(),
		mounter:                  newNodeMounter(),
		efsWatchdog:              watchdog,
		cloud:                    cloud,
		nodeCaps:                 nodeCaps,
		volStatter:               NewVolStatter(),
		volMetricsOptIn:          *options.VolMetricsOptIn,
		volMetricsRefreshPeriod:  *options.VolMetricsRefreshPeriod,
		volMetricsFsRateLimit:    *options.VolMetricsFsRateLimit,
		gidAllocator:             NewGidAllocator(),
		deleteAccessPointRootDir: *options.DeleteAccessPointRootDir,
		adaptiveRetryMode:        *options.AdaptiveRetryMode,
		tags:                     parseTagsFromStr(strings.TrimSpace(*options.Tags)),
		lockManager:              NewLockManagerMap(),
		inFlightMountTracker:     NewInFlightMountTracker(getMaxInflightMountCalls(*options.MaxInflightMountCallsOptIn, *options.MaxInflightMountCalls)),
		volumeAttachLimit:        getVolumeAttachLimit(*options.VolumeAttachLimitOptIn, *options.VolumeAttachLimit),
		forceUnmountAfterTimeout: *options.ForceUnmountAfterTimeout,
		unmountTimeout:           *options.UnmountTimeout,
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

	klog.Info("Starting efs-utils watchdog")
	if err := d.efsWatchdog.start(); err != nil {
		return err
	}

	reaper := newReaper()
	klog.Info("Starting reaper")
	reaper.start()

	// Remove taint from node to indicate driver startup success
	// This is done at the last possible moment to prevent race conditions or false positive removals
	go tryRemoveNotReadyTaintUntilSucceed(time.Second, func() error {
		return removeNotReadyTaint(cloud.DefaultKubernetesAPIClient)
	})

	klog.Infof("Listening for connections on address: %#v", listener.Addr())
	return d.srv.Serve(listener)
}

func splitToList(tagsStr string, splitter byte) []string {
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("Failed to parse input string: %v", tagsStr)
		}
	}()

	l := []string{}
	if tagsStr == "" {
		klog.Infof("Did not find any input tags.")
		return l
	}
	var tagBuilder strings.Builder
	var jumper int = 0
	for index, runeValue := range tagsStr {
		if jumper > index {
			continue
		}
		jumper++
		if byte(runeValue) == splitter {
			l = append(l, tagBuilder.String())
			tagBuilder.Reset()
			continue
		}

		// Handle escape character
		if runeValue == '\\' && tagsStr[index+1] == byte('\\') {
			tagBuilder.WriteRune('\\')
			jumper++
			continue
		}

		if runeValue == '\\' && tagsStr[index+1] == splitter {
			tagBuilder.WriteByte(splitter)
			jumper++
			continue
		}

		tagBuilder.WriteRune(runeValue)
	}
	l = append(l, tagBuilder.String())
	return l
}

func parseTagsFromStr(tagStr string) map[string]string {
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("Failed to parse input tag string: %v", tagStr)
		}
	}()

	m := map[string]string{}
	if tagStr == "" {
		klog.Infof("Did not find any input tags.")
		return m
	}
	tagsSplit := splitToList(tagStr, byte(' '))
	for _, currTag := range tagsSplit {
		var tagList = splitToList(currTag, byte(':'))
		switch len(tagList) {
		case 1:
			m[tagList[0]] = ""
		case 2:
			m[tagList[0]] = tagList[1]
		default:
			klog.Errorf("Failed to parse input tag: %v", tagList)
		}
	}
	return m
}
