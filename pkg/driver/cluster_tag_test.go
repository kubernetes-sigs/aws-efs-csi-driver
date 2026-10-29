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
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
)

const (
	testClusterTagKey = "example.com/cluster"
	testClusterName   = "my-cluster"
)

func testClusterTag(isolation ClusterTagIsolation) clusterTagConfig {
	return clusterTagConfig{clusterName: testClusterName, tagKey: testClusterTagKey, isolation: isolation}
}

func parseTestOptions(t *testing.T, args ...string) (*Options, error) {
	t.Helper()
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	os.Args = append([]string{"test"}, args...)
	opts := NewOptions()
	if err := flag.CommandLine.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	return opts, opts.Validate()
}

func TestClusterTagOptionsDefaults(t *testing.T) {
	opts, err := parseTestOptions(t)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if *opts.ClusterName != "" || *opts.ClusterTagKey != "" || *opts.ExtraTags != "" {
		t.Fatalf("expected empty cluster name, cluster tag key and extra tags by default")
	}
	if *opts.ClusterTagIsolation != "disable" {
		t.Fatalf("expected --cluster-tag-isolation=disable by default, got %q", *opts.ClusterTagIsolation)
	}
	if opts.clusterTag.enabled() {
		t.Fatalf("cluster tag must be disabled by default")
	}
	if len(opts.tagsParsed) != 0 {
		t.Fatalf("expected no tags by default, got %v", opts.tagsParsed)
	}
}

func TestClusterTagOptionsValidate(t *testing.T) {
	testCases := []struct {
		name    string
		args    []string
		wantErr string
		want    clusterTagConfig
		tags    map[string]string
	}{
		{
			name: "cluster name and key in disable state",
			args: []string{"--cluster-name=my-cluster", "--cluster-tag-key=example.com/cluster"},
			want: testClusterTag(ClusterTagIsolationDisable),
			tags: map[string]string{},
		},
		{
			name: "backfill",
			args: []string{"--cluster-name=my-cluster", "--cluster-tag-key=example.com/cluster", "--cluster-tag-isolation=backfill"},
			want: testClusterTag(ClusterTagIsolationBackfill),
			tags: map[string]string{},
		},
		{
			name: "enforce",
			args: []string{"--cluster-name=my-cluster", "--cluster-tag-key=example.com/cluster", "--cluster-tag-isolation=enforce"},
			want: testClusterTag(ClusterTagIsolationEnforce),
			tags: map[string]string{},
		},
		{
			name: "cluster name only",
			args: []string{"--cluster-name=my-cluster"},
			want: clusterTagConfig{clusterName: testClusterName, isolation: ClusterTagIsolationDisable},
			tags: map[string]string{},
		},
		{
			name:    "unknown isolation state",
			args:    []string{"--cluster-tag-isolation=strict"},
			wantErr: "invalid --cluster-tag-isolation",
		},
		{
			name: "cluster tag key only",
			args: []string{"--cluster-tag-key=example.com/cluster"},
			want: clusterTagConfig{tagKey: "example.com/cluster", isolation: ClusterTagIsolationDisable},
			tags: map[string]string{},
		},
		{
			name:    "backfill without cluster name",
			args:    []string{"--cluster-tag-key=example.com/cluster", "--cluster-tag-isolation=backfill"},
			wantErr: "--cluster-tag-isolation=backfill requires both --cluster-name and --cluster-tag-key",
		},
		{
			name:    "whitespace-only cluster name",
			args:    []string{"--cluster-name=  ", "--cluster-tag-key=example.com/cluster", "--cluster-tag-isolation=enforce"},
			wantErr: "--cluster-tag-isolation=enforce requires both --cluster-name and --cluster-tag-key",
		},
		{
			name:    "enforce without cluster tag key",
			args:    []string{"--cluster-name=my-cluster", "--cluster-tag-isolation=enforce"},
			wantErr: "--cluster-tag-isolation=enforce requires both --cluster-name and --cluster-tag-key",
		},
		{
			name:    "backfill without anything",
			args:    []string{"--cluster-tag-isolation=backfill"},
			wantErr: "--cluster-tag-isolation=backfill requires both --cluster-name and --cluster-tag-key",
		},
		{
			name:    "reserved aws prefix",
			args:    []string{"--cluster-name=my-cluster", "--cluster-tag-key=aws:cluster"},
			wantErr: "reserved",
		},
		{
			name:    "cluster tag key equal to the default tag key",
			args:    []string{"--cluster-name=my-cluster", "--cluster-tag-key=efs.csi.aws.com/cluster"},
			wantErr: "must not be efs.csi.aws.com/cluster",
		},
		{
			name: "extra tags",
			args: []string{"--extra-tags=team=storage,env=prod"},
			tags: map[string]string{"team": "storage", "env": "prod"},
			want: clusterTagConfig{isolation: ClusterTagIsolationDisable},
		},
		{
			name: "deprecated --tags still takes effect",
			args: []string{"--tags=team:storage env:prod"},
			tags: map[string]string{"team": "storage", "env": "prod"},
			want: clusterTagConfig{isolation: ClusterTagIsolationDisable},
		},
		{
			name: "--tags and --extra-tags are merged",
			args: []string{"--tags=team:storage", "--extra-tags=env=prod,team=storage"},
			tags: map[string]string{"team": "storage", "env": "prod"},
			want: clusterTagConfig{isolation: ClusterTagIsolationDisable},
		},
		{
			name:    "--tags and --extra-tags conflict",
			args:    []string{"--tags=team:storage", "--extra-tags=team=web"},
			wantErr: `tag key "team" has different values in --tags ("storage") and --extra-tags ("web")`,
		},
		{
			name:    "--extra-tags sets the cluster tag key",
			args:    []string{"--cluster-name=my-cluster", "--cluster-tag-key=example.com/cluster", "--extra-tags=example.com/cluster=other"},
			wantErr: "must not set the cluster tag key",
		},
		{
			name:    "--tags sets the cluster tag key",
			args:    []string{"--cluster-name=my-cluster", "--cluster-tag-key=example.com/cluster", "--tags=example.com/cluster:other"},
			wantErr: "must not set the cluster tag key",
		},
		{
			name:    "--extra-tags sets the default tag key",
			args:    []string{"--extra-tags=efs.csi.aws.com/cluster=false"},
			wantErr: "--extra-tags must not set efs.csi.aws.com/cluster",
		},
		{
			name:    "--extra-tags malformed",
			args:    []string{"--extra-tags=team"},
			wantErr: "expected key=value",
		},
		{
			name:    "--extra-tags duplicate key",
			args:    []string{"--extra-tags=team=a,team=b"},
			wantErr: "set more than once",
		},
		{
			name:    "--extra-tags reserved prefix",
			args:    []string{"--extra-tags=aws:team=a"},
			wantErr: "reserved",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseTestOptions(t, tc.args...)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate failed: %v", err)
			}
			if opts.clusterTag != tc.want {
				t.Fatalf("cluster tag config: expected %+v, got %+v", tc.want, opts.clusterTag)
			}
			if !reflect.DeepEqual(opts.tagsParsed, tc.tags) {
				t.Fatalf("tags: expected %v, got %v", tc.tags, opts.tagsParsed)
			}
		})
	}
}

