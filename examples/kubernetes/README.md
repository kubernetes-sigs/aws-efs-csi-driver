# Examples
Before following the examples, you need to:
* Get yourself familiar with how to setup Kubernetes on AWS and how to [create Amazon EFS file system](https://docs.aws.amazon.com/efs/latest/ug/getting-started.html).
* When creating an Amazon EFS file system, make sure it is accessible from the Kubernetes cluster. This can be achieved by creating the file system inside the same VPC as the Kubernetes cluster or using VPC peering.
* Install Amazon EFS CSI driver following the [Installation](README.md#Installation) steps.

## Example links
* [Static provisioning](static_provisioning/README.md)
* [Dynamic provisioning](dynamic_provisioning/README.md)
* [Encryption in transit](encryption_in_transit/README.md)
* [Accessing the file system from multiple pods](multiple_pods/README.md)
* [Consume Amazon EFS in StatefulSets](statefulset/README.md)
* [Mount subpath](volume_path/README.md)
* [Use Access Points](access_points/README.md)