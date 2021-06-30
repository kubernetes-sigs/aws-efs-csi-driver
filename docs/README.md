[![Build Status](https://travis-ci.org/kubernetes-sigs/aws-efs-csi-driver.svg?branch=master)](https://travis-ci.org/kubernetes-sigs/aws-efs-csi-driver)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-sigs/aws-efs-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-sigs/aws-efs-csi-driver?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes-sigs/aws-efs-csi-driver)](https://goreportcard.com/report/github.com/kubernetes-sigs/aws-efs-csi-driver)

## Amazon EFS CSI Driver

The [Amazon Elastic File System](https://aws.amazon.com/efs/) Container Storage Interface (CSI) Driver implements the [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) specification for container orchestrators to manage the lifecycle of Amazon EFS file systems.

### CSI Specification Compatibility Matrix
| AWS EFS CSI Driver \ CSI Spec Version  | v0.3.0| v1.1.0 | v1.2.0 |
|----------------------------------------|-------|--------|--------|
| master branch                          | no    | no     | yes    |
| v1.x.x                                 | no    | no     | yes    |
| v0.3.0                                 | no    | yes    | no     |
| v0.2.0                                 | no    | yes    | no     |
| v0.1.0                                 | yes   | no     | no     |

## Features
EFS CSI driver supports dynamic provisioning and static provisioning.
Currently Dynamic Provisioning creates an access point for each PV. This mean an AWS EFS file system has to be created manually on AWS first and should be provided as an input to the storage class parameter.
For static provisioning, AWS EFS file system needs to be created manually on AWS first. After that it can be mounted inside a container as a volume using the driver.

The following CSI interfaces are implemented:
* Controller Service: CreateVolume, DeleteVolume, ControllerGetCapabilities, ValidateVolumeCapabilities
* Node Service: NodePublishVolume, NodeUnpublishVolume, NodeGetCapabilities, NodeGetInfo, NodeGetId, NodeGetVolumeStats
* Identity Service: GetPluginInfo, GetPluginCapabilities, Probe

### Storage Class Parameters for Dynamic Provisioning
| Parameters          | Values | Default | Optional  | Description |
|---------------------|--------|---------|-----------|-------------|
| provisioningMode    | efs-ap |         | false     | Type of volume provisioned by efs. Currently, Access Points are supported. |
| fileSystemId        |        |         | false     | File System under which access points are created. | 
| directoryPerms      |        |         | false     | Directory permissions for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation. |
| gidRangeStart       |        | 50000   | true      | Start range of the POSIX group Id to be applied for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation. |
| gidRangeEnd         |        | 7000000 | true      | End range of the POSIX group Id. |
| basePath            |        |         | true      | Path under which access points for dynamic provisioning is created. If this parameter is not specified, access points are created under the root directory of the file system |
| az                  |        |   ""    | true      | Used for cross-account mount. `az` under storage class parameter is optional. If specified, mount target associated with the az will be used for cross-account mount. If not specified, a random mount target will be picked for cross account mount |

**Notes**:
* Custom Posix group Id range for Access Point root directory must include both `gidRangeStart` and `gidRangeEnd` parameters. These parameters are optional only if both are omitted. If you specify one, the other becomes mandatory.
* When using a custom Posix group ID range, there is a possibility for the driver to run out of available POSIX group Ids. We suggest ensuring custom group ID range is large enough or create a new storage class with a new file system to provision additional volumes. 
* `az` under storage class parameter is not be confused with efs-utils mount option `az`. The `az` mount option is used for cross-az mount or efs one zone file system mount within the same aws account as the cluster.

### Encryption In Transit
One of the advantages of using EFS is that it provides [encryption in transit](https://aws.amazon.com/blogs/aws/new-encryption-of-data-in-transit-for-amazon-efs/) support using TLS. Using encryption in transit, data will be encrypted during its transition over the network to the EFS service. This provides an extra layer of defence-in-depth for applications that requires strict security compliance.

Encryption in transit is enabled by default in the master branch version of the driver. To disable it and mount volumes using plain NFSv4, set `volumeAttributes` field `encryptInTransit` to `"false"` in your persistent volume manifest. For an example manifest, see [Encryption in Transit Example](../examples/kubernetes/encryption_in_transit/specs/pv.yaml).

**Note** Kubernetes version 1.13+ is required if you are using this feature in Kubernetes.

## EFS CSI Driver on Kubernetes
The following sections are Kubernetes specific. If you are a Kubernetes user, use this for driver features, installation steps and examples.

### Kubernetes Version Compability Matrix
| AWS EFS CSI Driver \ Kubernetes Version| maturity | v1.11 | v1.12 | v1.13 | v1.14 | v1.15 | v1.16 | v1.17+ |
|----------------------------------------|----------|-------|-------|-------|-------|-------|-------|-------|
| master branch                          | GA       | no    | no    | no    | no    | no    | no    | yes   |
| v1.3.x                                 | GA       | no    | no    | no    | no    | no    | no    | yes   |
| v1.2.x                                 | GA       | no    | no    | no    | no    | no    | no    | yes   |
| v1.1.x                                 | GA       | no    | no    | no    | yes   | yes   | yes   | yes   |
| v1.0.x                                 | GA       | no    | no    | no    | yes   | yes   | yes   | yes   |
| v0.3.0                                 | beta     | no    | no    | no    | yes   | yes   | yes   | yes   |
| v0.2.0                                 | beta     | no    | no    | no    | yes   | yes   | yes   | yes   |
| v0.1.0                                 | alpha    | yes   | yes   | yes   | no    | no    | no    | no    |

### Container Images
|EFS CSI Driver Version     | Image                               |
|---------------------------|-------------------------------------|
|master branch              |amazon/aws-efs-csi-driver:master     |
|v1.3.2                     |amazon/aws-efs-csi-driver:v1.3.2     |
|v1.3.1                     |amazon/aws-efs-csi-driver:v1.3.1     |
|v1.3.0                     |amazon/aws-efs-csi-driver:v1.3.0     |
|v1.2.1                     |amazon/aws-efs-csi-driver:v1.2.1     |
|v1.2.0                     |amazon/aws-efs-csi-driver:v1.2.0     |
|v1.1.1                     |amazon/aws-efs-csi-driver:v1.1.1     |
|v1.1.0                     |amazon/aws-efs-csi-driver:v1.1.0     |
|v1.0.0                     |amazon/aws-efs-csi-driver:v1.0.0     |
|v0.3.0                     |amazon/aws-efs-csi-driver:v0.3.0     |
|v0.2.0                     |amazon/aws-efs-csi-driver:v0.2.0     |
|v0.1.0                     |amazon/aws-efs-csi-driver:v0.1.0     |

### Features
* Static provisioning - EFS file system needs to be created manually first, then it could be mounted inside container as a persistent volume (PV) using the driver.
* Dynamic provisioning - Uses a persistent volume claim (PVC) to dynamically provision a persistent volume (PV). On Creating a PVC, kuberenetes requests EFS to create an Access Point in a file system which will be used to mount the PV.
* Mount Options - Mount options can be specified in the persistent volume (PV) or storage class for dynamic provisioning to define how the volume should be mounted.
* Encryption of data in transit - EFS file systems are mounted with encryption in transit enabled by default in the master branch version of the driver.
* Cross account mount - EFS file systems from different aws accounts can be mounted from an EKS cluster.
* Multiarch - EFS CSI driver image is now multiarch on ECR

**Notes**:
* Since EFS is an elastic file system it doesn't really enforce any file system capacity. The actual storage capacity value in persistent volume and persistent volume claim is not used when creating the file system. However, since the storage capacity is a required field by Kubernetes, you must specify the value and you can use any valid value for the capacity.

* If you are deploying Amazon EFS CSI Driver in on-premise environment, please refer to the doc [ON-PREMISE.md](./ON-PREMISE.md). 

### Installation
#### Set up driver permission:
The driver requires IAM permission to talk to Amazon EFS to manage the volume on user's behalf. There are several methods to grant driver IAM permission:
* Using IAM Role for Service Account (Recommended if you're using EKS): create an [IAM Role for service accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) with the [required permissions](./iam-policy-example.json). Uncomment annotations and put the IAM role ARN in [service-account manifest](../deploy/kubernetes/base/serviceaccount-csi-controller.yaml)
* Using IAM [instance profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) - grant all the worker nodes with [required permissions](./iam-policy-example.json) by attaching policy to the instance profile of the worker.

#### Deploy the driver:

If you want to deploy the stable driver:
```sh
kubectl apply -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=release-1.3"
```

If you want to deploy the development driver:
```sh
kubectl apply -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/dev/?ref=master"
```

Alternatively, you could also install the driver using helm:
```sh
helm repo add aws-efs-csi-driver https://kubernetes-sigs.github.io/aws-efs-csi-driver/
helm repo update
helm upgrade --install aws-efs-csi-driver --namespace kube-system aws-efs-csi-driver/aws-efs-csi-driver
```

### Examples
Before the example, you need to:
* Get yourself familiar with how to setup Kubernetes on AWS and how to [create EFS file system](https://docs.aws.amazon.com/efs/latest/ug/getting-started.html).
* When creating EFS file system, make sure it is accessible from Kubernetes cluster. This can be achieved by creating the file system inside the same VPC as Kubernetes cluster or using VPC peering.
* Install EFS CSI driver following the [Installation](README.md#Installation) steps.

#### Example links
* [Static provisioning](../examples/kubernetes/static_provisioning/README.md)
* [Dynamic provisioning](../examples/kubernetes/dynamic_provisioning/README.md)
* [Encryption in transit](../examples/kubernetes/encryption_in_transit/README.md)
* [Accessing the file system from multiple pods](../examples/kubernetes/multiple_pods/README.md)
* [Consume EFS in StatefulSets](../examples/kubernetes/statefulset/README.md)
* [Mount subpath](../examples/kubernetes/volume_path/README.md)
* [Use Access Points](../examples/kubernetes/access_points/README.md)

## Development
Please go through [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and [Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs) to get some basic understanding of CSI driver before you start.

### Requirements
* Golang 1.13.4+

### Dependency
Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, to build the project run: `make`

### Testing
To execute all unit tests, run: `make test`

## License
This library is licensed under the Apache 2.0 License.
