package cloud

import (
	"errors"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	csiNodeNameKey = "CSI_NODE_NAME"
	nodeName       = "foo"
	instanceId     = "i-1234567890abcdef0"
	nodeRegion     = "us-east-2"
	nodeZone       = "us-east-2a"
)

func TestErrorProducedIfCSI_NODE_NAMENotSet(t *testing.T) {
	clientSet := setupKubernetesClient(t, "", nil)
	k8sMp := kubernetesApiMetadataProvider{api: clientSet}

	_, err := k8sMp.getMetadata()

	if err == nil {
		t.Fatalf("Expected error to be thrown but instead got success")
	}
}

func TestErrorProducedIfNodeIsNotFound(t *testing.T) {
	clientSet := setupKubernetesClient(t, nodeName, nil)
	k8sMp := kubernetesApiMetadataProvider{api: clientSet}

	_, err := k8sMp.getMetadata()

	if k8sErrors.IsNotFound(errors.Unwrap(err)) {
		t.Fatalf("Expected k8s API to throw 404 but did not")
	}
}

func TestErrorProducedIfProviderIDIsEmpty(t *testing.T) {
	clientSet := setupKubernetesClient(t, nodeName, createNode(nodeName, nodeRegion, nodeZone, ""))
	k8sMp := kubernetesApiMetadataProvider{api: clientSet}

	_, err := k8sMp.getMetadata()

	if err == nil {
		t.Fatalf("Expected error to be thrown but instead got success")
	}
}

func TestErrorProducedIfProviderIDDoesNotMatchRegex(t *testing.T) {
	clientSet := setupKubernetesClient(t, nodeName, createNode(nodeName, nodeRegion, nodeZone, "bar-bar-bar"))
	k8sMp := kubernetesApiMetadataProvider{api: clientSet}

	_, err := k8sMp.getMetadata()

	if err == nil {
		t.Fatalf("Expected error to be thrown but instead got success")
	}
}

func TestInstanceIdParsedFromProviderIdCorrectly(t *testing.T) {
	clientSet := setupKubernetesClient(t, nodeName, createDefaultNode())
	k8sMp := kubernetesApiMetadataProvider{api: clientSet}

	metadata, err := k8sMp.getMetadata()

	if err != nil {
		t.Fatalf("Error occurred when parsing instance ID, %v", err)
	}
	if metadata.GetInstanceID() != instanceId {
		t.Fatalf("Instance ID not extracted correctly, expected %s, got %s", instanceId, metadata.GetInstanceID())
	}
}

func TestRegionAndZoneExtractedCorrectlyFromLabels(t *testing.T) {
	clientSet := setupKubernetesClient(t, nodeName, createDefaultNode())
	k8sMp := kubernetesApiMetadataProvider{api: clientSet}

	metadata, err := k8sMp.getMetadata()

	if err != nil {
		t.Fatalf("Error occurred when parsing instance ID %v", err)
	}
	if metadata.GetInstanceID() != instanceId {
		t.Fatalf("Instance ID not extracted correctly, expected %s, got %s", instanceId, metadata.GetInstanceID())
	}
}

func setupKubernetesClient(t *testing.T, csiNodeName string, node *v1.Node) kubernetes.Interface {
	t.Setenv(csiNodeNameKey, csiNodeName)
	if node == nil {
		return fake.NewSimpleClientset()
	}
	return fake.NewSimpleClientset(node)
}

func createNode(nodeName string, nodeRegion string, nodeZone string, providerId string) *v1.Node {
	return &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				"topology.kubernetes.io/region": nodeRegion,
				"topology.kubernetes.io/zone":   nodeZone,
			},
		},
		Spec: v1.NodeSpec{
			ProviderID: providerId,
		},
		Status: v1.NodeStatus{},
	}
}

func createDefaultNode() *v1.Node {
	return createNode(nodeName, nodeRegion, nodeZone, fmt.Sprintf("aws:///%s/%s", nodeZone, instanceId))
}
