package cloud

import (
	"context"
	"fmt"
	"os"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var DefaultKubernetesAPIClient = func() (kubernetes.Interface, error) {
	// Build the config from the cluster variables
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

type kubernetesApiMetadataProvider struct {
	api kubernetes.Interface
}

func (k kubernetesApiMetadataProvider) getMetadata() (MetadataService, error) {
	nodeName := os.Getenv("CSI_NODE_NAME")
	if nodeName == "" {
		return nil, fmt.Errorf("CSI_NODE_NAME env var not set")
	}

	node, err := k.api.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting Node %v: %v", nodeName, err)
	}

	providerId := node.Spec.ProviderID
	if providerId == "" {
		return nil, fmt.Errorf("node providerID empty, cannot parse")
	}

	re := regexp.MustCompile("i-[a-z0-9]+$")
	instanceID := re.FindString(providerId)
	if instanceID == "" {
		return nil, fmt.Errorf("did not find aws instance ID in node providerID string")
	}

	return &metadata{
		instanceID:       instanceID,
		region:           node.Labels["topology.kubernetes.io/region"],
		availabilityZone: node.Labels["topology.kubernetes.io/zone"],
	}, nil
}
