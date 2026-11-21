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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	workloadIAMMountOption = "workloadiam"
	workloadTokenDir       = efsStateDir + "/workload-identity"
	serviceAccountTokens   = "csi.storage.k8s.io/serviceAccount.tokens"
	serviceAccountName     = "csi.storage.k8s.io/serviceAccount.name"
	podNamespace           = "csi.storage.k8s.io/pod.namespace"
	podName                = "csi.storage.k8s.io/pod.name"
	podUID                 = "csi.storage.k8s.io/pod.uid"
	workloadRoleAnnotation = "eks.amazonaws.com/role-arn"
	stsAudience            = "sts.amazonaws.com"
)

var workloadRoleARN = regexp.MustCompile(`^arn:aws[a-z0-9-]*:iam::[0-9]{12}:role/([!-~]+/)?[A-Za-z0-9_+=,.@-]{1,64}$`)

type workloadIdentity struct {
	client   corev1client.CoreV1Interface
	tokenDir string
}

type serviceAccountToken struct {
	Token      string    `json:"token"`
	Expiration time.Time `json:"expirationTimestamp"`
	accountUID string
	jwtExpiry  time.Time
}

type workloadTokenClaims struct {
	Audience   json.RawMessage `json:"aud"`
	Expiration int64           `json:"exp"`
	Subject    string          `json:"sub"`
	Kubernetes struct {
		Namespace string `json:"namespace"`
		Pod       struct {
			Name string `json:"name"`
			UID  string `json:"uid"`
		} `json:"pod"`
		ServiceAccount struct {
			Name string `json:"name"`
			UID  string `json:"uid"`
		} `json:"serviceaccount"`
	} `json:"kubernetes.io"`
}

func (c workloadTokenClaims) hasSTSAudience() bool {
	var audience string
	if json.Unmarshal(c.Audience, &audience) == nil {
		return audience == stsAudience
	}
	var audiences []string
	return json.Unmarshal(c.Audience, &audiences) == nil && slices.Contains(audiences, stsAudience)
}

// AWS STS verifies the signature and the role trust policy. The claims here
// bind the token to the pod and service account that kubelet identified.
func readWorkloadTokenClaims(token string) (workloadTokenClaims, error) {
	var claims workloadTokenClaims
	if len(token) > 64*1024 {
		return claims, errors.New("invalid JWT")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return claims, errors.New("invalid JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &claims) != nil {
		return claims, errors.New("invalid JWT claims")
	}
	return claims, nil
}

func (w *workloadIdentity) tokenAndRole(ctx context.Context, req *csi.NodePublishVolumeRequest) (serviceAccountToken, string, error) {
	var tokens map[string]serviceAccountToken
	if err := json.Unmarshal([]byte(req.GetSecrets()[serviceAccountTokens]), &tokens); err != nil {
		return serviceAccountToken{}, "", status.Error(codes.Unauthenticated, "Expected workload service account tokens in the CSI secrets field")
	}
	token := tokens[stsAudience]
	claims, err := readWorkloadTokenClaims(token.Token)
	// The API server reads the clock separately for the token and response expirations.
	now := time.Now()
	token.jwtExpiry = time.Unix(claims.Expiration, 0)
	if err != nil || !claims.hasSTSAudience() || !token.Expiration.After(now) || !token.jwtExpiry.After(now) {
		return token, "", status.Error(codes.Unauthenticated, "Expected a valid, unexpired workload token for AWS STS")
	}
	volumeContext := req.GetVolumeContext()
	namespace, account := volumeContext[podNamespace], volumeContext[serviceAccountName]
	if namespace == "" || account == "" || volumeContext[podName] == "" || volumeContext[podUID] == "" ||
		claims.Kubernetes.Namespace != namespace || claims.Kubernetes.Pod.Name != volumeContext[podName] ||
		claims.Kubernetes.Pod.UID != volumeContext[podUID] || claims.Kubernetes.ServiceAccount.Name != account ||
		claims.Subject != "system:serviceaccount:"+namespace+":"+account || claims.Kubernetes.ServiceAccount.UID == "" {
		return token, "", status.Error(codes.Unauthenticated, "Workload token does not match the pod and service account identified by kubelet")
	}
	serviceAccount, err := w.client.ServiceAccounts(namespace).Get(ctx, account, metav1.GetOptions{})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return token, "", status.FromContextError(err).Err()
		case apierrors.IsNotFound(err):
			return token, "", status.Error(codes.PermissionDenied, "Workload service account no longer exists")
		case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
			return token, "", status.Errorf(codes.PermissionDenied, "Could not read the workload service account: %v", err)
		default:
			return token, "", status.Errorf(codes.Unavailable, "Could not look up the workload service account: %v", err)
		}
	}
	if string(serviceAccount.UID) != claims.Kubernetes.ServiceAccount.UID {
		return token, "", status.Error(codes.PermissionDenied, "Workload token belongs to a different service account UID")
	}
	role := serviceAccount.Annotations[workloadRoleAnnotation]
	if !workloadRoleARN.MatchString(role) {
		return token, "", status.Errorf(codes.FailedPrecondition, "Workload service account requires a valid %s annotation", workloadRoleAnnotation)
	}
	token.accountUID = string(serviceAccount.UID)
	return token, role, nil
}

