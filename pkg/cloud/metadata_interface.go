package cloud

import "github.com/aws/aws-sdk-go/aws/ec2metadata"

// MetadataService represents AWS metadata service.
type MetadataService interface {
	GetInstanceID() string
	GetRegion() string
	GetAvailabilityZone() string
}

type EC2Metadata interface {
	Available() bool
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
}

type TaskMetadataService interface {
	GetTMDSV4Response() ([]byte, error)
}