func TestBuildAccessPointTagsWithClusterTag(t *testing.T) {
	testCases := []struct {
		name       string
		tags       map[string]string
		clusterTag clusterTagConfig
		want       map[string]string
	}{
		{
			name: "nothing configured",
			want: map[string]string{DefaultTagKey: DefaultTagValue},
		},
		{
			name:       "cluster name without key adds nothing",
			clusterTag: clusterTagConfig{clusterName: testClusterName, isolation: ClusterTagIsolationDisable},
			want:       map[string]string{DefaultTagKey: DefaultTagValue},
		},
		{
			name:       "disable state still tags new access points",
			clusterTag: testClusterTag(ClusterTagIsolationDisable),
			want:       map[string]string{DefaultTagKey: DefaultTagValue, testClusterTagKey: testClusterName},
		},
		{
			name:       "enforce with extra tags",
			tags:       map[string]string{"team": "storage"},
			clusterTag: testClusterTag(ClusterTagIsolationEnforce),
			want:       map[string]string{DefaultTagKey: DefaultTagValue, testClusterTagKey: testClusterName, "team": "storage"},
		},
		{
			name: "deprecated --tags keeps overriding the default tag",
			tags: parseTagsFromStr("efs.csi.aws.com/cluster:custom"),
			want: map[string]string{DefaultTagKey: "custom"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := &Driver{tags: tc.tags, clusterTag: tc.clusterTag}
			if got := d.buildAccessPointTags(d.clusterTag); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func newClusterTagCreateRequest(params map[string]string) *csi.CreateVolumeRequest {
	p := map[string]string{
		ProvisioningMode: "efs-ap",
		FsId:             "fs-abcd1234",
		DirectoryPerms:   "777",
		Uid:              "1000",
		Gid:              "1000",
	}
	for k, v := range params {
		p[k] = v
	}
	return &csi.CreateVolumeRequest{
		Name: "volumeName",
		VolumeCapabilities: []*csi.VolumeCapability{{
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
		}},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 5368709120},
		Parameters:    p,
	}
}

func TestCreateVolumeClusterTag(t *testing.T) {
	ctx := context.Background()

	t.Run("new access point carries the cluster tag in disable state", func(t *testing.T) {
		mockCtl := gomock.NewController(t)
		mockCloud := mocks.NewMockCloud(mockCtl)
		d := &Driver{cloud: mockCloud, gidAllocator: NewGidAllocator(), lockManager: NewLockManagerMap(), clusterTag: testClusterTag(ClusterTagIsolationDisable)}
		mockCloud.EXPECT().CreateAccessPoint(gomock.Any(), gomock.Any(), gomock.Any(), util.FileSystemTypeEFS).
			DoAndReturn(func(_ context.Context, _ string, opts *cloud.AccessPointOptions, _ util.FileSystemType) (*cloud.AccessPoint, error) {
				if opts.Tags[testClusterTagKey] != testClusterName {
					t.Fatalf("expected cluster tag on create, got %v", opts.Tags)
				}
				return &cloud.AccessPoint{AccessPointId: "fsap-abcd1234", FileSystemId: "fs-abcd1234"}, nil
			})
		if _, err := d.CreateVolume(ctx, newClusterTagCreateRequest(nil)); err != nil {
			t.Fatalf("CreateVolume failed: %v", err)
		}
	})

	t.Run("new S3 Files access point carries the cluster tag", func(t *testing.T) {
		mockCtl := gomock.NewController(t)
		mockCloud := mocks.NewMockCloud(mockCtl)
		d := &Driver{cloud: mockCloud, gidAllocator: NewGidAllocator(), lockManager: NewLockManagerMap(), clusterTag: testClusterTag(ClusterTagIsolationEnforce)}
		mockCloud.EXPECT().CreateAccessPoint(gomock.Any(), gomock.Any(), gomock.Any(), util.FileSystemTypeS3Files).
			DoAndReturn(func(_ context.Context, _ string, opts *cloud.AccessPointOptions, _ util.FileSystemType) (*cloud.AccessPoint, error) {
				if opts.Tags[testClusterTagKey] != testClusterName {
					t.Fatalf("expected cluster tag on create, got %v", opts.Tags)
				}
				return &cloud.AccessPoint{AccessPointId: "fsap-abcd1234", FileSystemId: "fs-abcd1234"}, nil
			})
		if _, err := d.CreateVolume(ctx, newClusterTagCreateRequest(map[string]string{ProvisioningMode: "s3files-ap"})); err != nil {
			t.Fatalf("CreateVolume failed: %v", err)
		}
	})

	t.Run("enableTagging=false is rejected when the cluster tag is configured", func(t *testing.T) {
		mockCtl := gomock.NewController(t)
		mockCloud := mocks.NewMockCloud(mockCtl)
		d := &Driver{cloud: mockCloud, gidAllocator: NewGidAllocator(), lockManager: NewLockManagerMap(), clusterTag: testClusterTag(ClusterTagIsolationDisable)}
		_, err := d.CreateVolume(ctx, newClusterTagCreateRequest(map[string]string{EnableTagging: "false"}))
		if status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), testClusterTagKey) {
			t.Fatalf("expected InvalidArgument naming the cluster tag key, got %v", err)
		}
	})

	t.Run("enableTagging=false still works without the cluster tag", func(t *testing.T) {
		mockCtl := gomock.NewController(t)
		mockCloud := mocks.NewMockCloud(mockCtl)
		d := &Driver{cloud: mockCloud, gidAllocator: NewGidAllocator(), lockManager: NewLockManagerMap(), clusterTag: clusterTagConfig{clusterName: testClusterName, isolation: ClusterTagIsolationDisable}}
		mockCloud.EXPECT().CreateAccessPoint(gomock.Any(), gomock.Any(), gomock.Any(), util.FileSystemTypeEFS).
			Return(&cloud.AccessPoint{AccessPointId: "fsap-abcd1234", FileSystemId: "fs-abcd1234"}, nil)
		if _, err := d.CreateVolume(ctx, newClusterTagCreateRequest(map[string]string{EnableTagging: "false"})); err != nil {
			t.Fatalf("CreateVolume failed: %v", err)
		}
	})

	reuseParams := map[string]string{ReuseAccessPointKey: "true", PvcNameKey: "my-pvc"}
	existing := func(tags map[string]string) *cloud.AccessPoint {
		return &cloud.AccessPoint{
			AccessPointId: "fsap-abcd1234", FileSystemId: "fs-abcd1234", AccessPointRootDir: "/x",
			PosixUser: &cloud.PosixUser{Gid: 1000, Uid: 1000}, Tags: tags,
		}
	}
	for _, tc := range []struct {
		name      string
		isolation ClusterTagIsolation
		tags      map[string]string
		wantCode  codes.Code
	}{
		{"disable reuses an access point without the cluster tag", ClusterTagIsolationDisable, map[string]string{DefaultTagKey: DefaultTagValue}, codes.OK},
		{"backfill does not filter discovery", ClusterTagIsolationBackfill, map[string]string{DefaultTagKey: DefaultTagValue}, codes.OK},
		{"enforce reuses an access point tagged for this cluster", ClusterTagIsolationEnforce, map[string]string{testClusterTagKey: testClusterName}, codes.OK},
		{"enforce ignores an access point without the cluster tag", ClusterTagIsolationEnforce, map[string]string{DefaultTagKey: DefaultTagValue}, codes.FailedPrecondition},
		{"enforce ignores an access point tagged for another cluster", ClusterTagIsolationEnforce, map[string]string{testClusterTagKey: "other"}, codes.FailedPrecondition},
	} {
		t.Run("reuseAccessPoint: "+tc.name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			mockCloud := mocks.NewMockCloud(mockCtl)
			d := &Driver{cloud: mockCloud, gidAllocator: NewGidAllocator(), lockManager: NewLockManagerMap(), clusterTag: testClusterTag(tc.isolation)}
			mockCloud.EXPECT().FindAccessPointByClientToken(gomock.Any(), gomock.Any(), "fs-abcd1234", util.FileSystemTypeEFS).Return(existing(tc.tags), nil)
			_, err := d.CreateVolume(ctx, newClusterTagCreateRequest(reuseParams))
			if status.Code(err) != tc.wantCode {
				t.Fatalf("expected %v, got %v", tc.wantCode, err)
			}
		})
	}

	t.Run("enforce ignores an existing access point returned for the client token", func(t *testing.T) {
		mockCtl := gomock.NewController(t)
		mockCloud := mocks.NewMockCloud(mockCtl)
		d := &Driver{cloud: mockCloud, gidAllocator: NewGidAllocator(), lockManager: NewLockManagerMap(), clusterTag: testClusterTag(ClusterTagIsolationEnforce)}
		mockCloud.EXPECT().CreateAccessPoint(gomock.Any(), gomock.Any(), gomock.Any(), util.FileSystemTypeEFS).Return(nil, cloud.ErrAlreadyExists)
		mockCloud.EXPECT().FindAccessPointByClientToken(gomock.Any(), gomock.Any(), "fs-abcd1234", util.FileSystemTypeEFS).Return(existing(map[string]string{testClusterTagKey: "other"}), nil)
		_, err := d.CreateVolume(ctx, newClusterTagCreateRequest(nil))
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})
}

