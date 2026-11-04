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

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	ginkgo "github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	// clusterTagKey is the key the EKS Pod Identity tag-based policies check.
	clusterTagKey = "eks:eks-cluster-name"

	clusterTagPolicyName         = "e2e-efs-csi-cluster-tag"
	clusterTagBackfillPolicyName = "e2e-efs-csi-cluster-tag-backfill"

	// docsDir assumes tests are run from test/e2e/, like localHelmChartDir.
	docsDir = "../../docs"

	// iamPropagationDelay is how long to wait after changing a role's policies.
	iamPropagationDelay = 20 * time.Second
)

// The rollout in docs/cluster-tag-isolation.md#existing-clusters, on one cluster:
// disable (managed policy) -> backfill (tag-based and backfill policies alongside
// the managed policy) -> enforce -> managed and backfill policies detached. It
// changes the controller role's policies and the driver's Helm values, so it runs
// serially and restores both at the end.
var _ = ginkgo.Describe("[efs-csi] Cluster tag isolation", ginkgo.Serial, func() {
	f := framework.NewDefaultFramework("efs-cluster-tag")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	ginkgo.BeforeEach(func() {
		if upgradeNewImageTag == "" {
			ginkgo.Skip("--upgrade-new-image-tag not set, skipping cluster tag isolation test")
		}
		if FileSystemId == "" || ClusterName == "" || Region == "" {
			ginkgo.Skip("needs --file-system-id (or a created file system), --cluster-name and --region")
		}
	})

	ginkgo.It("[efs] should backfill existing access points and enforce the cluster tag", func() {
		ctx := context.TODO()
		c := NewCloud(Region)
		imageRepo := upgradeNewImageRepo
		if imageRepo == "" {
			imageRepo = publicEFSDriverRepo
		}

		role, err := c.podIdentityRoleName(ClusterName, helmNamespace, "efs-csi-controller-sa")
		if err != nil {
			ginkgo.Skip(fmt.Sprintf("no EKS Pod Identity role for efs-csi-controller-sa: %v", err))
		}
		managedPolicyArn := fmt.Sprintf("arn:%s:iam::aws:policy/service-role/AmazonEFSCSIDriverPolicy", c.partition())

		ginkgo.DeferCleanup(func() {
			ginkgo.By("Restoring the controller role and the default driver configuration")
			c.restoreControllerRole(role, managedPolicyArn)
			helmDeployFromLocal(f, imageRepo, upgradeNewImageTag)
			waitForDriverReady(f)
		})

		ginkgo.By(fmt.Sprintf("Tagging file system %s with %s=%s", FileSystemId, clusterTagKey, ClusterName))
		_, err = c.efsclient.TagResource(ctx, &efs.TagResourceInput{
			ResourceId: aws.String(FileSystemId),
			Tags:       []efstypes.Tag{{Key: aws.String(clusterTagKey), Value: aws.String(ClusterName)}},
		})
		framework.ExpectNoError(err, "tagging the file system")

		// Phase 1: disable, with the managed policy.
		ginkgo.By("disable: installing the driver without a cluster tag")
		helmDeployFromLocal(f, imageRepo, upgradeNewImageTag)
		waitForDriverReady(f)

		sc := createClusterTagStorageClass(f, map[string]string{
			"provisioningMode": "efs-ap",
			"fileSystemId":     FileSystemId,
			"directoryPerms":   "700",
			"basePath":         "/cluster-tag",
		}, v1.PersistentVolumeReclaimDelete)
		reuseSC := createClusterTagStorageClass(f, map[string]string{
			"provisioningMode": "efs-ap",
			"fileSystemId":     FileSystemId,
			"directoryPerms":   "700",
			"basePath":         "/cluster-tag-reuse",
			"reuseAccessPoint": "true",
		}, v1.PersistentVolumeReclaimRetain)

		legacyPVC, legacyAP := createBoundPVC(f, "legacy", sc)
		framework.ExpectNoError(expectAccessPointTag(c, legacyAP, ""), "legacy access point")

		// An access point that no PersistentVolume references any more, so backfill
		// leaves it untagged and enforce must refuse to reuse it.
		reusePVC, reuseAP := createBoundPVC(f, "reuse", reuseSC)
		ginkgo.DeferCleanup(func() { _ = c.DeleteAccessPoint(reuseAP) })
		deletePVCKeepAccessPoint(f, reusePVC)

		// A reuseAccessPoint volume that still exists during backfill: other clusters
		// may reuse its access point, so backfill must report it, not tag it.
		reuseLiveSC := createClusterTagStorageClass(f, map[string]string{
			"provisioningMode": "efs-ap",
			"fileSystemId":     FileSystemId,
			"directoryPerms":   "700",
			"basePath":         "/cluster-tag-reuse-live",
			"reuseAccessPoint": "true",
		}, v1.PersistentVolumeReclaimDelete)
		reuseLivePVC, reuseLiveAP := createBoundPVC(f, "reuse-live", reuseLiveSC)

		// Phase 2: backfill, with the tag-based and backfill policies alongside the
		// managed policy, so CreateVolume keeps working.
		ginkgo.By("backfill: attaching the tag-based and backfill policies alongside the managed policy")
		framework.ExpectNoError(c.putRolePolicyFile(role, clusterTagPolicyName, docsDir+"/iam-policy-cluster-tag-controller-example.json"), "attaching the tag-based policy")
		framework.ExpectNoError(c.putRolePolicyFile(role, clusterTagBackfillPolicyName, docsDir+"/iam-policy-cluster-tag-controller-backfill-example.json"), "attaching the backfill policy")
		time.Sleep(iamPropagationDelay)

		helmDeployClusterTag(f, imageRepo, "backfill")
		waitForDriverReady(f)
		waitForControllerLog(f, `clusterTagIsolation="backfill"`)

		ginkgo.By(fmt.Sprintf("backfill: waiting for %s to get the cluster tag", legacyAP))
		err = wait.PollUntilContextTimeout(ctx, 10*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			return expectAccessPointTag(c, legacyAP, ClusterName) == nil, nil
		})
		framework.ExpectNoError(err, "waiting for the backfill to tag %s: %v", legacyAP, expectAccessPointTag(c, legacyAP, ClusterName))
		framework.ExpectNoError(expectAccessPointTag(c, reuseAP, ""), "backfill must not tag an access point no PersistentVolume references")
		framework.ExpectNoError(expectAccessPointTag(c, reuseLiveAP, ""), "backfill must not tag an access point other clusters may reuse")

		_, backfillAP := createBoundPVC(f, "during-backfill", sc)
		framework.ExpectNoError(expectAccessPointTag(c, backfillAP, ClusterName), "access point created during backfill")

		// The managed policy still allows deleting the untagged reuse access point.
		framework.ExpectNoError(f.ClientSet.CoreV1().PersistentVolumeClaims(f.Namespace.Name).Delete(ctx, reuseLivePVC.Name, metav1.DeleteOptions{}), "deleting the reuse-live PVC")
		framework.ExpectNoError(waitForAccessPointDeleted(c, reuseLiveAP), "waiting for %s to be deleted", reuseLiveAP)

		// Phase 3: enforce, then detach the managed and backfill policies.
		ginkgo.By("enforce: setting enforce, then detaching the managed and backfill policies")
		helmDeployClusterTag(f, imageRepo, "enforce")
		waitForDriverReady(f)
		waitForControllerLog(f, `clusterTagIsolation="enforce"`)
		framework.ExpectNoError(c.detachRolePolicy(role, managedPolicyArn), "detaching the managed policy")
		framework.ExpectNoError(c.deleteRolePolicy(role, clusterTagBackfillPolicyName), "detaching the backfill policy")
		time.Sleep(iamPropagationDelay)

		_, newAP := createBoundPVC(f, "new", sc)
		framework.ExpectNoError(expectAccessPointTag(c, newAP, ClusterName), "access point created under enforce")

		ginkgo.By(fmt.Sprintf("enforce: re-creating %s must not reuse untagged access point %s", reusePVC.Name, reuseAP))
		_, err = createEFSPVCPVDynamicProvisioning(f.ClientSet, f.Namespace.Name, reusePVC.Name, reuseSC.Name)
		framework.ExpectNoError(err, "re-creating the reuse PVC")
		framework.ExpectNoError(waitForProvisioningFailure(f, reusePVC.Name, "is not tagged"), "enforce refusing the untagged access point")

		ginkgo.By(fmt.Sprintf("enforce: deleting %s deletes backfilled access point %s", legacyPVC.Name, legacyAP))
		framework.ExpectNoError(f.ClientSet.CoreV1().PersistentVolumeClaims(f.Namespace.Name).Delete(ctx, legacyPVC.Name, metav1.DeleteOptions{}), "deleting the legacy PVC")
		framework.ExpectNoError(waitForAccessPointDeleted(c, legacyAP), "waiting for %s to be deleted", legacyAP)

		framework.ExpectNoError(deleteVolumesAndWait(f, FileSystemId), "deleting the test volumes")
	})
})

