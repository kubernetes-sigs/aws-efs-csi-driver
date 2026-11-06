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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/version"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver"
)

const nodeNameExtraKey = "authentication.kubernetes.io/node-name"

// The chart's efs-csi-node ValidatingAdmissionPolicy limits the node service account to removing the agent-not-ready
// taint from its own Node. These specs use a token of that service account bound to a running node pod, like the
// driver's own token.
var _ = ginkgo.Describe("[efs-csi] Node ValidatingAdmissionPolicy", func() {
	f := framework.NewDefaultFramework("efs-node-vap")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs        clientset.Interface
		pod       *v1.Pod
		ownNode   string
		otherNode string
	)

	ginkgo.BeforeEach(func(ctx context.Context) {
		cs = f.ClientSet
		serverVersion, err := cs.Discovery().ServerVersion()
		framework.ExpectNoError(err)
		if version.MustParseGeneric(serverVersion.GitVersion).LessThan(version.MajorMinor(1, 30)) {
			ginkgo.Skip("the policy needs Kubernetes 1.30 or later")
		}

		pod = runningNodePod(ctx, cs, "")
		ownNode = pod.Spec.NodeName

		nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		framework.ExpectNoError(err)
		otherNode = ""
		for _, n := range nodes.Items {
			if n.Name != ownNode {
				otherNode = n.Name
				break
			}
		}
		gomega.Expect(otherNode).NotTo(gomega.BeEmpty(), "the cluster needs a second node")
	})

	ginkgo.It("lets the node service account remove only the agent-not-ready taint from its own node", func(ctx context.Context) {
		expiration := int64(600)
		token, err := cs.CoreV1().ServiceAccounts(pod.Namespace).CreateToken(ctx, pod.Spec.ServiceAccountName, &authenticationv1.TokenRequest{
			Spec: authenticationv1.TokenRequestSpec{
				ExpirationSeconds: &expiration,
				BoundObjectRef:    &authenticationv1.BoundObjectReference{Kind: "Pod", APIVersion: "v1", Name: pod.Name, UID: pod.UID},
			},
		}, metav1.CreateOptions{})
		framework.ExpectNoError(err)
		nodeClient := clientWithToken(f.ClientConfig(), token.Status.Token)

		// The API server reads the node from the token, as the policy expects.
		review, err := nodeClient.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
		framework.ExpectNoError(err)
		gomega.Expect(review.Status.UserInfo.Username).To(gomega.Equal(fmt.Sprintf("system:serviceaccount:%s:%s", pod.Namespace, pod.Spec.ServiceAccountName)))
		gomega.Expect(review.Status.UserInfo.Extra[nodeNameExtraKey]).To(gomega.ConsistOf(authenticationv1.ExtraValue{ownNode}))

		// Taint both nodes briefly, with NoSchedule so running pods stay.
		otherTaint := v1.Taint{Key: "efs-node-vap", Effect: v1.TaintEffectNoSchedule}
		addTaint(ctx, cs, ownNode, v1.Taint{Key: driver.AgentNotReadyNodeTaintKey, Effect: v1.TaintEffectNoSchedule})
		addTaint(ctx, cs, ownNode, otherTaint)
		addTaint(ctx, cs, otherNode, v1.Taint{Key: driver.AgentNotReadyNodeTaintKey, Effect: v1.TaintEffectNoSchedule})
		ginkgo.DeferCleanup(func(ctx context.Context) {
			for _, node := range []string{ownNode, otherNode} {
				for _, key := range []string{driver.AgentNotReadyNodeTaintKey, otherTaint.Key} {
					if hasTaint(ctx, cs, node, key) {
						framework.ExpectNoError(removeTaint(ctx, cs, node, key))
					}
				}
			}
		})

		onlyTaints := "may only change the taints of its own Node object"
		onlyRemove := fmt.Sprintf("may only remove the %s taint", driver.AgentNotReadyNodeTaintKey)

		ginkgo.By("denying the agent-not-ready taint removal on another node")
		err = removeTaint(ctx, nodeClient, otherNode, driver.AgentNotReadyNodeTaintKey)
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring("may only update its own Node object")))
		gomega.Expect(hasTaint(ctx, cs, otherNode, driver.AgentNotReadyNodeTaintKey)).To(gomega.BeTrue())

		ginkgo.By("denying a label change on its own node")
		err = mergePatch(ctx, nodeClient, ownNode, `{"metadata":{"labels":{"efs-node-vap":"denied"}}}`)
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(onlyTaints)))

		ginkgo.By("denying an annotation change on its own node")
		err = mergePatch(ctx, nodeClient, ownNode, `{"metadata":{"annotations":{"efs-node-vap":"denied"}}}`)
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(onlyTaints)))

		ginkgo.By("denying spec.unschedulable on its own node")
		err = mergePatch(ctx, nodeClient, ownNode, `{"spec":{"unschedulable":true}}`)
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(onlyTaints)))

		ginkgo.By("denying another taint on its own node")
		err = patchTaints(ctx, nodeClient, ownNode, func(taints []v1.Taint) []v1.Taint {
			return append(taints, v1.Taint{Key: "efs-node-vap-added", Effect: v1.TaintEffectNoSchedule})
		})
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(onlyRemove)))

		ginkgo.By("denying the removal of another taint from its own node")
		err = removeTaint(ctx, nodeClient, ownNode, otherTaint.Key)
		gomega.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(onlyRemove)))

		ginkgo.By("denying an update of its own node's status")
		_, err = nodeClient.CoreV1().Nodes().Patch(ctx, ownNode, types.MergePatchType,
			[]byte(`{"status":{"capacity":{"efs-node-vap":"1"}}}`), metav1.PatchOptions{}, "status")
		expectDenied(err)

		ginkgo.By("allowing the agent-not-ready taint removal on its own node")
		framework.ExpectNoError(removeTaint(ctx, nodeClient, ownNode, driver.AgentNotReadyNodeTaintKey))
		gomega.Expect(hasTaint(ctx, cs, ownNode, driver.AgentNotReadyNodeTaintKey)).To(gomega.BeFalse())
		gomega.Expect(hasTaint(ctx, cs, ownNode, otherTaint.Key)).To(gomega.BeTrue())
	})

	ginkgo.It("lets the driver remove the agent-not-ready taint from its node on startup", ginkgo.Serial, func(ctx context.Context) {
		// NoSchedule, so that other pods on the node keep running. The node pods tolerate all taints.
		addTaint(ctx, cs, ownNode, v1.Taint{Key: driver.AgentNotReadyNodeTaintKey, Effect: v1.TaintEffectNoSchedule})
		ginkgo.DeferCleanup(func(ctx context.Context) {
			if hasTaint(ctx, cs, ownNode, driver.AgentNotReadyNodeTaintKey) {
				framework.ExpectNoError(removeTaint(ctx, cs, ownNode, driver.AgentNotReadyNodeTaintKey))
			}
		})

		ginkgo.By(fmt.Sprintf("restarting the node pod %s on %s", pod.Name, ownNode))
		framework.ExpectNoError(cs.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}))
		framework.ExpectNoError(e2epod.WaitForPodNotFoundInNamespace(ctx, cs, pod.Name, pod.Namespace, 5*time.Minute))
		newPod := runningNodePod(ctx, cs, ownNode)
		framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(ctx, cs, newPod.Name, newPod.Namespace))

		ginkgo.By("waiting for the driver to remove the agent-not-ready taint")
		gomega.Eventually(ctx, func(ctx context.Context) bool {
			return hasTaint(ctx, cs, ownNode, driver.AgentNotReadyNodeTaintKey)
		}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(gomega.BeFalse())
	})
})

