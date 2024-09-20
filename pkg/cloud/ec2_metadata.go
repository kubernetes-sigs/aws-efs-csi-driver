package cloud

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
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

	return &metadata{
		instanceID:       doc.InstanceID,
		region:           doc.Region,
		availabilityZone: doc.AvailabilityZone,
	}, nil
}