func TestClusterTagCrossAccount(t *testing.T) {
	d := &Driver{clusterTag: testClusterTag(ClusterTagIsolationEnforce)}
	crossAccount := map[string]string{RoleArn: "arn:aws:iam::123456789012:role/EFSCrossAccountRole"}

	if got := d.clusterTagFor(nil); got != d.clusterTag {
		t.Fatalf("same-account requests use the driver's settings, got %+v", got)
	}
	got := d.clusterTagFor(crossAccount)
	if got.enabled() || got.enforce() {
		t.Fatalf("awsRoleArn requests get no cluster tag and no enforce checks, got %+v", got)
	}
	if tags := d.buildAccessPointTags(got); tags[testClusterTagKey] != "" {
		t.Fatalf("cross-account access points must not get the cluster tag, got %v", tags)
	}
	untagged := &cloud.AccessPoint{AccessPointId: "fsap-00000001", Tags: map[string]string{DefaultTagKey: DefaultTagValue}}
	if err := d.checkClusterTag(got, untagged); err != nil {
		t.Fatalf("enforce must not reject cross-account access points, got %v", err)
	}
	if err := d.checkClusterTag(d.clusterTagFor(nil), untagged); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("enforce must still reject same-account access points, got %v", err)
	}

	// enableTagging=false is allowed for cross-account StorageClasses: the request
	// gets past the cluster tag check and fails later, when it assumes the role.
	mockCtl := gomock.NewController(t)
	d.cloud = mocks.NewMockCloud(mockCtl)
	d.gidAllocator = NewGidAllocator()
	_, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name:               "pvc-cross-account",
		VolumeCapabilities: []*csi.VolumeCapability{{AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}}, AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}}},
		Parameters:         map[string]string{ProvisioningMode: "efs-ap", FsId: "fs-abcd1234", DirectoryPerms: "700", EnableTagging: "false"},
		Secrets:            crossAccount,
	})
	if status.Code(err) == codes.InvalidArgument {
		t.Fatalf("enableTagging=false must not be rejected for a cross-account StorageClass, got %v", err)
	}
}

