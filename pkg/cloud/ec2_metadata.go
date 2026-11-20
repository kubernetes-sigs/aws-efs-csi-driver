package cloud

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"k8s.io/klog/v2"
)

type EC2Metadata interface {
	GetInstanceIdentityDocument(context.Context, *imds.GetInstanceIdentityDocumentInput, ...func(*imds.Options)) (*imds.GetInstanceIdentityDocumentOutput, error)
	GetMetadata(context.Context, *imds.GetMetadataInput, ...func(*imds.Options)) (*imds.GetMetadataOutput, error)
}

type ec2MetadataProvider struct {
	ec2MetadataService EC2Metadata
}

func (e ec2MetadataProvider) getMetadata() (MetadataService, error) {
	doc, err := e.ec2MetadataService.GetInstanceIdentityDocument(context.TODO(), &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		return nil, fmt.Errorf("could not get EC2 instance identity metadata")
	}

	if len(doc.InstanceID) == 0 {
		return nil, fmt.Errorf("could not get valid EC2 instance ID")
	}

	if len(doc.Region) == 0 {
		return nil, fmt.Errorf("could not get valid EC2 region")
	}

	if len(doc.AvailabilityZone) == 0 {
		return nil, fmt.Errorf("could not get valid EC2 availavility zone")
	}

	// The instance identity document does not include the AZ ID, so read it from IMDS directly.
	// It is not fatal if it is missing: callers fall back to the AZ name.
	azId, err := e.getAvailabilityZoneId()
	if err != nil {
		klog.Warningf("could not get EC2 availability zone id: %v", err)
	}

	return &metadata{
		instanceID:         doc.InstanceID,
		region:             doc.Region,
		availabilityZone:   doc.AvailabilityZone,
		availabilityZoneId: azId,
	}, nil
}

func (e ec2MetadataProvider) getAvailabilityZoneId() (string, error) {
	output, err := e.ec2MetadataService.GetMetadata(context.TODO(), &imds.GetMetadataInput{
		Path: "placement/availability-zone-id",
	})
	if err != nil {
		return "", err
	}
	if output == nil || output.Content == nil {
		return "", fmt.Errorf("empty response for placement/availability-zone-id")
	}
	defer output.Content.Close()
	content, err := io.ReadAll(output.Content)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}
