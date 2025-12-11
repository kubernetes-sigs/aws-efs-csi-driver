# Amazon EFS CSI Driver
[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/kubernetes-sigs/aws-efs-csi-driver?filter=!*chart*)](https://github.com/kubernetes-sigs/aws-efs-csi-driver/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes-sigs/aws-efs-csi-driver)](https://goreportcard.com/report/github.com/kubernetes-sigs/aws-efs-csi-driver)


## Overview
The [Amazon Elastic File System](https://aws.amazon.com/efs/) Container Storage Interface (CSI) Driver implements the [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) specification for container orchestrators to manage the lifecycle of Amazon EFS file systems.

> **Note:** The latest release version may not include the most recent code changes in master branch. Please check the [changelog](../CHANGELOG-2.x.md) for updates included in the corresponding release versions.

## Features
* Static provisioning - Amazon EFS file system needs to be created manually first, then it could be mounted inside container as a persistent volume (PV) using the driver.
* Dynamic provisioning - Uses a persistent volume claim (PVC) to dynamically provision a persistent volume (PV). On Creating a PVC, Kubernetes requests Amazon EFS to create an Access Point in a file system which will be used to mount the PV.
* Mount Options - Mount options can be specified in the persistent volume (PV) or storage class for dynamic provisioning to define how the volume should be mounted.
* Encryption of data in transit - Amazon EFS file systems are mounted with encryption in transit enabled by default in the master branch version of the driver.
* Cross account mount - Amazon EFS file systems from different aws accounts can be mounted from an Amazon EKS cluster.
* Multiarch - Amazon EFS CSI driver image is now multiarch on ECR

Currently, Dynamic Provisioning creates an access point for each PV. This mean an Amazon EFS file system has to be created manually on AWS first and should be provided as an input to the storage class parameter.
For static provisioning, the Amazon EFS file system needs to be created manually on AWS first. After that, it can be mounted inside a container as a volume using the driver.

The following CSI interfaces are implemented:
* Controller Service: CreateVolume, DeleteVolume, ControllerGetCapabilities, ValidateVolumeCapabilities
* Node Service: NodePublishVolume, NodeUnpublishVolume, NodeGetCapabilities, NodeGetInfo, NodeGetId, NodeGetVolumeStats
* Identity Service: GetPluginInfo, GetPluginCapabilities, Probe

**Note**  
Since Amazon EFS is an elastic file system, it doesn't really enforce any file system capacity. The actual storage capacity value in persistent volume and persistent volume claim is not used when creating the file system. However, since the storage capacity is a required field by Kubernetes, you must specify the value and you can use any valid value for the capacity.

For detailed parameter explanations, see the [parameters documentation](parameters.md).

## Releases
### ECR Image
| Driver Version | [ECR](https://gallery.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver) Image |
|----------------|-------------------------------------------------------------------------------|
| v2.2.0         | public.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver:v2.2.0                |

**Note**  
You can find previous efs-csi-driver versions' images from [here](https://gallery.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver)

## Documentation
- [Parameters/Features](parameters.md)
- [Compatibitlity Matrix](compatibility.md)
- [Driver Installation](install.md)
- [Driver Upgrade](install.md#upgrading-the-amazon-efs-csi-driver)
- [Driver Uninstallation](install.md#uninstalling-the-amazon-efs-csi-driver)
- [Kubernetes Examples](../examples/kubernetes)
- [Frequently Asked Questions](faq.md)
- [Development and Contributing](CONTRIBUTING.md)