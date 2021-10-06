package cloud

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/efs"
)

// Efs abstracts efs client(https://docs.aws.amazon.com/sdk-for-go/api/service/efs/)
type Efs interface {
	CreateAccessPointWithContext(aws.Context, *efs.CreateAccessPointInput, ...request.Option) (*efs.CreateAccessPointOutput, error)
	DeleteAccessPointWithContext(aws.Context, *efs.DeleteAccessPointInput, ...request.Option) (*efs.DeleteAccessPointOutput, error)
	DescribeAccessPointsWithContext(aws.Context, *efs.DescribeAccessPointsInput, ...request.Option) (*efs.DescribeAccessPointsOutput, error)
	DescribeFileSystemsWithContext(aws.Context, *efs.DescribeFileSystemsInput, ...request.Option) (*efs.DescribeFileSystemsOutput, error)
	DescribeMountTargetsWithContext(aws.Context, *efs.DescribeMountTargetsInput, ...request.Option) (*efs.DescribeMountTargetsOutput, error)
}