func TestDeleteVolumeRootDirClusterTag(t *testing.T) {
	ctx := context.Background()
	mine := map[string]string{testClusterTagKey: testClusterName}
	other := map[string]string{testClusterTagKey: "other"}
	testCases := []struct {
		name     string
		fsType   util.FileSystemType
		apTags   map[string]string // Tags returned by DescribeAccessPoint
		refetch  map[string]string // tags returned by GetAccessPointTags, when called
		refetchE error
		wantCode codes.Code
		removed  bool // the root directory is removed and the access point deleted
	}{
		{name: "EFS access point tagged for this cluster", fsType: util.FileSystemTypeEFS, apTags: mine, wantCode: codes.OK, removed: true},
		{name: "EFS access point tagged for another cluster", fsType: util.FileSystemTypeEFS, apTags: other, wantCode: codes.FailedPrecondition},
		{name: "S3 Files access point without tags in the description, tagged for this cluster", fsType: util.FileSystemTypeS3Files, refetch: mine, wantCode: codes.OK, removed: true},
		{name: "S3 Files access point without tags in the description, tagged for another cluster", fsType: util.FileSystemTypeS3Files, refetch: other, wantCode: codes.FailedPrecondition},
		{name: "reading the tags fails", fsType: util.FileSystemTypeS3Files, refetchE: errors.New("throttled"), wantCode: codes.Internal},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			mockCloud := mocks.NewMockCloud(mockCtl)
			mockMounter := mocks.NewMockMounter(mockCtl)
			d := &Driver{cloud: mockCloud, mounter: mockMounter, lockManager: NewLockManagerMap(), deleteAccessPointRootDir: true, clusterTag: testClusterTag(ClusterTagIsolationEnforce)}

			volumeID := "fs-abcd1234::fsap-abcd1234"
			if tc.fsType == util.FileSystemTypeS3Files {
				volumeID = "s3files:" + volumeID
			}
			mockCloud.EXPECT().DescribeAccessPoint(gomock.Any(), "fsap-abcd1234", "fs-abcd1234", tc.fsType).
				Return(&cloud.AccessPoint{AccessPointId: "fsap-abcd1234", FileSystemId: "fs-abcd1234", AccessPointRootDir: "/x", Tags: tc.apTags}, nil)
			if tc.apTags == nil {
				mockCloud.EXPECT().GetAccessPointTags(gomock.Any(), "fsap-abcd1234", tc.fsType).Return(tc.refetch, tc.refetchE)
			}
			if tc.removed {
				dir := mocks.NewMockFileInfo("x", 0, 0755, time.Now(), true, nil)
				mockMounter.EXPECT().MakeDir(gomock.Any()).Return(nil)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil)
				mockMounter.EXPECT().Mount("fs-abcd1234", gomock.Any(), tc.fsType.String(), gomock.Any()).Return(nil)
				mockMounter.EXPECT().Stat(gomock.Any()).Return(dir, nil)
				mockMounter.EXPECT().Unmount(gomock.Any()).Return(nil)
				mockCloud.EXPECT().DeleteAccessPoint(gomock.Any(), "fsap-abcd1234", tc.fsType).Return(nil)
			} else {
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Any()).Return(true, nil).AnyTimes()
			}

			_, err := d.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
			if status.Code(err) != tc.wantCode {
				t.Fatalf("expected %v, got %v", tc.wantCode, err)
			}
		})
	}
}

