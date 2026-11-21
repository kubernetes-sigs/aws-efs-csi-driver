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
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

const workloadTestRole = "arn:aws:iam::123456789012:role/WorkloadRole"

func newWorkloadTestRequest(target string) *csi.NodePublishVolumeRequest {
	return &csi.NodePublishVolumeRequest{
		VolumeId:   "fs-abcd1234::fsap-00000001",
		TargetPath: target,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{MountFlags: []string{"workloadiam"}}},
		},
		VolumeContext: map[string]string{podName: "workload", podNamespace: "test", podUID: "pod-uid", serviceAccountName: "workload-sa"},
	}
}

func setWorkloadTestToken(t *testing.T, req *csi.NodePublishVolumeRequest, expiration time.Time, audience json.RawMessage, accountUID string) string {
	t.Helper()
	var claims workloadTokenClaims
	claims.Audience = audience
	claims.Expiration = expiration.Unix()
	claims.Subject = "system:serviceaccount:" + req.VolumeContext[podNamespace] + ":" + req.VolumeContext[serviceAccountName]
	claims.Kubernetes.Namespace = req.VolumeContext[podNamespace]
	claims.Kubernetes.Pod.Name = req.VolumeContext[podName]
	claims.Kubernetes.Pod.UID = req.VolumeContext[podUID]
	claims.Kubernetes.ServiceAccount.Name = req.VolumeContext[serviceAccountName]
	claims.Kubernetes.ServiceAccount.UID = accountUID
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	// STS verifies JWT signatures. The fake mounter exercises the CSI side of the handoff.
	token := "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(payload) + ".test-signature"
	tokens, err := json.Marshal(map[string]serviceAccountToken{
		stsAudience: {Token: token, Expiration: time.Unix(expiration.Unix()-1, 0).UTC()},
	})
	if err != nil {
		t.Fatal(err)
	}
	req.Secrets = map[string]string{serviceAccountTokens: string(tokens)}
	return token
}

func newWorkloadTestAccount(name, uid, role string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: "test", UID: types.UID(uid), Annotations: map[string]string{workloadRoleAnnotation: role},
	}}
}

