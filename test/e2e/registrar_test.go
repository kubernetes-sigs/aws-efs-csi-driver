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
	"fmt"
	"strings"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/kubectl"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
)

const (
	efsCSIDriverName      = "efs.csi.aws.com"
	efsNodeDaemonSetName  = "efs-csi-node"
	registrarContainer    = "csi-driver-registrar"
	duplicateNodeDSName   = "efs-csi-node-dup-e2e"
	duplicatePortOffset   = 1000
	reRegistrationTimeout = 3 * time.Minute
	registrationStableFor = 30 * time.Second
	duplicateObserveFor   = 45 * time.Second
)

// node-driver-registrar removes the registration socket at start-up and on SIGTERM
// without checking which instance created it. With two efs-csi-node pods on a node,
// removing one can leave kubelet without the driver; the registrar liveness probe
// must restore it.
var _ = ginkgo.Describe("[efs-csi] Node registration", ginkgo.Serial, func() {
	f := framework.NewDefaultFramework("efs-registration")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	ginkgo.It("should register the driver again after a second efs-csi-node pod on the same node is removed", func() {
		if FileSystemId == "" {
			ginkgo.Skip("EFS FileSystemId is empty, skipping node registration test")
		}
		ctx := context.TODO()
		cs := f.ClientSet

		ginkgo.By("Reading the efs-csi-node DaemonSet")
		ds, err := cs.AppsV1().DaemonSets(EfsDriverNamespace).Get(ctx, efsNodeDaemonSetName, metav1.GetOptions{})
		framework.ExpectNoError(err, "getting DaemonSet %s/%s", EfsDriverNamespace, efsNodeDaemonSetName)
		if !registrarHasHTTPEndpoint(ds) {
			ginkgo.Fail(fmt.Sprintf("container %s of %s/%s has no --http-endpoint argument; the registrar liveness probe is not enabled",
				registrarContainer, EfsDriverNamespace, efsNodeDaemonSetName))
		}

		ginkgo.By("Picking a node with a running efs-csi-node pod")
		nodeName, originalPod := pickNodeWithDriverPod(ctx, cs)
		framework.Logf("Using node %q, efs-csi-node pod %q", nodeName, originalPod)
		framework.ExpectNoError(waitForDriverRegistration(ctx, cs, nodeName, reRegistrationTimeout),
			"driver %s is not registered on node %s before the test", efsCSIDriverName, nodeName)

		defer func() {
			// Remove the copy and restore the registration so later tests are not affected.
			deleteDuplicateDaemonSet(ctx, cs)
			if waitForDriverRegistration(ctx, cs, nodeName, 10*time.Second) != nil {
				framework.Logf("Driver still not registered on %s at cleanup; restarting its efs-csi-node pod", nodeName)
				restartDriverPodOnNode(ctx, cs, nodeName)
				_ = waitForDriverRegistration(ctx, cs, nodeName, reRegistrationTimeout)
			}
		}()

		ginkgo.By("Starting a second efs-csi-node pod on the same node")
		dup := duplicateNodeDaemonSet(ds, nodeName)
		_, err = cs.AppsV1().DaemonSets(EfsDriverNamespace).Create(ctx, dup, metav1.CreateOptions{})
		framework.ExpectNoError(err, "creating DaemonSet %s", duplicateNodeDSName)
		dupPod := waitForDuplicatePodStarted(ctx, cs, nodeName)
		framework.Logf("Duplicate efs-csi-node pod %q started on node %q", dupPod, nodeName)

		ginkgo.By("Observing registration while both pods run")
		// Not asserted: the registration can come and go while both pods run.
		observeEnd := time.Now().Add(duplicateObserveFor)
		for time.Now().Before(observeEnd) {
			framework.Logf("both pods running: driver registered=%v, registrar restarts: original=%d duplicate=%d",
				driverRegistered(ctx, cs, nodeName), registrarRestarts(ctx, cs, originalPod),
				registrarRestarts(ctx, cs, dupPod))
			time.Sleep(5 * time.Second)
		}

		ginkgo.By("Removing the second efs-csi-node pod")
		deleteDuplicateDaemonSet(ctx, cs)

		ginkgo.By("Waiting for the driver to be registered again on the node")
		framework.ExpectNoError(waitForDriverRegistration(ctx, cs, nodeName, reRegistrationTimeout),
			"driver %s was not registered again on node %s within %v after the second pod was removed",
			efsCSIDriverName, nodeName, reRegistrationTimeout)
		ginkgo.By(fmt.Sprintf("Checking that the driver stays registered for %v", registrationStableFor))
		stableErr := wait.PollUntilContextTimeout(ctx, 5*time.Second, registrationStableFor, true, func(ctx context.Context) (bool, error) {
			registered, err := getDriverRegistered(ctx, cs, nodeName)
			if err != nil {
				// A read error is not a lost registration; retry.
				framework.Logf("reading CSINode %s: %v", nodeName, err)
				return false, nil
			}
			framework.Logf("driver registered on %s: %v", nodeName, registered)
			if !registered {
				return false, fmt.Errorf("driver %s dropped from node %s again", efsCSIDriverName, nodeName)
			}
			return false, nil
		})
		if stableErr != nil && !wait.Interrupted(stableErr) {
			framework.Failf("%v", stableErr)
		}

		ginkgo.By("Mounting an EFS volume on the node")
		config := FileSystemTestConfig{FSType: util.FileSystemTypeEFS, ProvisioningMode: "efs-ap"}
		pvc, pv, err := createEFSPVCPV(cs, f.Namespace.Name, f.Namespace.Name, "", map[string]string{}, config)
		framework.ExpectNoError(err, "creating EFS PVC and PV")
		defer func() {
			_ = cs.CoreV1().PersistentVolumes().Delete(ctx, pv.Name, metav1.DeleteOptions{})
		}()
		testFile := e2epod.VolumeMountPath1 + "/registration-" + generateRandomString(8)
		pod := e2epod.MakePod(f.Namespace.Name, map[string]string{"kubernetes.io/hostname": nodeName},
			[]*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline,
			fmt.Sprintf("echo ok > %s && while true; do sleep 5; done", testFile))
		pod, err = cs.CoreV1().Pods(f.Namespace.Name).Create(ctx, pod, metav1.CreateOptions{})
		framework.ExpectNoError(err, "creating pod")
		defer func() {
			_ = cs.CoreV1().Pods(f.Namespace.Name).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		}()
		framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(ctx, cs, pod.Name, f.Namespace.Name),
			"pod using EFS on node %s did not start", nodeName)
		out, err := kubectl.RunKubectl(f.Namespace.Name, "exec", pod.Name, "--", "cat", testFile)
		framework.ExpectNoError(err, "reading %s", testFile)
		if strings.TrimSpace(out) != "ok" {
			framework.Failf("unexpected content in %s: %q", testFile, out)
		}
	})
})