// fakeTagCloud is a cloud.Cloud for the backfill sweep.
type fakeTagCloud struct {
	cloud.Cloud
	apTags   map[string]map[string]string
	fsTags   map[string]map[string]string
	tagErr   error
	tagCalls []map[string]string
}

func (f *fakeTagCloud) GetAccessPointTags(_ context.Context, id string, _ util.FileSystemType) (map[string]string, error) {
	tags, ok := f.apTags[id]
	if !ok {
		return nil, cloud.ErrNotFound
	}
	return tags, nil
}

func (f *fakeTagCloud) GetFileSystemTags(_ context.Context, id string, _ util.FileSystemType) (map[string]string, error) {
	tags, ok := f.fsTags[id]
	if !ok {
		return nil, cloud.ErrNotFound
	}
	return tags, nil
}

func (f *fakeTagCloud) TagAccessPoint(_ context.Context, id string, tags map[string]string, _ util.FileSystemType) error {
	f.tagCalls = append(f.tagCalls, tags)
	if f.tagErr != nil {
		return f.tagErr
	}
	for k, v := range tags {
		f.apTags[id][k] = v
	}
	return nil
}

func efsPV(name, handle string, dynamic bool) *corev1.PersistentVolume {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{}},
		Spec: corev1.PersistentVolumeSpec{PersistentVolumeSource: corev1.PersistentVolumeSource{
			CSI: &corev1.CSIPersistentVolumeSource{Driver: driverName, VolumeHandle: handle},
		}},
	}
	if dynamic {
		pv.Annotations[provisionedByAnnotation] = driverName
		pv.Spec.StorageClassName = testStorageClassName
	}
	return pv
}

// testStorageClassName is the StorageClass of dynamically provisioned test PVs.
const testStorageClassName = "efs"

func efsSC(name string, params map[string]string) *storagev1.StorageClass {
	return &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: name}, Provisioner: driverName, Parameters: params}
}

// withStorageClass returns pv with its StorageClass set to sc.
func withStorageClass(pv *corev1.PersistentVolume, sc string) *corev1.PersistentVolume {
	pv.Spec.StorageClassName = sc
	return pv
}