func TestWorkloadIAMMountLifecycle(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mounter := mocks.NewMockMounter(gomock.NewController(t))
	otherRole := "arn:aws:iam::123456789012:role/team/Read=Only,Audit+"
	client := fake.NewSimpleClientset(
		newWorkloadTestAccount("workload-sa", "sa-uid", workloadTestRole),
		newWorkloadTestAccount("other-sa", "other-uid", otherRole),
	)
	workload := &workloadIdentity{client: client.CoreV1(), tokenDir: filepath.Join(root, "tokens")}
	d := &Driver{mounter: mounter, workloadIdentity: workload, metaDir: filepath.Join(root, "mounts")}
	req := newWorkloadTestRequest(filepath.Join(root, "first"))
	other := newWorkloadTestRequest(filepath.Join(root, "second"))
	other.VolumeContext[serviceAccountName] = "other-sa"
	other.VolumeContext[podUID] = "other-pod-uid"
	expiration := time.Now().Add(time.Hour)
	first := setWorkloadTestToken(t, req, expiration, json.RawMessage(`"sts.amazonaws.com"`), "sa-uid")
	second := setWorkloadTestToken(t, other, expiration, json.RawMessage(`["sts.amazonaws.com"]`), "other-uid")

	publish := func(d *Driver, req *csi.NodePublishVolumeRequest, want codes.Code) {
		t.Helper()
		if _, err := d.NodePublishVolume(ctx, req); status.Code(err) != want {
			t.Fatalf("NodePublishVolume returned %v, want %s", err, want)
		}
	}
	var firstPath, secondPath string
	expectMount := func(req *csi.NodePublishVolumeRequest, role, token string, tokenPath *string) {
		t.Helper()
		mounter.EXPECT().GetDeviceName(req.TargetPath).Return("", 0, nil)
		mounter.EXPECT().MakeDir(req.TargetPath).Return(nil)
		mounter.EXPECT().Mount("fs-abcd1234:/", req.TargetPath, "efs", gomock.Any()).DoAndReturn(
			func(_, _, _ string, options []string) error {
				if !hasOption(options, "requirewebidentity="+url.QueryEscape(role)) ||
					!hasOption(options, "iam") || !hasOption(options, "tls") || !hasOption(options, "accesspoint=fsap-00000001") {
					t.Fatalf("unexpected mount options: %v", options)
				}
				for _, option := range options {
					if path, ok := strings.CutPrefix(option, "jwtpath="); ok {
						*tokenPath = path
					}
				}
				assertStoredWorkloadToken(t, *tokenPath, token)
				return nil
			},
		)
	}
	// The same volume can be mounted for different workloads on the same node.
	expectMount(req, workloadTestRole, first, &firstPath)
	expectMount(other, otherRole, second, &secondPath)
	publish(d, req, codes.OK)
	publish(d, other, codes.OK)
	if firstPath == secondPath {
		t.Fatal("the two workloads shared a token file")
	}
	metadata, err := os.ReadFile(d.efsMetaPath(req.TargetPath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadata), firstPath) || strings.Contains(string(metadata), first) {
		t.Fatal("mount metadata contained workload token material")
	}

	// Republish after a driver restart, then deliver an older request out of order.
	restarted := &Driver{mounter: mounter, workloadIdentity: workload, metaDir: d.metaDir}
	refreshed := setWorkloadTestToken(t, req, expiration.Add(time.Hour), json.RawMessage(`"sts.amazonaws.com"`), "sa-uid")
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("efs", 1, nil).Times(2)
	publish(restarted, req, codes.OK)
	setWorkloadTestToken(t, req, expiration, json.RawMessage(`"sts.amazonaws.com"`), "sa-uid")
	publish(restarted, req, codes.OK)
	assertStoredWorkloadToken(t, firstPath, refreshed)
	assertStoredWorkloadToken(t, secondPath, second)

	// A role change cannot republish an existing mount under a different identity.
	_, err = client.CoreV1().ServiceAccounts("test").Update(ctx,
		newWorkloadTestAccount("workload-sa", "sa-uid", "arn:aws:iam::123456789012:role/UpdatedRole"), metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("efs", 1, nil)
	publish(restarted, req, codes.FailedPrecondition)
	assertStoredWorkloadToken(t, firstPath, refreshed)

	unpublish := &csi.NodeUnpublishVolumeRequest{TargetPath: req.TargetPath, VolumeId: req.VolumeId}
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("efs", 1, nil).Times(2)
	mounter.EXPECT().Unmount(req.TargetPath).Return(errors.New("mount is busy"))
	if _, err := restarted.NodeUnpublishVolume(ctx, unpublish); status.Code(err) != codes.Internal {
		t.Fatalf("NodeUnpublishVolume returned %v, want a failed unmount", err)
	}
	assertStoredWorkloadToken(t, firstPath, refreshed)
	mounter.EXPECT().Unmount(req.TargetPath).Return(nil)
	if _, err := restarted.NodeUnpublishVolume(ctx, unpublish); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatalf("workload token still exists after unpublish: %v", err)
	}
	assertStoredWorkloadToken(t, secondPath, second)
}