func (d *Driver) workloadTargetDir(target string) string {
	root := workloadTokenDir
	if d.workloadIdentity != nil {
		root = d.workloadIdentity.tokenDir
	}
	return filepath.Join(root, fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(target)))))
}

func (d *Driver) removeWorkloadToken(target string) error {
	targetDir := d.workloadTargetDir(target)
	root, err := os.Lstat(filepath.Dir(targetDir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !root.IsDir() || root.Mode().Perm() != 0700 {
		return status.Error(codes.Internal, "Workload token directory must be a private directory with mode 0700")
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return status.Errorf(codes.Internal, "Could not remove the workload token: %v", err)
	}
	return nil
}

// prepareWorkloadMount updates the token in place when kubelet republishes an
// existing mount. The mount helper's watchdog reads the same path when renewing
// the EFS client certificate.
func (d *Driver) prepareWorkloadMount(ctx context.Context, req *csi.NodePublishVolumeRequest, mountOptions []string, mounted bool) ([]string, error) {
	token, role, err := d.workloadIdentity.tokenAndRole(ctx, req)
	if err != nil {
		return nil, err
	}
	for _, option := range mountOptions {
		key, _, _ := strings.Cut(option, "=")
		if strings.Contains(option, ",") || (key == "iam" && option != "iam") {
			return nil, status.Error(codes.InvalidArgument, "Workload IAM requires separate mount options without embedded commas")
		}
		switch key {
		case "awscredsuri", "awsprofile", "jwtpath", "rolearn", "requirewebidentity", "sloppy", "notls":
			return nil, status.Errorf(codes.InvalidArgument, "Mount option %q is incompatible with workload IAM", key)
		}
	}

	targetDir := d.workloadTargetDir(req.GetTargetPath())
	volumeContext := req.GetVolumeContext()
	identity := fmt.Sprintf("%q %q %q %q %t %q", req.GetVolumeId(), role, volumeContext[podUID],
		token.accountUID, req.GetReadonly(), mountOptions)
	tokenPath := filepath.Join(targetDir, fmt.Sprintf("%x.jwt", sha256.Sum256([]byte(identity))))
	if mounted {
		previous, err := os.ReadFile(tokenPath)
		if os.IsNotExist(err) {
			return nil, status.Error(codes.FailedPrecondition, "Existing mount does not match the requested workload IAM identity and mount options")
		}
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not read the workload token: %v", err)
		}
		claims, err := readWorkloadTokenClaims(string(previous))
		if err != nil {
			return nil, status.Error(codes.Internal, "Stored workload token is malformed")
		}
		if !token.jwtExpiry.After(time.Unix(claims.Expiration, 0)) {
			return nil, nil
		}
	} else {
		if err := d.removeWorkloadToken(req.GetTargetPath()); err != nil {
			return nil, err
		}
		for _, dir := range []string{d.workloadIdentity.tokenDir, targetDir} {
			if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
				return nil, status.Errorf(codes.Internal, "Could not create a private workload token directory: %v", err)
			}
			info, err := os.Lstat(dir)
			if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
				return nil, status.Error(codes.Internal, "Workload token directory must be a private directory with mode 0700")
			}
		}
	}
	if err := writeWorkloadToken(tokenPath, token.Token); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not save the workload token: %v", err)
	}
	// Escape the role ARN so IAM punctuation can pass through the mount option parser.
	return append(mountOptions, "requirewebidentity="+url.QueryEscape(role), "jwtpath="+tokenPath), nil
}

func writeWorkloadToken(path, token string) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".token-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(token)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