func TestClusterTagBackfillSweep(t *testing.T) {
	ctx := context.Background()
	mine := map[string]string{testClusterTagKey: testClusterName}
	legacy := func() map[string]string { return map[string]string{DefaultTagKey: DefaultTagValue} }

	newBackfiller := func(f *fakeTagCloud, objs ...interface{}) *clusterTagBackfiller {
		rt := []interface{}{efsSC(testStorageClassName, map[string]string{ProvisioningMode: "efs-ap"})}
		rt = append(rt, objs...)
		client := fake.NewSimpleClientset()
		for _, o := range rt {
			switch v := o.(type) {
			case *corev1.PersistentVolume:
				_, _ = client.CoreV1().PersistentVolumes().Create(ctx, v, metav1.CreateOptions{})
			case *storagev1.StorageClass:
				_, _ = client.StorageV1().StorageClasses().Create(ctx, v, metav1.CreateOptions{})
			}
		}
		return &clusterTagBackfiller{
			cloud:         f,
			clusterTag:    testClusterTag(ClusterTagIsolationBackfill),
			newKubeClient: func() (kubernetes.Interface, error) { return client, nil },
		}
	}

	t.Run("tags owned legacy access points with only the cluster tag", func(t *testing.T) {
		f := &fakeTagCloud{
			apTags: map[string]map[string]string{"fsap-00000001": legacy(), "fsap-00000002": {DefaultTagKey: DefaultTagValue, testClusterTagKey: testClusterName}},
			fsTags: map[string]map[string]string{"fs-00000001": mine},
		}
		b := newBackfiller(f, efsPV("a", "fs-00000001::fsap-00000001", true), efsPV("b", "fs-00000001::fsap-00000002", true))
		res := b.sweep(ctx)
		if res.counts() != [4]int{1, 0, 0, 0} {
			t.Fatalf("unexpected result %+v", res)
		}
		if len(f.tagCalls) != 1 || !reflect.DeepEqual(f.tagCalls[0], mine) {
			t.Fatalf("expected one TagResource with only the cluster tag, got %v", f.tagCalls)
		}
		if !b.completeOnce {
			t.Fatalf("expected completion after a clean sweep")
		}
		// A second sweep has nothing to do.
		if res := b.sweep(ctx); res.counts() != [4]int{} {
			t.Fatalf("unexpected second result %+v", res)
		}
	})

	t.Run("reports resources it does not own", func(t *testing.T) {
		f := &fakeTagCloud{
			apTags: map[string]map[string]string{
				"fsap-0000000a": legacy(),
				"fsap-0000000b": {},
				"fsap-0000000c": {DefaultTagKey: DefaultTagValue, testClusterTagKey: "other"},
				"fsap-0000000d": legacy(),
			},
			fsTags: map[string]map[string]string{"fs-00000001": {}, "fs-00000002": mine, "fs-00000003": {}},
		}
		sc := efsSC("efs-fs3", map[string]string{FsId: "fs-00000003", ProvisioningMode: "efs-ap"})
		otherSC := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "ebs"}, Provisioner: "ebs.csi.aws.com", Parameters: map[string]string{FsId: "fs-00000009"}}
		b := newBackfiller(f,
			efsPV("static", "fs-00000001::fsap-0000000a", false),
			efsPV("nomark", "fs-00000002::fsap-0000000b", true),
			efsPV("other", "fs-00000002::fsap-0000000c", true),
			efsPV("dyn", "fs-00000002::fsap-0000000d", true),
			efsPV("plain", "fs-00000001", false),
			sc, otherSC)
		res := b.sweep(ctx)
		if want := [4]int{1, 0, 5, 0}; res.counts() != want {
			t.Fatalf("expected %v, got %+v", want, res)
		}
		wantAdmin := []string{"fsap-0000000a", "fsap-0000000b", "fsap-0000000c", "fs-00000001", "fs-00000003"}
		if !reflect.DeepEqual(res.needsAdministratorResources, wantAdmin) {
			t.Fatalf("needsAdministratorResources: expected %v, got %v", wantAdmin, res.needsAdministratorResources)
		}
		if b.completeOnce {
			t.Fatalf("backfill must not report completion while resources need an administrator")
		}
	})

	t.Run("failed tag calls block completion", func(t *testing.T) {
		f := &fakeTagCloud{
			apTags: map[string]map[string]string{"fsap-00000001": legacy()},
			fsTags: map[string]map[string]string{"fs-00000001": mine},
			tagErr: errors.New("AccessDeniedException"),
		}
		b := newBackfiller(f, efsPV("a", "fs-00000001::fsap-00000001", true))
		res := b.sweep(ctx)
		if res.counts() != [4]int{0, 1, 0, 0} {
			t.Fatalf("a failed tag call counts in failed only, got %+v", res)
		}
		if !reflect.DeepEqual(res.failedResources, []string{"fsap-00000001"}) || res.firstError == nil || res.firstError.Error() != "AccessDeniedException" {
			t.Fatalf("unexpected failedResources or firstError: %+v", res)
		}
		if b.completeOnce {
			t.Fatalf("backfill must not report completion after a failure")
		}
	})

	t.Run("resources it cannot find are reported", func(t *testing.T) {
		// For example, cross-account volumes, which the controller's own
		// credentials cannot see.
		f := &fakeTagCloud{apTags: map[string]map[string]string{}, fsTags: map[string]map[string]string{}}
		b := newBackfiller(f, efsPV("a", "fs-00000001::fsap-0000000e", true))
		res := b.sweep(ctx)
		if res.counts() != [4]int{0, 0, 2, 0} {
			t.Fatalf("unexpected result %+v", res)
		}
		if want := []string{"fsap-0000000e", "fs-00000001"}; !reflect.DeepEqual(res.needsAdministratorResources, want) {
			t.Fatalf("needsAdministratorResources: expected %v, got %v", want, res.needsAdministratorResources)
		}
		if b.completeOnce {
			t.Fatalf("backfill must not report completion while resources cannot be found")
		}
	})

	t.Run("access points that other clusters may reuse are reported", func(t *testing.T) {
		f := &fakeTagCloud{
			apTags: map[string]map[string]string{
				"fsap-00000011": legacy(),
				"fsap-00000012": legacy(),
				"fsap-00000013": {DefaultTagKey: DefaultTagValue, testClusterTagKey: testClusterName},
			},
			fsTags: map[string]map[string]string{"fs-00000001": mine},
		}
		reuseSC := efsSC("efs-reuse", map[string]string{ProvisioningMode: "efs-ap", ReuseAccessPointKey: "true"})
		b := newBackfiller(f,
			withStorageClass(efsPV("reuse", "fs-00000001::fsap-00000011", true), reuseSC.Name),
			withStorageClass(efsPV("deleted-sc", "fs-00000001::fsap-00000012", true), "deleted"),
			withStorageClass(efsPV("reuse-tagged", "fs-00000001::fsap-00000013", true), reuseSC.Name),
			reuseSC)
		res := b.sweep(ctx)
		if res.counts() != [4]int{0, 0, 2, 0} {
			t.Fatalf("unexpected result %+v", res)
		}
		if want := []string{"fsap-00000011", "fsap-00000012"}; !reflect.DeepEqual(res.needsAdministratorResources, want) {
			t.Fatalf("needsAdministratorResources: expected %v, got %v", want, res.needsAdministratorResources)
		}
		if len(f.tagCalls) != 0 {
			t.Fatalf("backfill must not tag access points that other clusters may reuse, got %v", f.tagCalls)
		}
	})

	t.Run("cross-account volumes are left out", func(t *testing.T) {
		// The controller's own credentials cannot see the other account's
		// resources, so the fake cloud knows none of them.
		f := &fakeTagCloud{apTags: map[string]map[string]string{}, fsTags: map[string]map[string]string{}}
		xSC := efsSC("efs-x-account", map[string]string{ProvisioningMode: "efs-ap", FsId: "fs-000000aa", provisionerSecretNameKey: "x-account"})
		b := newBackfiller(f, withStorageClass(efsPV("x", "fs-000000aa::fsap-000000aa", true), xSC.Name), xSC)
		res := b.sweep(ctx)
		if res.counts() != [4]int{} {
			t.Fatalf("cross-account volumes must not be tagged or reported, got %+v", res)
		}
		if !b.completeOnce {
			t.Fatalf("expected completion when only cross-account volumes exist")
		}
	})

	t.Run("Kubernetes errors count as remaining", func(t *testing.T) {
		b := &clusterTagBackfiller{
			cloud:         &fakeTagCloud{},
			clusterTag:    testClusterTag(ClusterTagIsolationBackfill),
			newKubeClient: func() (kubernetes.Interface, error) { return nil, errors.New("no cluster") },
		}
		if res := b.sweep(ctx); res.counts() != [4]int{0, 0, 0, 1} {
			t.Fatalf("unexpected result %+v", res)
		}
	})
}

