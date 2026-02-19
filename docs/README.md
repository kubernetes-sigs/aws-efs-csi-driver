# Amazon EFS CSI Driver
[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/kubernetes-sigs/aws-efs-csi-driver?filter=!*chart*)](https://github.com/kubernetes-sigs/aws-efs-csi-driver/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes-sigs/aws-efs-csi-driver)](https://goreportcard.com/report/github.com/kubernetes-sigs/aws-efs-csi-driver)


## Overview

[Amazon Elastic File System](https://docs.aws.amazon.com/efs/latest/ug/whatisefs.html) (Amazon EFS) provides serverless, fully elastic file storage so that you can share file data without provisioning or managing storage capacity and performance.

[S3 Files](https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-files.html) is a shared file system that connects any AWS compute directly with your data in Amazon S3. It provides fast, direct access to all of your S3 data as files with full file system semantics and low-latency performance, without your data ever leaving S3. That means file-based applications, agents, and teams can access and work with S3 data as a file system using the tools they already depend on.

The Amazon EFS Container Storage Interface (CSI) driver allows Kubernetes clusters running on AWS to mount [Amazon EFS](https://aws.amazon.com/efs/) and [Amazon S3 file systems](https://aws.amazon.com/s3/features/files/) starting with version `3.0.0` or above as persistent volumes. The client includes a mount helper program that simplifies mounting Amazon EFS and Amazon S3 file systems and enables Amazon CloudWatch metrics to monitor your file system's mount status.

> **Note:** The latest release version may not include the most recent code changes in master branch. Please check the [changelog](../CHANGELOG-3.x.md) for updates included in the corresponding release versions.

## Features
Amazon EFS CSI driver supports dynamic provisioning and static provisioning.
* Static provisioning - Amazon EFS file system or Amazon S3 file system needs to be created manually first, then it can be mounted inside a container as a persistent volume (PV) using the driver.
* Dynamic provisioning - Uses a persistent volume claim (PVC) to dynamically provision a persistent volume (PV). On creating a PVC, Kubernetes requests an Amazon EFS or Amazon S3 file system to create an access point in a file system which will be used to mount the PV.
* Mount options - Mount options can be specified in the persistent volume (PV) or storage class for dynamic provisioning to define how the volume should be mounted.
* Encryption of data in transit - Amazon EFS and Amazon S3 file systems are mounted with encryption in transit enabled by default in the master branch version of the driver.
* Cross account mount (Amazon EFS only) - Amazon EFS file systems from different AWS accounts can be mounted from an Amazon EKS cluster.
* Multiarch - Amazon EFS CSI driver image is now multiarch on ECR

The following CSI interfaces are implemented:
* Controller Service: CreateVolume, DeleteVolume, ControllerGetCapabilities, ValidateVolumeCapabilities
* Node Service: NodePublishVolume, NodeUnpublishVolume, NodeGetCapabilities, NodeGetInfo, NodeGetId, NodeGetVolumeStats
* Identity Service: GetPluginInfo, GetPluginCapabilities, Probe

**Note**  
Since Amazon EFS and Amazon S3 file systems can scale elastically, it doesn't really enforce any file system capacity. The actual storage capacity value in persistent volume and persistent volume claim is not used when creating the file system. However, since the storage capacity is a required field by Kubernetes, you must specify the value and you can use any valid value for the capacity.

For detailed parameter explanations, see the [parameters documentation](parameters.md).

## Releases
The EFS CSI Driver follows semantic versioning. The version `MAJOR.MINOR.PATCH` will be bumped following the rules below after `v2.2.0`:
- Significant breaking changes will be released as a `MAJOR` update.
- New features will be released as a `MINOR` update.
- Bug or vulnerability fixes will be released as a `PATCH` update.

### ECR Image
| Driver Version | [ECR](https://gallery.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver) Image |
|----------------|-------------------------------------------------------------------------------|
| v2.3.1         | public.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver:v2.3.1                |

**Note**  
You can find previous efs-csi-driver versions' images from [here](https://gallery.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver)

## Documentation
- [Parameters/Features](parameters.md)
- [Compatibility Matrix](compatibility.md)
- [Driver Installation](install.md)
- [Driver Upgrade](install.md#upgrading-the-amazon-efs-csi-driver)
- [Driver Uninstallation](install.md#uninstalling-the-amazon-efs-csi-driver)
- [Kubernetes Examples](../examples/kubernetes)
- [Fips Support](fips.md)
- [Frequently Asked Questions](faq.md)
- [Development and Contributing](CONTRIBUTING.md)