func registrarHasHTTPEndpoint(ds *appsv1.DaemonSet) bool {
	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name != registrarContainer {
			continue
		}
		for _, a := range c.Args {
			if strings.HasPrefix(a, "--http-endpoint=") {
				return true
			}
		}
	}
	return false
}

// duplicateNodeDaemonSet copies the efs-csi-node DaemonSet to run a second pod on
// nodeName with the same hostPaths. Declared ports are hostPorts, so the copy drops the
// registrar port and moves the other ports to be scheduled. The registrar keeps
// --http-endpoint, so it competes with the original for the same port.
func duplicateNodeDaemonSet(ds *appsv1.DaemonSet, nodeName string) *appsv1.DaemonSet {
	labels := map[string]string{"app": duplicateNodeDSName}
	tmpl := ds.Spec.Template.DeepCopy()
	tmpl.ObjectMeta = metav1.ObjectMeta{Labels: labels}
	tmpl.Spec.NodeSelector = map[string]string{"kubernetes.io/hostname": nodeName}
	tmpl.Spec.Affinity = nil

	movedPorts := map[int32]int32{}
	for i := range tmpl.Spec.Containers {
		c := &tmpl.Spec.Containers[i]
		if c.Name == registrarContainer {
			c.Ports = nil
			continue
		}
		for j := range c.Ports {
			p := &c.Ports[j]
			moved := p.ContainerPort + duplicatePortOffset
			movedPorts[p.ContainerPort] = moved
			p.ContainerPort = moved
			p.HostPort = 0
		}
	}
	for i := range tmpl.Spec.Containers {
		c := &tmpl.Spec.Containers[i]
		if c.Name == registrarContainer {
			continue
		}
		for j, a := range c.Args {
			for old, moved := range movedPorts {
				if a == fmt.Sprintf("--health-port=%d", old) {
					c.Args[j] = fmt.Sprintf("--health-port=%d", moved)
				}
			}
		}
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: duplicateNodeDSName, Namespace: EfsDriverNamespace, Labels: labels},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: *tmpl,
		},
	}
}

