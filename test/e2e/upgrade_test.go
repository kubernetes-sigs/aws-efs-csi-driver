/*
Copyright 2025 The Kubernetes Authors.

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
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
	ginkgo "github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/kubectl"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	helmReleaseName     = "aws-efs-csi-driver"
	helmNamespace       = "kube-system"
	publicEFSDriverRepo = "public.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver"
	// localHelmChartDir assumes tests are run from test/e2e/
	localHelmChartDir = "../../charts/aws-efs-csi-driver"
)

var _ = ginkgo.Describe("[efs-csi] Driver Upgrade", ginkgo.Serial, func() {
	f := framework.NewDefaultFramework("efs-upgrade")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	ginkgo.BeforeEach(func() {
		if upgradeNewImageTag == "" {
			ginkgo.Skip("--upgrade-new-image-tag not set, skipping upgrade test")
		}
	})

	ginkgo.It("[efs] should maintain existing EFS mounts and support new mounts after upgrade", func() {
		if FileSystemId == "" {
			ginkgo.Skip("EFS FileSystemId is empty, skipping EFS upgrade test")
		}
		config := FileSystemTestConfig{
			FSType:           util.FileSystemTypeEFS,
			ProvisioningMode: "efs-ap",
		}
		testUpgrade(f, config, "efs")
	})

	ginkgo.It("[s3files] should maintain existing S3Files mounts and support new mounts after upgrade", func() {
		if S3FilesFileSystemId == "" {
			ginkgo.Skip("S3Files FileSystemId is empty, skipping S3Files upgrade test")
		}
		config := FileSystemTestConfig{
			FSType:           util.FileSystemTypeS3Files,
			ProvisioningMode: "s3files-ap",
		}
		testUpgrade(f, config, "s3files")
	})
})

// testUpgrade runs the full upgrade test flow for a given filesystem type.
func testUpgrade(f *framework.Framework, config FileSystemTestConfig, prefix string) {
	testData := prefix + "-upgrade-data-" + generateRandomString(8)
	writePath := "/mnt/volume1/" + prefix + "-upgrade-test.txt"

	// Step 1: Deploy old version of the driver
	ginkgo.By("Installing latest released driver version via Helm (public chart)")
	helmDeployFromRepo(f)
	waitForDriverReady(f)

	// Step 2: Create a StorageClass and PVC via dynamic provisioning
	ginkgo.By("Creating StorageClass for dynamic provisioning")
	sc := GetStorageClass(map[string]string{
		"provisioningMode": config.ProvisioningMode,
		"fileSystemId":     config.GetFSID(),
		"directoryPerms":   "700",
		"basePath":         "/" + prefix + "-upgrade",
	})
	sc, err := f.ClientSet.StorageV1().StorageClasses().Create(context.TODO(), sc, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating storage class")
	defer func() {
		_ = f.ClientSet.StorageV1().StorageClasses().Delete(context.TODO(), sc.Name, metav1.DeleteOptions{})
	}()

	ginkgo.By("Creating PVC for pre-upgrade pod")
	pvc, err := createEFSPVCPVDynamicProvisioning(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-"+prefix+"-pre", sc.Name)
	framework.ExpectNoError(err, "creating pre-upgrade PVC")

	ginkgo.By("Writing test data to volume")
	writeCmd := fmt.Sprintf("echo '%s' > %s && sync", testData, writePath)
	writePod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, writeCmd)
	writePod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), writePod, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating write pod")
	framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(context.TODO(), f.ClientSet, writePod.Name, f.Namespace.Name), "waiting for write pod success")

	ginkgo.By("Creating long-running pod with volume mounted (survives upgrade)")
	longRunPod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, "while true; do sleep 5; done")
	longRunPod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), longRunPod, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating long-running pod")
	framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(context.TODO(), f.ClientSet, longRunPod.Name, f.Namespace.Name), "waiting for long-running pod")
	defer func() {
		_ = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), longRunPod.Name, metav1.DeleteOptions{})
	}()

	ginkgo.By("Verifying data is readable before upgrade")
	readCmd := fmt.Sprintf("cat %s", writePath)
	output := kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", longRunPod.Name, "--", "/bin/sh", "-c", readCmd)
	output = strings.TrimSpace(output)
	if output != testData {
		ginkgo.Fail(fmt.Sprintf("Pre-upgrade read failed: expected %q, got %q", testData, output))
	}

	// Step 3: Upgrade to new version
	newRepo := upgradeNewImageRepo
	if newRepo == "" {
		newRepo = publicEFSDriverRepo
	}
	ginkgo.By(fmt.Sprintf("Upgrading driver to %s:%s via Helm (local chart)", newRepo, upgradeNewImageTag))
	helmDeployFromLocal(f, newRepo, upgradeNewImageTag)
	waitForDriverReady(f)

	// Step 4: Verify existing mount still works (read and write)
	ginkgo.By("Verifying existing mount still works after upgrade (reading test data)")
	output = kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", longRunPod.Name, "--", "/bin/sh", "-c", readCmd)
	output = strings.TrimSpace(output)
	if output != testData {
		ginkgo.Fail(fmt.Sprintf("Post-upgrade read from existing mount failed: expected %q, got %q", testData, output))
	}

	ginkgo.By("Verifying existing mount supports writes after upgrade")
	postUpgradeData := prefix + "-post-upgrade-" + generateRandomString(8)
	postWritePath := "/mnt/volume1/" + prefix + "-post-upgrade-test.txt"
	postWriteCmd := fmt.Sprintf("echo '%s' > %s && sync", postUpgradeData, postWritePath)
	kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", longRunPod.Name, "--", "/bin/sh", "-c", postWriteCmd)
	output = kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", longRunPod.Name, "--", "/bin/sh", "-c", fmt.Sprintf("cat %s", postWritePath))
	output = strings.TrimSpace(output)
	if output != postUpgradeData {
		ginkgo.Fail(fmt.Sprintf("Post-upgrade write/read on existing mount failed: expected %q, got %q", postUpgradeData, output))
	}

	// Step 5: Create a new PVC after upgrade to verify new provisioning works
	ginkgo.By("Creating new PVC after upgrade")
	pvcNew, err := createEFSPVCPVDynamicProvisioning(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-"+prefix+"-post", sc.Name)
	framework.ExpectNoError(err, "creating post-upgrade PVC")

	ginkgo.By("Verifying read/write on new mount after upgrade")
	newTestData := prefix + "-new-mount-" + generateRandomString(8)
	newWritePath := "/mnt/volume1/" + prefix + "-new-mount-test.txt"
	newWriteCmd := fmt.Sprintf("echo '%s' > %s && sync", newTestData, newWritePath)
	newPod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcNew}, admissionapi.LevelBaseline, newWriteCmd)
	newPod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), newPod, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating post-upgrade write pod")
	framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(context.TODO(), f.ClientSet, newPod.Name, f.Namespace.Name), "waiting for post-upgrade write pod success")

	newReadPod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcNew}, admissionapi.LevelBaseline, "while true; do sleep 5; done")
	newReadPod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), newReadPod, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating post-upgrade read pod")
	framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(context.TODO(), f.ClientSet, newReadPod.Name, f.Namespace.Name), "waiting for post-upgrade read pod")
	defer func() {
		_ = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), newReadPod.Name, metav1.DeleteOptions{})
	}()

	output = kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", newReadPod.Name, "--", "/bin/sh", "-c", fmt.Sprintf("cat %s", newWritePath))
	output = strings.TrimSpace(output)
	if output != newTestData {
		ginkgo.Fail(fmt.Sprintf("Post-upgrade new mount read/write failed: expected %q, got %q", newTestData, output))
	}
}

// helmDeployFromRepo installs the EFS CSI driver from the public Helm chart repository.
func helmDeployFromRepo(f *framework.Framework) {
	runHelm("repo", "add", "--force-update", "aws-efs-csi-driver", "https://kubernetes-sigs.github.io/aws-efs-csi-driver/")
	runHelm("repo", "update")
	args := []string{
		"upgrade", "--install", helmReleaseName,
		"--namespace", helmNamespace,
		"--wait",
		"--timeout", "5m",
		"aws-efs-csi-driver/aws-efs-csi-driver",
	}
	if serviceAccountsNotManagedByHelm(f) {
		args = append(args,
			"--set", "controller.serviceAccount.create=false",
			"--set", "node.serviceAccount.create=false",
		)
	}
	runHelm(args...)
}

// helmDeployFromLocal upgrades the EFS CSI driver using the local Helm chart.
func helmDeployFromLocal(f *framework.Framework, imageRepo, imageTag string) {
	args := []string{
		"upgrade", "--install", helmReleaseName,
		"--namespace", helmNamespace,
		"--set", fmt.Sprintf("image.repository=%s", imageRepo),
		"--set", fmt.Sprintf("image.tag=%s", imageTag),
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

// serviceAccountsNotManagedByHelm returns true if the EFS CSI service accounts exist but are
// NOT managed by Helm (e.g. pre-created by eksctl), meaning Helm should not try to own them.
func serviceAccountsNotManagedByHelm(f *framework.Framework) bool {
	for _, saName := range []string{"efs-csi-controller-sa", "efs-csi-node-sa"} {
		sa, err := f.ClientSet.CoreV1().ServiceAccounts(helmNamespace).Get(context.TODO(), saName, metav1.GetOptions{})
		if err != nil {
			continue
		}
		if sa.Labels["app.kubernetes.io/managed-by"] != "Helm" {
			return true
		}
	}
	return false
}

// runHelm executes a helm command and fails the test on error.
// It passes --kubeconfig from the test framework context, which is required
// when running in CI with a non-default kubeconfig path.
func runHelm(args ...string) {
	if kubeconfig := framework.TestContext.KubeConfig; kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	framework.Logf("helm %s", strings.Join(args, " "))
	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	framework.Logf("helm %s: %s", strings.Join(args[:2], " "), string(output))
	if err != nil {
		framework.Failf("helm command failed: %v\nOutput: %s", err, string(output))
	}
}

// waitForDriverReady waits for the EFS CSI driver DaemonSet and controller Deployment to be ready.
func waitForDriverReady(f *framework.Framework) {
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		ds, err := f.ClientSet.AppsV1().DaemonSets(helmNamespace).Get(ctx, "efs-csi-node", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return ds.Status.NumberReady > 0 && ds.Status.NumberReady == ds.Status.DesiredNumberScheduled, nil
	})
	framework.ExpectNoError(err, "waiting for efs-csi-node DaemonSet to be ready")

	err = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		deploy, err := f.ClientSet.AppsV1().Deployments(helmNamespace).Get(ctx, "efs-csi-controller", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return deploy.Status.ReadyReplicas > 0 && deploy.Status.ReadyReplicas == *deploy.Spec.Replicas, nil
	})
	framework.ExpectNoError(err, "waiting for efs-csi-controller Deployment to be ready")
}