// helmDeployClusterTag deploys the local chart with this cluster's name, the Pod
// Identity cluster tag key and the given --cluster-tag-isolation state.
func helmDeployClusterTag(f *framework.Framework, imageRepo, isolation string) {
	args := []string{
		"upgrade", "--install", helmReleaseName,
		"--namespace", helmNamespace,
		"--set", fmt.Sprintf("image.repository=%s", imageRepo),
		"--set", fmt.Sprintf("image.tag=%s", upgradeNewImageTag),
		"--set", fmt.Sprintf("controller.clusterName=%s", ClusterName),
		"--set", fmt.Sprintf("controller.clusterTagKey=%s", clusterTagKey),
		"--set", fmt.Sprintf("controller.clusterTagIsolation=%s", isolation),
		"--wait",
		"--timeout", "5m",
		localHelmChartDir,
	}
	if serviceAccountsNotManagedByHelm(f) {
		args = append(args,
			"--set", "controller.serviceAccount.create=false",
			"--set", "node.serviceAccount.create=false",
		)
	}
	runHelm(args...)
}

func createClusterTagStorageClass(f *framework.Framework, params map[string]string, reclaim v1.PersistentVolumeReclaimPolicy) *storagev1.StorageClass {
	sc := GetStorageClass(params)
	sc.ReclaimPolicy = &reclaim
	sc, err := f.ClientSet.StorageV1().StorageClasses().Create(context.TODO(), sc, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating storage class")
	ginkgo.DeferCleanup(func() {
		_ = f.ClientSet.StorageV1().StorageClasses().Delete(context.TODO(), sc.Name, metav1.DeleteOptions{})
	})
	return sc
}

// createBoundPVC creates a PVC, waits for it to bind, and returns it with the ID
// of its access point.
func createBoundPVC(f *framework.Framework, name string, sc *storagev1.StorageClass) (*v1.PersistentVolumeClaim, string) {
	ctx := context.TODO()
	pvc, err := createEFSPVCPVDynamicProvisioning(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-"+name, sc.Name)
	framework.ExpectNoError(err, "creating PVC %s", name)
	var bound *v1.PersistentVolumeClaim
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		bound, err = f.ClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, metav1.GetOptions{})
		return err == nil && bound.Status.Phase == v1.ClaimBound, nil
	})
	framework.ExpectNoError(err, "waiting for PVC %s to bind", pvc.Name)
	pv, err := f.ClientSet.CoreV1().PersistentVolumes().Get(ctx, bound.Spec.VolumeName, metav1.GetOptions{})
	framework.ExpectNoError(err, "getting PV of %s", pvc.Name)
	handle := pv.Spec.CSI.VolumeHandle
	apID := handle[strings.LastIndex(handle, ":")+1:]
	if !strings.HasPrefix(apID, "fsap-") {
		framework.Failf("volume handle %q of %s has no access point", handle, pvc.Name)
	}
	return bound, apID
}

