[![Build Status](https://travis-ci.org/kubernetes-sigs/aws-efs-csi-driver.svg?branch=master)](https://travis-ci.org/kubernetes-sigs/aws-efs-csi-driver)
[![Coverage Status](https://coveralls.io/repos/github/kubernetes-sigs/aws-efs-csi-driver/badge.svg?branch=master)](https://coveralls.io/github/kubernetes-sigs/aws-efs-csi-driver?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes-sigs/aws-efs-csi-driver)](https://goreportcard.com/report/github.com/kubernetes-sigs/aws-efs-csi-driver)

## Amazon EFS CSI Driver

The [Amazon Elastic File System](https://aws.amazon.com/efs/) Container Storage Interface (CSI) Driver implements the [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) specification for container orchestrators to manage the lifecycle of Amazon EFS filesystems.

### CSI Specification Compatibility Matrix
| AWS EFS CSI Driver \ CSI Spec Version  | v0.3.0| v1.1.0 | v1.2.0 |
|----------------------------------------|-------|--------|--------|
| master branch                          | no    | no     | yes    |
| v0.3.0                                 | no    | yes    | no     |
| v0.2.0                                 | no    | yes    | no     |
| v0.1.0                                 | yes   | no     | no     |

## Features
Currently only static provisioning is supported. This means an AWS EFS filesystem needs to be created manually on AWS first. After that it can be mounted inside a container as a volume using the driver.

The following CSI interfaces are implemented:
* Node Service: NodePublishVolume, NodeUnpublishVolume, NodeGetCapabilities, NodeGetInfo, NodeGetId
* Identity Service: GetPluginInfo, GetPluginCapabilities, Probe

### Encryption In Transit
One of the advantages of using EFS is that it provides [encryption in transit](https://aws.amazon.com/blogs/aws/new-encryption-of-data-in-transit-for-amazon-efs/) support using TLS. Using encryption in transit, data will be encrypted during its transition over the network to the EFS service. This provides an extra layer of defence-in-depth for applications that requires strict security compliance.

To enable encryption in transit, `tls` needs to be set in the `NodePublishVolumeRequest.VolumeCapability.MountVolume` object's `MountFlags` fields. For an example of using it in kubernetes, see the persistence volume manifest in [Encryption in Transit Example](../examples/kubernetes/encryption_in_transit/specs/pv.yaml)

**Note** Kubernetes version 1.13+ is required if you are using this feature in Kubernetes.

## EFS CSI Driver on Kubernetes
The following sections are Kubernetes specific. If you are a Kubernetes user, use this for driver features, installation steps and examples.

### Kubernetes Version Compability Matrix
| AWS EFS CSI Driver \ Kubernetes Version| maturity | v1.11 | v1.12 | v1.13 | v1.14 | v1.15 |
|----------------------------------------|----------|-------|-------|-------|-------|-------|
| master branch                          | beta     | no    | no    | no    | yes   | yes   |
| v0.3.0                                 | beta     | no    | no    | no    | yes   | yes   |
| v0.2.0                                 | beta     | no    | no    | no    | yes   | yes   |
| v0.1.0                                 | alpha    | yes   | yes   | yes   | no    | no    |

### Container Images
|EFS CSI Driver Version     | Image                               |
|---------------------------|-------------------------------------|
|master branch              |amazon/aws-efs-csi-driver:latest     |
|v0.3.0                     |amazon/aws-efs-csi-driver:v0.3.0     |
|v0.2.0                     |amazon/aws-efs-csi-driver:v0.2.0     |
|v0.1.0                     |amazon/aws-efs-csi-driver:v0.1.0     |

### Features
* Static provisioning - EFS filesystem needs to be created manually first, then it could be mounted inside container as a persistent volume (PV) using the driver.
* Mount Options - Mount options can be specified in the persistence volume (PV) to define how the volume should be mounted. Aside from normal mount options, you can also specify `tls` as a mount option to enable encryption in transit of the EFS filesystem.

**Notes**:
* Since EFS is an elastic filesystem it doesn't really enforce any filesystem capacity. The actual storage capacity value in persistence volume and persistence volume claim is not used when creating the filesystem. However, since the storage capacity is a required field by Kubernetes, you must specify the value and you can use any valid value for the capacity.

### Installation

####Deploy the stable driver

#####From AWS ECR
```sh
kubectl apply -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/k8s/v0.3.0/overlays/ecr"
```

#####Or from Docker Hub
```sh
kubectl apply -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/k8s/v0.3.0/overlays/dockerhub"
```

####Deploy the driver that is currently under development
**WARNING: DO NOT use this version of driver in a PRODUCTION environment since it may contain major bugs which are undetected**
```sh
kubectl apply -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/k8s/dev"
```

####Alternatively, install the driver using helm
```sh
helm repo add aws-efs-csi-driver https://kubernetes-sigs.github.io/aws-efs-csi-driver/
helm install aws-efs-csi-driver aws-efs-csi-driver/aws-efs-csi-driver
```

### Examples
Before the example, you need to:
* Get yourself familiar with how to setup Kubernetes on AWS and how to [create EFS filesystem](https://docs.aws.amazon.com/efs/latest/ug/getting-started.html).
* When creating EFS filesystem, make sure it is accessible from Kuberenetes cluster. This can be achieved by creating the filesystem inside the same VPC as Kubernetes cluster or using VPC peering.
* Install EFS CSI driver following the [Installation](README.md#Installation) steps.

#### Example links
* [Static provisioning](../examples/kubernetes/static_provisioning/README.md)
* [Encryption in transit](../examples/kubernetes/encryption_in_transit/README.md)
* [Accessing the filesystem from multiple pods](../examples/kubernetes/multiple_pods/README.md)
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
