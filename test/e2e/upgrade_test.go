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

	ginkgo "github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/kubectl"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
)

const (
	helmReleaseName = "aws-efs-csi-driver"
	helmNamespace   = "kube-system"
)

var _ = ginkgo.Describe("[efs-csi] Driver Upgrade", ginkgo.Ordered, ginkgo.Label("Disruptive", "Serial"), func() {
	f := framework.NewDefaultFramework("efs-upgrade")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	ginkgo.BeforeEach(func() {
		if upgradeOldImageTag == "" {
			ginkgo.Skip("--upgrade-old-image-tag not set, skipping upgrade test")
		}
		if upgradeNewImageTag == "" {
			ginkgo.Skip("--upgrade-new-image-tag not set, skipping upgrade test")
		}
		if FileSystemId == "" {
			ginkgo.Skip("EFS FileSystemId is empty, skipping upgrade test")
		}
	})

	ginkgo.It("should maintain existing mounts and support new mounts after upgrade", func() {
		config := FileSystemTestConfig{
			FSType:           util.FileSystemTypeEFS,
			ProvisioningMode: "efs-ap",
		}
		testData := "upgrade-test-data-" + generateRandomString(8)
		writePath := "/mnt/volume1/upgrade-test.txt"

		// Step 1: Deploy old version of the driver
		ginkgo.By(fmt.Sprintf("Installing old driver version %s/%s via Helm", upgradeOldImageRepo, upgradeOldImageTag))
		helmDeploy(upgradeOldImageRepo, upgradeOldImageTag)
		waitForDriverReady(f)

		// Step 2: Create a pod with an EFS volume and write test data
		ginkgo.By("Creating EFS PVC and PV for pre-upgrade pod")
		pvc, pv, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-pre-upgrade", "/upgrade-test", map[string]string{}, config)
		framework.ExpectNoError(err, "creating pre-upgrade EFS PVC/PV")
		defer func() {
			_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
		}()

		ginkgo.By("Creating pre-upgrade pod and writing test data")
		writeCmd := fmt.Sprintf("echo '%s' > %s && sync", testData, writePath)
		writePod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, writeCmd)
		writePod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), writePod, metav1.CreateOptions{})
		framework.ExpectNoError(err, "creating write pod")
		framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(context.TODO(), f.ClientSet, writePod.Name, f.Namespace.Name), "waiting for write pod success")

		ginkgo.By("Creating long-running pod with EFS volume mounted (survives upgrade)")
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
			newRepo = upgradeOldImageRepo
		}
		ginkgo.By(fmt.Sprintf("Upgrading driver to %s/%s via Helm", newRepo, upgradeNewImageTag))
		helmDeploy(newRepo, upgradeNewImageTag)
		waitForDriverReady(f)

		// Step 4: Verify existing mount still works
		ginkgo.By("Verifying existing mount still works after upgrade (reading test data)")
		output = kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", longRunPod.Name, "--", "/bin/sh", "-c", readCmd)
		output = strings.TrimSpace(output)
		if output != testData {
			ginkgo.Fail(fmt.Sprintf("Post-upgrade read from existing mount failed: expected %q, got %q", testData, output))
		}

		ginkgo.By("Verifying existing mount supports writes after upgrade")
		postUpgradeData := "post-upgrade-" + generateRandomString(8)
		postWritePath := "/mnt/volume1/post-upgrade-test.txt"
		postWriteCmd := fmt.Sprintf("echo '%s' > %s && sync", postUpgradeData, postWritePath)
		kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", longRunPod.Name, "--", "/bin/sh", "-c", postWriteCmd)
		output = kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", longRunPod.Name, "--", "/bin/sh", "-c", fmt.Sprintf("cat %s", postWritePath))
		output = strings.TrimSpace(output)
		if output != postUpgradeData {
			ginkgo.Fail(fmt.Sprintf("Post-upgrade write/read on existing mount failed: expected %q, got %q", postUpgradeData, output))
		}

		// Step 5: Create a new pod with a new EFS volume after upgrade
		ginkgo.By("Creating new EFS PVC and PV after upgrade")
		pvcNew, pvNew, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-post-upgrade", "/upgrade-test-new", map[string]string{}, config)
		framework.ExpectNoError(err, "creating post-upgrade EFS PVC/PV")
		defer func() {
			_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pvNew.Name, metav1.DeleteOptions{})
		}()

		ginkgo.By("Creating post-upgrade pod and verifying read/write on new mount")
		newTestData := "new-mount-data-" + generateRandomString(8)
		newWritePath := "/mnt/volume1/new-mount-test.txt"
		newWriteCmd := fmt.Sprintf("echo '%s' > %s && sync", newTestData, newWritePath)
		newPod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcNew}, admissionapi.LevelBaseline, newWriteCmd)
		newPod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), newPod, metav1.CreateOptions{})
		framework.ExpectNoError(err, "creating post-upgrade write pod")
		framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(context.TODO(), f.ClientSet, newPod.Name, f.Namespace.Name), "waiting for post-upgrade write pod success")

		ginkgo.By("Verifying data written by post-upgrade pod")
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

		// Cleanup: uninstall the driver (restore to pre-test state)
		ginkgo.By("Cleaning up: uninstalling driver")
		helmUninstall()
	})
})

// helmDeploy installs or upgrades the EFS CSI driver via Helm with the specified image.
func helmDeploy(imageRepo, imageTag string) {
	args := []string{
		"upgrade", "--install", helmReleaseName,
		"--namespace", helmNamespace,
		"--set", fmt.Sprintf("image.repository=%s", imageRepo),
		"--set", fmt.Sprintf("image.tag=%s", imageTag),
		"--wait",
		"--timeout", "5m",
		upgradeHelmChartDir,
	}
	runHelm(args...)
}

// helmUninstall removes the EFS CSI driver Helm release.
func helmUninstall() {
	args := []string{
		"uninstall", helmReleaseName,
		"--namespace", helmNamespace,
	}
	runHelm(args...)
}

// runHelm executes a helm command and fails the test on error.
func runHelm(args ...string) {
	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	framework.Logf("helm %s: %s", strings.Join(args[:2], " "), string(output))
	if err != nil {
		framework.Failf("helm command failed: %v\nOutput: %s", err, string(output))
	}
}

// waitForDriverReady waits for the EFS CSI driver DaemonSet pods to be ready.
func waitForDriverReady(f *framework.Framework) {
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		ds, err := f.ClientSet.AppsV1().DaemonSets(helmNamespace).Get(ctx, "efs-csi-node", metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return ds.Status.NumberReady > 0 && ds.Status.NumberReady == ds.Status.DesiredNumberScheduled, nil
	})
	framework.ExpectNoError(err, "waiting for efs-csi-node DaemonSet to be ready")
}
