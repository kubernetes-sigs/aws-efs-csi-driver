/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License").
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
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/kubectl"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"
)

func testWorkloadIAM(ctx context.Context, f *framework.Framework) {
	if workloadIAMAllowedRoleARN == "" || workloadIAMDeniedRoleARN == "" {
		ginkgo.Skip("both workload IAM role ARN flags are required")
	}
	if FileSystemId == "" {
		ginkgo.Fail("set --file-system-id to the EFS file system that grants the allowed role access")
	}

	driver, err := f.ClientSet.StorageV1().CSIDrivers().Get(ctx, "efs.csi.aws.com", metav1.GetOptions{})
	framework.ExpectNoError(err, "getting the EFS CSI driver")
	var tokenLifetime time.Duration
	for _, request := range driver.Spec.TokenRequests {
		if request.Audience == "sts.amazonaws.com" && request.ExpirationSeconds != nil {
			tokenLifetime = time.Duration(*request.ExpirationSeconds) * time.Second
		}
	}
	if tokenLifetime == 0 || driver.Spec.RequiresRepublish == nil || !*driver.Spec.RequiresRepublish {
		ginkgo.Fail("enable workload IAM on the installed Helm chart before running this test")
	}

	ginkgo.By("Creating a volume with workload IAM authentication")
	ns := f.Namespace.Name
	config := FileSystemTestConfig{FSType: util.FileSystemTypeEFS}
	pvc, pv := makeEFSPVCPV(ns, ns+"-workload-iam", "/", map[string]string{}, config)
	pv.Spec.MountOptions = append(pv.Spec.MountOptions, "workloadiam")
	pvs := f.ClientSet.CoreV1().PersistentVolumes()
	pv, err = pvs.Create(ctx, pv, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating the EFS persistent volume")
	ginkgo.DeferCleanup(func(ctx context.Context) error {
		return pvs.Delete(ctx, pv.Name, metav1.DeleteOptions{})
	})
	pvcs := f.ClientSet.CoreV1().PersistentVolumeClaims(ns)
	pvc, err = pvcs.Create(ctx, pvc, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating the EFS persistent volume claim")
	ginkgo.DeferCleanup(func(ctx context.Context) error {
		return pvcs.Delete(ctx, pvc.Name, metav1.DeleteOptions{})
	})

	ginkgo.By("Mounting the volume with the authorized workload role")
	allowed := createWorkloadIAMTestPod(ctx, f, pvc, "workload-iam-allowed", workloadIAMAllowedRoleARN, "")
	framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(ctx, f.ClientSet, allowed.Name, ns), "waiting for the authorized pod to mount EFS")
	allowed, err = f.ClientSet.CoreV1().Pods(ns).Get(ctx, allowed.Name, metav1.GetOptions{})
	framework.ExpectNoError(err, "getting the authorized pod's node")
	checkMount := func() {
		_, err := kubectl.NewKubectlCommand(ns, "exec", allowed.Name, "--", "/bin/sh", "-c", "ls -A /mnt/volume1 >/dev/null").
			WithTimeout(time.After(30 * time.Second)).Exec()
		framework.ExpectNoError(err, "reading the EFS volume from the authorized pod")
	}
	checkMount()

	// Observe timestamps on the node plugin so the test never reads a token.
	ginkgo.By("Recording the initial token file timestamp")
	driverPod, tokenDir := workloadIAMTokenDir(ctx, f, allowed, pv.Name)
	tokenTimestamp := func() (int64, error) {
		output, err := kubectl.NewKubectlCommand(EfsDriverNamespace, "exec", driverPod, "-c", "efs-plugin", "--", "/bin/sh", "-c",
			"stat -c %Y "+tokenDir+"/*.jwt").WithTimeout(time.After(30 * time.Second)).Exec()
		if err != nil {
			return 0, err
		}
		return strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	}
	initialTimestamp, err := tokenTimestamp()
	framework.ExpectNoError(err, "reading the initial token file timestamp")

	ginkgo.By("Verifying that EFS denies a second workload on the same node")
	denied := createWorkloadIAMTestPod(ctx, f, pvc, "workload-iam-denied", workloadIAMDeniedRoleARN, allowed.Spec.NodeName)
	eventSelector := fields.Set{"involvedObject.uid": string(denied.UID), "reason": "FailedMount"}.AsSelector()
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		events, err := f.ClientSet.CoreV1().Events(ns).List(ctx, metav1.ListOptions{FieldSelector: eventSelector.String()})
		if err != nil {
			return false, err
		}
		for _, event := range events.Items {
			if strings.Contains(event.Message, "access denied by server while mounting") {
				return true, nil
			}
		}
		return false, nil
	})
	framework.ExpectNoError(err, "waiting for an EFS mount denial for the second workload role")

	ginkgo.By("Waiting for kubelet to refresh the token and checking that the authorized mount still works")
	err = wait.PollUntilContextTimeout(ctx, 10*time.Second, tokenLifetime+2*time.Minute, true, func(context.Context) (bool, error) {
		timestamp, err := tokenTimestamp()
		return timestamp > initialTimestamp, err
	})
	framework.ExpectNoError(err, "waiting for the node plugin to replace the token")
	checkMount()
}

func createWorkloadIAMTestPod(ctx context.Context, f *framework.Framework, pvc *v1.PersistentVolumeClaim, name, roleARN, nodeName string) *v1.Pod {
	ns := f.Namespace.Name
	_, err := f.ClientSet.CoreV1().ServiceAccounts(ns).Create(ctx, &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: map[string]string{"eks.amazonaws.com/role-arn": roleARN},
		},
	}, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating the %s service account", name)

	pod := e2epod.MakePod(ns, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, "")
	pod.Spec.ServiceAccountName = name
	pod.Spec.NodeName = nodeName
	pod, err = f.ClientSet.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	framework.ExpectNoError(err, "creating the pod for %s", name)
	ginkgo.DeferCleanup(e2epod.DeletePodWithWait, f.ClientSet, pod)
	return pod
}

func workloadIAMTokenDir(ctx context.Context, f *framework.Framework, pod *v1.Pod, pvName string) (string, string) {
	driverPods, err := f.ClientSet.CoreV1().Pods(EfsDriverNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: buildLabelSelector(EfsDriverLabelSelectors),
		FieldSelector: "spec.nodeName=" + pod.Spec.NodeName,
	})
	framework.ExpectNoError(err, "listing EFS driver pods on %s", pod.Spec.NodeName)
	for _, driverPod := range driverPods.Items {
		for _, container := range driverPod.Spec.Containers {
			if container.Name != "efs-plugin" {
				continue
			}
			for _, volume := range container.VolumeMounts {
				if volume.Name == "kubelet-dir" {
					target := filepath.Join(volume.MountPath, "pods", string(pod.UID), "volumes", "kubernetes.io~csi", pvName, "mount")
					return driverPod.Name, fmt.Sprintf("/var/run/efs/workload-identity/%x", sha256.Sum256([]byte(target)))
				}
			}
		}
	}
	ginkgo.Fail("no EFS driver pod on the authorized workload's node has a kubelet directory mount")
	return "", ""
}