func TestWorkloadIAMRefreshWithFallbackMountTarget(t *testing.T) {
	root := t.TempDir()
	ctrl := gomock.NewController(t)
	mounter := mocks.NewMockMounter(ctrl)
	cloud := mocks.NewMockCloud(ctrl)
	cloud.EXPECT().GetMetadata().Return(&mockMetadata{availabilityZone: "us-west-2d"}).AnyTimes()
	client := fake.NewSimpleClientset(newWorkloadTestAccount("workload-sa", "sa-uid", workloadTestRole))
	d := &Driver{
		mounter:          mounter,
		cloud:            cloud,
		workloadIdentity: &workloadIdentity{client: client.CoreV1(), tokenDir: filepath.Join(root, "tokens")},
		metaDir:          filepath.Join(root, "mounts"),
	}
	req := newWorkloadTestRequest(filepath.Join(root, "target"))
	req.VolumeContext[MountTargetIpMap] = `{"us-west-2a":"192.0.2.10","us-west-2b":"192.0.2.20","us-west-2c":"192.0.2.30"}`
	audience := json.RawMessage(`"sts.amazonaws.com"`)
	expiration := time.Now().Add(time.Hour)
	setWorkloadTestToken(t, req, expiration, audience, "sa-uid")
	var tokenPath string
	gomock.InOrder(
		mounter.EXPECT().GetDeviceName(req.TargetPath).Return("", 0, nil),
		mounter.EXPECT().MakeDir(req.TargetPath).Return(nil),
		mounter.EXPECT().Mount("fs-abcd1234:/", req.TargetPath, "efs", gomock.Any()).DoAndReturn(
			func(_, _, _ string, options []string) error {
				for _, option := range options {
					if path, ok := strings.CutPrefix(option, "jwtpath="); ok {
						tokenPath = path
					}
				}
				return nil
			},
		),
		mounter.EXPECT().GetDeviceName(req.TargetPath).Return("efs", 1, nil).Times(20),
	)
	ctx := context.Background()
	if _, err := d.NodePublishVolume(ctx, req); err != nil {
		t.Fatal(err)
	}

	refreshed := setWorkloadTestToken(t, req, expiration.Add(time.Hour), audience, "sa-uid")
	for i := 0; i < 20; i++ {
		if _, err := d.NodePublishVolume(ctx, req); err != nil {
			t.Fatalf("republishing an unchanged volume returned %v", err)
		}
	}
	assertStoredWorkloadToken(t, tokenPath, refreshed)
}

func assertStoredWorkloadToken(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != want {
		t.Fatal("saved token differs from the token kubelet sent")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("token file has permissions %o, want 0600", info.Mode().Perm())
	}
}

func TestWorkloadIAMRejectsInvalidMounts(t *testing.T) {
	tests := []struct {
		name        string
		change      func(*Driver, *csi.NodePublishVolumeRequest, *corev1.ServiceAccount)
		code        codes.Code
		checkTarget bool
	}{
		{"feature disabled", func(d *Driver, _ *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			d.workloadIdentity = nil
		}, codes.FailedPrecondition, false},
		{"missing token", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			req.Secrets = nil
		}, codes.Unauthenticated, true},
		{"token in volume context", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			req.VolumeContext[serviceAccountTokens] = req.Secrets[serviceAccountTokens]
			req.Secrets = nil
		}, codes.Unauthenticated, true},
		{"token for another pod", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			req.VolumeContext[podUID] = "another-uid"
		}, codes.Unauthenticated, true},
		{"expired token", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			setWorkloadTestToken(t, req, time.Now().Add(-time.Minute), json.RawMessage(`"sts.amazonaws.com"`), "sa-uid")
		}, codes.Unauthenticated, true},
		{"wrong token audience", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			setWorkloadTestToken(t, req, time.Now().Add(time.Hour), json.RawMessage(`"kubernetes"`), "sa-uid")
		}, codes.Unauthenticated, true},
		{"recreated service account", func(_ *Driver, _ *csi.NodePublishVolumeRequest, sa *corev1.ServiceAccount) {
			sa.UID = "recreated"
		}, codes.PermissionDenied, true},
		{"missing IAM role", func(_ *Driver, _ *csi.NodePublishVolumeRequest, sa *corev1.ServiceAccount) {
			sa.Annotations = nil
		}, codes.FailedPrecondition, true},
		{"invalid IAM role", func(_ *Driver, _ *csi.NodePublishVolumeRequest, sa *corev1.ServiceAccount) {
			sa.Annotations[workloadRoleAnnotation] = workloadTestRole + "\n"
		}, codes.FailedPrecondition, true},
		{"alternate provider", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			req.VolumeCapability.GetMount().MountFlags = append(req.VolumeCapability.GetMount().MountFlags, "awsprofile=default")
		}, codes.InvalidArgument, true},
		{"embedded credential option", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			req.VolumeCapability.GetMount().MountFlags = append(req.VolumeCapability.GetMount().MountFlags, "ro,awsprofile=default")
		}, codes.InvalidArgument, true},
		{"TLS disabled", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			req.VolumeContext["encryptInTransit"] = "false"
		}, codes.FailedPrecondition, false},
		{"S3 Files volume", func(_ *Driver, req *csi.NodePublishVolumeRequest, _ *corev1.ServiceAccount) {
			req.VolumeId = "s3files:fs-abcd1234::fsap-00000001"
		}, codes.FailedPrecondition, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			req := newWorkloadTestRequest(filepath.Join(root, "target"))
			setWorkloadTestToken(t, req, time.Now().Add(time.Hour), json.RawMessage(`"sts.amazonaws.com"`), "sa-uid")
			account := newWorkloadTestAccount("workload-sa", "sa-uid", workloadTestRole)
			mounter := mocks.NewMockMounter(gomock.NewController(t))
			d := &Driver{mounter: mounter, workloadIdentity: &workloadIdentity{tokenDir: filepath.Join(root, "tokens")}}
			test.change(d, req, account)
			if d.workloadIdentity != nil {
				d.workloadIdentity.client = fake.NewSimpleClientset(account).CoreV1()
			}
			if test.checkTarget {
				mounter.EXPECT().GetDeviceName(req.TargetPath).Return("", 0, nil)
				mounter.EXPECT().MakeDir(req.TargetPath).Return(nil)
			}
			if _, err := d.NodePublishVolume(context.Background(), req); status.Code(err) != test.code {
				t.Fatalf("NodePublishVolume returned %v, want %s", err, test.code)
			}
			if _, err := os.Stat(filepath.Join(root, "tokens")); !os.IsNotExist(err) {
				t.Fatalf("invalid request created a token directory: %v", err)
			}
		})
	}
}