func pickNodeWithDriverPod(ctx context.Context, cs clientset.Interface) (string, string) {
	pods, err := cs.CoreV1().Pods(EfsDriverNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: buildLabelSelector(EfsDriverLabelSelectors),
	})
	framework.ExpectNoError(err, "listing efs-csi-node pods")
	for _, p := range pods.Items {
		if p.Status.Phase == v1.PodRunning && p.Spec.NodeName != "" && p.DeletionTimestamp == nil {
			return p.Spec.NodeName, p.Name
		}
	}
	framework.Failf("no running efs-csi-node pod found with selector %q", buildLabelSelector(EfsDriverLabelSelectors))
	return "", ""
}

func waitForDuplicatePodStarted(ctx context.Context, cs clientset.Interface, nodeName string) string {
	var name string
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		pods, err := cs.CoreV1().Pods(EfsDriverNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=" + duplicateNodeDSName,
			FieldSelector: "spec.nodeName=" + nodeName,
		})
		if err != nil || len(pods.Items) == 0 {
			return false, nil
		}
		p := pods.Items[0]
		for _, s := range p.Status.ContainerStatuses {
			if s.Name == registrarContainer && (s.State.Running != nil || s.RestartCount > 0 || s.State.Terminated != nil) {
				name = p.Name
				return true, nil
			}
		}
		return false, nil
	})
	framework.ExpectNoError(err, "waiting for the duplicate efs-csi-node pod's registrar to start on node %s", nodeName)
	return name
}

func deleteDuplicateDaemonSet(ctx context.Context, cs clientset.Interface) {
	fg := metav1.DeletePropagationForeground
	err := cs.AppsV1().DaemonSets(EfsDriverNamespace).Delete(ctx, duplicateNodeDSName, metav1.DeleteOptions{PropagationPolicy: &fg})
	if err != nil && !apierrors.IsNotFound(err) {
		framework.Logf("deleting DaemonSet %s: %v", duplicateNodeDSName, err)
	}
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		pods, err := cs.CoreV1().Pods(EfsDriverNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + duplicateNodeDSName})
		if err != nil {
			return false, nil
		}
		_, dsErr := cs.AppsV1().DaemonSets(EfsDriverNamespace).Get(ctx, duplicateNodeDSName, metav1.GetOptions{})
		return len(pods.Items) == 0 && apierrors.IsNotFound(dsErr), nil
	})
	if err != nil {
		framework.Logf("waiting for DaemonSet %s and its pods to be deleted: %v", duplicateNodeDSName, err)
	}
}

// getDriverRegistered reports whether the node's CSINode lists the EFS driver.
func getDriverRegistered(ctx context.Context, cs clientset.Interface, nodeName string) (bool, error) {
	csiNode, err := cs.StorageV1().CSINodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	for _, d := range csiNode.Spec.Drivers {
		if d.Name == efsCSIDriverName {
			return true, nil
		}
	}
	return false, nil
}

func driverRegistered(ctx context.Context, cs clientset.Interface, nodeName string) bool {
	registered, err := getDriverRegistered(ctx, cs, nodeName)
	if err != nil {
		framework.Logf("reading CSINode %s: %v", nodeName, err)
	}
	return registered
}

func waitForDriverRegistration(ctx context.Context, cs clientset.Interface, nodeName string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		return driverRegistered(ctx, cs, nodeName), nil
	})
}

func registrarRestarts(ctx context.Context, cs clientset.Interface, podName string) int32 {
	p, err := cs.CoreV1().Pods(EfsDriverNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return -1
	}
	for _, s := range p.Status.ContainerStatuses {
		if s.Name == registrarContainer {
			return s.RestartCount
		}
	}
	return -1
}

func restartDriverPodOnNode(ctx context.Context, cs clientset.Interface, nodeName string) {
	pods, err := cs.CoreV1().Pods(EfsDriverNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: buildLabelSelector(EfsDriverLabelSelectors),
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		framework.Logf("listing efs-csi-node pods on %s: %v", nodeName, err)
		return
	}
	for _, p := range pods.Items {
		if err := cs.CoreV1().Pods(EfsDriverNamespace).Delete(ctx, p.Name, metav1.DeleteOptions{}); err != nil {
			framework.Logf("deleting pod %s: %v", p.Name, err)
		}
	}
}