// counts returns tagged, failed, needsAdministrator and remaining.
func (r backfillResult) counts() [4]int {
	return [4]int{r.tagged, r.failed, r.needsAdministrator, r.remaining}
}

// syncBuffer is a goroutine-safe klog sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureKlog sends klog output to a buffer at the default verbosity (0).
func captureKlog(t *testing.T) *syncBuffer {
	t.Helper()
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	_ = fs.Set("logtostderr", "false")
	_ = fs.Set("alsologtostderr", "false")
	buf := &syncBuffer{}
	klog.SetOutput(buf)
	t.Cleanup(func() {
		klog.Flush()
		_ = fs.Set("logtostderr", "true")
	})
	return buf
}

// logMessages returns the structured part (after "] ") of each klog line in out.
func logMessages(out string) []string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if _, msg, ok := strings.Cut(l, "] "); ok {
			lines = append(lines, msg)
		}
	}
	return lines
}

func TestClusterTagBackfillLogLines(t *testing.T) {
	ctx := context.Background()
	buf := captureKlog(t)

	client := fake.NewSimpleClientset(efsSC(testStorageClassName, map[string]string{ProvisioningMode: "efs-ap"}))
	f := &fakeTagCloud{apTags: map[string]map[string]string{}, fsTags: map[string]map[string]string{}, tagErr: errors.New("AccessDeniedException")}
	var admin []string
	for i := 0; i < 22; i++ {
		id := fmt.Sprintf("fsap-%08d", i)
		f.apTags[id] = map[string]string{}
		_, _ = client.CoreV1().PersistentVolumes().Create(ctx, efsPV("static-"+id, "fs-00000001::"+id, false), metav1.CreateOptions{})
		if i < 20 {
			admin = append(admin, id)
		}
	}
	f.apTags["fsap-000000ff"] = map[string]string{DefaultTagKey: DefaultTagValue}
	_, _ = client.CoreV1().PersistentVolumes().Create(ctx, efsPV("dyn", "fs-00000001::fsap-000000ff", true), metav1.CreateOptions{})
	f.fsTags["fs-00000001"] = map[string]string{testClusterTagKey: testClusterName}
	b := &clusterTagBackfiller{
		cloud:         f,
		clusterTag:    testClusterTag(ClusterTagIsolationBackfill),
		newKubeClient: func() (kubernetes.Interface, error) { return client, nil },
	}

	// Sweep 1: 22 resources for an administrator (list capped at 20) and one failed tag call.
	b.sweep(ctx)
	// Sweep 2: clean, logs complete.
	empty := fake.NewSimpleClientset()
	b.newKubeClient = func() (kubernetes.Interface, error) { return empty, nil }
	b.sweep(ctx)
	// Sweep 3: clean again, does not log complete again.
	b.sweep(ctx)
	// Sweep 4: not clean; complete is not logged again later in this process.
	b.newKubeClient = func() (kubernetes.Interface, error) { return nil, errors.New("no cluster") }
	b.sweep(ctx)
	b.newKubeClient = func() (kubernetes.Interface, error) { return empty, nil }
	b.sweep(ctx)
	klog.Flush()

	want := []string{
		`"cluster tag backfill sweep finished" tagged=0 failed=1 needsAdministrator=22 remaining=0 needsAdministratorResources=["` + strings.Join(admin, `","`) + `"] needsAdministratorTotal=22 failedResources=["fsap-000000ff"] firstError="AccessDeniedException"`,
		`"cluster tag backfill sweep finished" tagged=0 failed=0 needsAdministrator=0 remaining=0`,
		`"cluster tag backfill complete" clusterName="my-cluster" tagKey="example.com/cluster"`,
		`"cluster tag backfill sweep finished" tagged=0 failed=0 needsAdministrator=0 remaining=0`,
		`"cluster tag backfill sweep finished" tagged=0 failed=0 needsAdministrator=0 remaining=1 firstError="no cluster"`,
		`"cluster tag backfill sweep finished" tagged=0 failed=0 needsAdministrator=0 remaining=0`,
	}
	if got := logMessages(buf.String()); !reflect.DeepEqual(got, want) {
		t.Fatalf("log lines:\n got %q\nwant %q", got, want)
	}
}