// deletePVCKeepAccessPoint deletes a PVC with a Retain reclaim policy and then
// its PV, which leaves the access point in place.
func deletePVCKeepAccessPoint(f *framework.Framework, pvc *v1.PersistentVolumeClaim) {
	ctx := context.TODO()
	framework.ExpectNoError(f.ClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{}), "deleting PVC %s", pvc.Name)
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pv, err := f.ClientSet.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
		return err == nil && pv.Status.Phase == v1.VolumeReleased, nil
	})
	framework.ExpectNoError(err, "waiting for PV %s to be released", pvc.Spec.VolumeName)
	framework.ExpectNoError(f.ClientSet.CoreV1().PersistentVolumes().Delete(ctx, pvc.Spec.VolumeName, metav1.DeleteOptions{}), "deleting PV %s", pvc.Spec.VolumeName)
}

// expectAccessPointTag checks the access point's cluster tag value; an empty
// want means the access point must have no cluster tag.
func expectAccessPointTag(c *cloud, accessPointID, want string) error {
	out, err := c.efsclient.DescribeAccessPoints(context.TODO(), &efs.DescribeAccessPointsInput{AccessPointId: aws.String(accessPointID)})
	if err != nil {
		return err
	}
	if len(out.AccessPoints) != 1 {
		return fmt.Errorf("expected 1 access point %s, got %d", accessPointID, len(out.AccessPoints))
	}
	got := ""
	for _, t := range out.AccessPoints[0].Tags {
		if aws.ToString(t.Key) == clusterTagKey {
			got = aws.ToString(t.Value)
		}
	}
	if got != want {
		return fmt.Errorf("access point %s has %s=%q, want %q", accessPointID, clusterTagKey, got, want)
	}
	return nil
}

// waitForAccessPointDeleted waits until the access point no longer exists.
func waitForAccessPointDeleted(c *cloud, accessPointID string) error {
	return wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err := c.efsclient.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{AccessPointId: aws.String(accessPointID)})
		var notFound *efstypes.AccessPointNotFound
		return errors.As(err, &notFound), nil
	})
}