// runningNodePod returns a running EFS CSI node pod, on the given node if it is not empty. It waits for a new pod
// after a restart.
func runningNodePod(ctx context.Context, cs clientset.Interface, node string) *v1.Pod {
	var found *v1.Pod
	gomega.Eventually(ctx, func(ctx context.Context) error {
		pods, err := cs.CoreV1().Pods(EfsDriverNamespace).List(ctx, metav1.ListOptions{LabelSelector: buildLabelSelector(EfsDriverLabelSelectors)})
		if err != nil {
			return err
		}
		for i := range pods.Items {
			p := &pods.Items[i]
			if p.Status.Phase == v1.PodRunning && p.DeletionTimestamp == nil && p.Spec.NodeName != "" && (node == "" || p.Spec.NodeName == node) {
				found = p
				return nil
			}
		}
		return fmt.Errorf("no running EFS CSI node pod in %s on node %q", EfsDriverNamespace, node)
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(gomega.Succeed())
	return found
}

// expectDenied checks that the API server rejected a request, in authorization or admission.
func expectDenied(err error) {
	gomega.Expect(err).To(gomega.HaveOccurred())
	gomega.Expect(apierrors.IsForbidden(err) || apierrors.IsInvalid(err)).To(gomega.BeTrue(), "unexpected error: %v", err)
}

// clientWithToken returns a client for the same API server that authenticates only with the given bearer token.
func clientWithToken(config *rest.Config, token string) clientset.Interface {
	c := rest.AnonymousClientConfig(config)
	c.BearerToken = token
	client, err := clientset.NewForConfig(c)
	framework.ExpectNoError(err)
	return client
}

func mergePatch(ctx context.Context, cs clientset.Interface, node, patch string) error {
	_, err := cs.CoreV1().Nodes().Patch(ctx, node, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func hasTaint(ctx context.Context, cs clientset.Interface, node, key string) bool {
	n, err := cs.CoreV1().Nodes().Get(ctx, node, metav1.GetOptions{})
	framework.ExpectNoError(err)
	for _, t := range n.Spec.Taints {
		if t.Key == key {
			return true
		}
	}
	return false
}

func addTaint(ctx context.Context, cs clientset.Interface, node string, taint v1.Taint) {
	framework.ExpectNoError(patchTaints(ctx, cs, node, func(taints []v1.Taint) []v1.Taint {
		return append(taints, taint)
	}))
}

func removeTaint(ctx context.Context, cs clientset.Interface, node, key string) error {
	return patchTaints(ctx, cs, node, func(taints []v1.Taint) []v1.Taint {
		var keep []v1.Taint
		for _, t := range taints {
			if t.Key != key {
				keep = append(keep, t)
			}
		}
		return keep
	})
}

// patchTaints changes a node's taints with the same JSON patch the driver sends in removeNotReadyTaint: test the
// current taints, then replace them. A node without taints gets an add instead, since the test needs the path. The
// patch also carries the resourceVersion it read, so it never overwrites taints changed concurrently (for example by
// Karpenter); it retries on a conflict instead.
func patchTaints(ctx context.Context, cs clientset.Interface, node string, change func([]v1.Taint) []v1.Taint) error {
	var err error
	for range 5 {
		var n *v1.Node
		n, err = cs.CoreV1().Nodes().Get(ctx, node, metav1.GetOptions{})
		if err != nil {
			return err
		}
		rv := map[string]any{"op": "add", "path": "/metadata/resourceVersion", "value": n.ResourceVersion}
		ops := []map[string]any{rv, {"op": "add", "path": "/spec/taints", "value": change(n.Spec.Taints)}}
		if len(n.Spec.Taints) > 0 {
			ops = []map[string]any{
				rv,
				{"op": "test", "path": "/spec/taints", "value": n.Spec.Taints},
				{"op": "replace", "path": "/spec/taints", "value": change(n.Spec.Taints)},
			}
		}
		patch, merr := json.Marshal(ops)
		if merr != nil {
			return merr
		}
		_, err = cs.CoreV1().Nodes().Patch(ctx, node, types.JSONPatchType, patch, metav1.PatchOptions{})
		// Retry when the node changed in between, for example by the node lifecycle controller.
		if err == nil || !(apierrors.IsConflict(err) || strings.Contains(err.Error(), "testing value")) {
			return err
		}
	}
	return err
}