func TestClusterTagSettingsAndEnforceLogLines(t *testing.T) {
	buf := captureKlog(t)
	testClusterTag(ClusterTagIsolationBackfill).logSettings()
	clusterTagConfig{isolation: ClusterTagIsolationDisable}.logSettings()
	d := &Driver{clusterTag: testClusterTag(ClusterTagIsolationEnforce)}
	_ = d.checkClusterTag(d.clusterTag, &cloud.AccessPoint{AccessPointId: "fsap-00000001", Tags: map[string]string{testClusterTagKey: "other"}})
	_ = d.checkClusterTag(d.clusterTag, &cloud.AccessPoint{AccessPointId: "fsap-00000002"})
	klog.Flush()
	want := []string{
		`"cluster tag isolation settings" clusterName="my-cluster" tagKey="example.com/cluster" clusterTagIsolation="backfill" backfillInterval="1h0m0s"`,
		`"cluster tag isolation settings" clusterName="" tagKey="" clusterTagIsolation="disable"`,
		`"ignoring resource not tagged for this cluster" resource="fsap-00000001" tagKey="example.com/cluster" clusterName="my-cluster" tagValue="other"`,
		`"ignoring resource not tagged for this cluster" resource="fsap-00000002" tagKey="example.com/cluster" clusterName="my-cluster" tagValue=""`,
	}
	if got := logMessages(buf.String()); !reflect.DeepEqual(got, want) {
		t.Fatalf("log lines:\n got %q\nwant %q", got, want)
	}
}

func TestParseClusterTagIsolation(t *testing.T) {
	enum := func(v string) string {
		return `invalid --cluster-tag-isolation "` + v + `": must be "disable", "backfill" or "enforce"`
	}
	testCases := []struct {
		in      string
		want    ClusterTagIsolation
		wantErr string
	}{
		{in: "", want: ClusterTagIsolationDisable},
		{in: "disable", want: ClusterTagIsolationDisable},
		{in: "backfill", want: ClusterTagIsolationBackfill},
		{in: "enforce", want: ClusterTagIsolationEnforce},
		{in: "off", wantErr: enum("off")},
		{in: "false", wantErr: enum("false")},
		{in: "true", wantErr: enum("true")},
		{in: "on", wantErr: enum("on")},
		{in: "No", wantErr: enum("No")},
		{in: "OFF", wantErr: enum("OFF")},
		{in: " disable", wantErr: enum(" disable")},
		{in: "Disable", wantErr: enum("Disable")},
		{in: "strict", wantErr: enum("strict")},
	}
	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseClusterTagIsolation(tc.in)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("expected error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("expected %q, got %q, %v", tc.want, got, err)
			}
		})
	}
	// The flag path returns the same message.
	if _, err := parseTestOptions(t, "--cluster-name=my-cluster", "--cluster-tag-key=example.com/cluster", "--cluster-tag-isolation=off"); err == nil || err.Error() != enum("off") {
		t.Fatalf("expected the invalid-value error from Validate, got %v", err)
	}
}

func TestAddFileSystemNormalizesDNSName(t *testing.T) {
	fss := map[string]sweepFileSystem{}
	addFileSystem(fss, "fs-abcd1234.efs.us-west-2.amazonaws.com", util.FileSystemTypeEFS)
	addFileSystem(fss, "", util.FileSystemTypeEFS)
	if _, ok := fss["fs-abcd1234"]; !ok || len(fss) != 1 {
		t.Fatalf("unexpected file systems %v", fss)
	}
}
