# Examples
Before following the examples, you need to:
* Get yourself familiar with how to setup Kubernetes on AWS and how to [create Amazon EFS file system](../../docs/efs-create-filesystem.md) or [create Amazon S3 file system](../../docs/s3files-create-filesystem.md).
* When creating Amazon S3 Files or Amazon EFS file system, make sure it is accessible from the Kubernetes cluster. This can be achieved by creating the file system inside the same VPC as the Kubernetes cluster or using VPC peering
* Install Amazon EFS CSI driver following the [Installation](../../docs/install.md) steps.

## Example links


| Example | Amazon EFS | Amazon S3 Files |
|---------|------------|-----------------|
| Static provisioning | [Link](efs/static_provisioning/README.md) | [Link](s3files/static_provisioning/README.md) |
| Dynamic provisioning | [Link](efs/dynamic_provisioning/README.md) | [Link](s3files/dynamic_provisioning/README.md) |
| Encryption in transit | [Link](efs/encryption_in_transit/README.md) | N/A |
| Accessing the file system from multiple pods | [Link](efs/multiple_pods/README.md) | [Link](s3files/multiple_pods/README.md) |
| Consume file system in StatefulSets | [Link](efs/statefulset/README.md) | [Link](s3files/statefulset/README.md) |
| Mount subpath | [Link](efs/volume_path/README.md) | [Link](s3files/volume_path/README.md) |
| Use Access Points | [Link](efs/access_points/README.md) | [Link](s3files/access_points/README.md) |
| Cross account mount | [Link](efs/cross_account_mount/README.md) | N/A |
| Availability Zone ID configuration | N/A | [Link](s3files/azid/README.md) |