func TestWorkloadIAMMountFailureCleansUpTokenAndAllowsRetry(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mounter := mocks.NewMockMounter(gomock.NewController(t))
	client := fake.NewSimpleClientset(newWorkloadTestAccount("workload-sa", "sa-uid", workloadTestRole))
	d := &Driver{
		mounter:          mounter,
		workloadIdentity: &workloadIdentity{client: client.CoreV1(), tokenDir: filepath.Join(root, "tokens")},
		metaDir:          filepath.Join(root, "mounts"),
	}
	req := newWorkloadTestRequest(filepath.Join(root, "target"))
	setWorkloadTestToken(t, req, time.Now().Add(time.Hour), json.RawMessage(`"sts.amazonaws.com"`), "sa-uid")
	unpublish := &csi.NodeUnpublishVolumeRequest{TargetPath: req.TargetPath, VolumeId: req.VolumeId}

	mounter.EXPECT().MakeDir(req.TargetPath).Return(nil).Times(2)
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("", 0, nil).Times(3)
	mounter.EXPECT().Mount("fs-abcd1234:/", req.TargetPath, "efs", gomock.Any()).DoAndReturn(func(_, _, _ string, _ []string) error {
		if _, err := d.NodeUnpublishVolume(ctx, unpublish); status.Code(err) != codes.Aborted {
			t.Fatalf("NodeUnpublishVolume during mount returned %v, want Aborted", err)
		}
		return errors.New("STS denied the workload role")
	})
	if _, err := d.NodePublishVolume(ctx, req); status.Code(err) != codes.Internal {
		t.Fatalf("NodePublishVolume returned %v, want failed mount", err)
	}
	if _, err := os.Stat(d.workloadTargetDir(req.TargetPath)); !os.IsNotExist(err) {
		t.Fatalf("failed mount left a workload token directory: %v", err)
	}
	mounter.EXPECT().Mount("fs-abcd1234:/", req.TargetPath, "efs", gomock.Any()).Return(nil)
	if _, err := d.NodePublishVolume(ctx, req); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestWorkloadIAMRefreshServiceAccountLookupFailure(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mounter := mocks.NewMockMounter(gomock.NewController(t))
	client := fake.NewSimpleClientset(newWorkloadTestAccount("workload-sa", "sa-uid", workloadTestRole))
	var lookupErr error
	client.PrependReactor("get", "serviceaccounts", func(clienttesting.Action) (bool, runtime.Object, error) {
		return lookupErr != nil, nil, lookupErr
	})
	driver := &Driver{
		mounter:          mounter,
		workloadIdentity: &workloadIdentity{client: client.CoreV1(), tokenDir: filepath.Join(root, "tokens")},
		metaDir:          filepath.Join(root, "mounts"),
	}
	req := newWorkloadTestRequest(filepath.Join(root, "target"))
	audience := json.RawMessage(`"sts.amazonaws.com"`)
	expiration := time.Now().Add(time.Hour)
	original := setWorkloadTestToken(t, req, expiration, audience, "sa-uid")
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("", 0, nil)
	mounter.EXPECT().MakeDir(req.TargetPath).Return(nil)
	mounter.EXPECT().Mount("fs-abcd1234:/", req.TargetPath, "efs", gomock.Any()).Return(nil)
	if _, err := driver.NodePublishVolume(ctx, req); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(driver.workloadTargetDir(req.TargetPath), "*.jwt"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("want one token file, got %v, %v", paths, err)
	}

	refreshed := setWorkloadTestToken(t, req, expiration.Add(time.Hour), audience, "sa-uid")
	resource := schema.GroupResource{Resource: "serviceaccounts"}
	for _, test := range []struct {
		err  error
		code codes.Code
	}{
		{errors.New("connection reset"), codes.Unavailable},
		{apierrors.NewNotFound(resource, "workload-sa"), codes.PermissionDenied},
		{apierrors.NewForbidden(resource, "workload-sa", errors.New("missing RBAC grant")), codes.PermissionDenied},
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
	} {
		lookupErr = test.err
		mounter.EXPECT().GetDeviceName(req.TargetPath).Return("efs", 1, nil)
		if _, err := driver.NodePublishVolume(ctx, req); status.Code(err) != test.code {
			t.Errorf("NodePublishVolume for lookup error %v returned %v, want %s", test.err, err, test.code)
		}
		assertStoredWorkloadToken(t, paths[0], original)
	}

	lookupErr = nil
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("efs", 1, nil)
	if _, err := driver.NodePublishVolume(ctx, req); err != nil {
		t.Fatalf("republish after service account lookup recovered: %v", err)
	}
	assertStoredWorkloadToken(t, paths[0], refreshed)
}

func TestWorkloadIAMDoesNotChangeRegularMounts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	mounter := mocks.NewMockMounter(gomock.NewController(t))
	d := &Driver{
		mounter:          mounter,
		workloadIdentity: &workloadIdentity{tokenDir: filepath.Join(root, "tokens")},
		metaDir:          filepath.Join(root, "mounts"),
	}
	req := newWorkloadTestRequest(filepath.Join(root, "target"))
	req.VolumeCapability.GetMount().MountFlags = []string{"iam"}
	setWorkloadTestToken(t, req, time.Now().Add(time.Hour), json.RawMessage(`"sts.amazonaws.com"`), "sa-uid")
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("", 0, nil)
	mounter.EXPECT().MakeDir(req.TargetPath).Return(nil)
	mounter.EXPECT().Mount("fs-abcd1234:/", req.TargetPath, "efs", []string{"accesspoint=fsap-00000001", "tls", "iam"}).Return(nil)
	if _, err := d.NodePublishVolume(ctx, req); err != nil {
		t.Fatal(err)
	}
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("efs", 1, nil)
	if _, err := d.NodePublishVolume(ctx, req); err != nil {
		t.Fatalf("republish: %v", err)
	}
	if _, err := os.Stat(d.workloadTargetDir(req.TargetPath)); !os.IsNotExist(err) {
		t.Fatalf("regular mount created a workload token directory: %v", err)
	}
	req.VolumeCapability.GetMount().MountFlags = []string{"workloadiam"}
	d.workloadIdentity.client = fake.NewSimpleClientset(newWorkloadTestAccount("workload-sa", "sa-uid", workloadTestRole)).CoreV1()
	mounter.EXPECT().GetDeviceName(req.TargetPath).Return("efs", 1, nil)
	if _, err := d.NodePublishVolume(ctx, req); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("existing mount with different identity returned %v, want FailedPrecondition", err)
	}
}