// waitForControllerLog waits until every controller pod has logged a line
// containing substr.
func waitForControllerLog(f *framework.Framework, substr string) {
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		pods, err := f.ClientSet.CoreV1().Pods(helmNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app=efs-csi-controller"})
		if err != nil || len(pods.Items) == 0 {
			return false, nil
		}
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				return false, nil
			}
			logs, err := f.ClientSet.CoreV1().Pods(helmNamespace).GetLogs(pod.Name, &v1.PodLogOptions{Container: "efs-plugin"}).DoRaw(ctx)
			if err != nil || !strings.Contains(string(logs), substr) {
				return false, nil
			}
		}
		return true, nil
	})
	framework.ExpectNoError(err, "waiting for every controller pod to log %q", substr)
}

// waitForProvisioningFailure waits for a ProvisioningFailed event on the PVC
// whose message contains substr.
func waitForProvisioningFailure(f *framework.Framework, pvcName, substr string) error {
	var last string
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		events, err := f.ClientSet.CoreV1().Events(f.Namespace.Name).List(ctx, metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + pvcName + ",reason=ProvisioningFailed",
		})
		if err != nil {
			return false, nil
		}
		for _, e := range events.Items {
			last = e.Message
			if strings.Contains(e.Message, substr) {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("no ProvisioningFailed event containing %q on %s (last: %q): %w", substr, pvcName, last, err)
	}
	return nil
}

// podIdentityRoleName returns the name of the IAM role associated with a
// service account through EKS Pod Identity.
func (c *cloud) podIdentityRoleName(clusterName, namespace, serviceAccount string) (string, error) {
	ctx := context.TODO()
	list, err := c.eksclient.ListPodIdentityAssociations(ctx, &eks.ListPodIdentityAssociationsInput{
		ClusterName:    aws.String(clusterName),
		Namespace:      aws.String(namespace),
		ServiceAccount: aws.String(serviceAccount),
	})
	if err != nil {
		return "", err
	}
	if len(list.Associations) == 0 {
		return "", fmt.Errorf("no pod identity association for %s/%s in cluster %s", namespace, serviceAccount, clusterName)
	}
	assoc, err := c.eksclient.DescribePodIdentityAssociation(ctx, &eks.DescribePodIdentityAssociationInput{
		ClusterName:   aws.String(clusterName),
		AssociationId: list.Associations[0].AssociationId,
	})
	if err != nil {
		return "", err
	}
	arn := aws.ToString(assoc.Association.RoleArn)
	return arn[strings.LastIndex(arn, "/")+1:], nil
}

func (c *cloud) partition() string {
	switch {
	case strings.HasPrefix(c.region, "cn-"):
		return "aws-cn"
	case strings.HasPrefix(c.region, "us-gov-"):
		return "aws-us-gov"
	}
	return "aws"
}

func (c *cloud) putRolePolicyFile(role, name, path string) error {
	doc, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = c.iamclient.PutRolePolicy(context.TODO(), &iam.PutRolePolicyInput{
		RoleName:       aws.String(role),
		PolicyName:     aws.String(name),
		PolicyDocument: aws.String(string(doc)),
	})
	return err
}

func (c *cloud) deleteRolePolicy(role, name string) error {
	_, err := c.iamclient.DeleteRolePolicy(context.TODO(), &iam.DeleteRolePolicyInput{
		RoleName:   aws.String(role),
		PolicyName: aws.String(name),
	})
	return err
}

func (c *cloud) detachRolePolicy(role, policyArn string) error {
	_, err := c.iamclient.DetachRolePolicy(context.TODO(), &iam.DetachRolePolicyInput{
		RoleName:  aws.String(role),
		PolicyArn: aws.String(policyArn),
	})
	return err
}

// restoreControllerRole reattaches the managed policy and removes the test's
// inline policies, logging rather than failing so cleanup always finishes.
func (c *cloud) restoreControllerRole(role, managedPolicyArn string) {
	ctx := context.TODO()
	if _, err := c.iamclient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  aws.String(role),
		PolicyArn: aws.String(managedPolicyArn),
	}); err != nil {
		framework.Logf("reattaching %s to %s: %v", managedPolicyArn, role, err)
	}
	for _, name := range []string{clusterTagPolicyName, clusterTagBackfillPolicyName} {
		if err := c.deleteRolePolicy(role, name); err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
			framework.Logf("deleting inline policy %s from %s: %v", name, role, err)
		}
	}
	time.Sleep(iamPropagationDelay)
}
