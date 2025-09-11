[![Go Report Card](https://goreportcard.com/badge/github.com/kubernetes-sigs/aws-efs-csi-driver)](https://goreportcard.com/report/github.com/kubernetes-sigs/aws-efs-csi-driver)

## Amazon EFS CSI Driver

The [Amazon Elastic File System](https://aws.amazon.com/efs/) Container Storage Interface (CSI) Driver implements the [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) specification for container orchestrators to manage the lifecycle of Amazon EFS file systems.

### CSI Specification Compatibility Matrix
| Amazon EFS CSI Driver \ CSI Spec Version | v0.3.0| v1.1.0 | v1.2.0 |
|------------------------------------------|-------|--------|--------|
| master branch                            | no    | no     | yes    |
| v2.x.x                                   | no    | no     | yes    |
| v1.x.x                                   | no    | no     | yes    |
| v0.3.0                                   | no    | yes    | no     |
| v0.2.0                                   | no    | yes    | no     |
| v0.1.0                                   | yes   | no     | no     |

## Features
Amazon EFS CSI driver supports dynamic provisioning and static provisioning.
Currently, Dynamic Provisioning creates an access point for each PV. This mean an Amazon EFS file system has to be created manually on AWS first and should be provided as an input to the storage class parameter.
For static provisioning, the Amazon EFS file system needs to be created manually on AWS first. After that, it can be mounted inside a container as a volume using the driver.

The following CSI interfaces are implemented:
* Controller Service: CreateVolume, DeleteVolume, ControllerGetCapabilities, ValidateVolumeCapabilities
* Node Service: NodePublishVolume, NodeUnpublishVolume, NodeGetCapabilities, NodeGetInfo, NodeGetId, NodeGetVolumeStats
* Identity Service: GetPluginInfo, GetPluginCapabilities, Probe

### Storage Class Parameters for Dynamic Provisioning
| Parameters            | Values | Default         | Optional | Description                                                                                                                                                                                                                                                                                                                                                                                   |
|-----------------------|--------|-----------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| provisioningMode      | efs-ap |                 | false    | Type of volume provisioned by efs. Currently, Access Points are supported.                                                                                                                                                                                                                                                                                                                    |
| fileSystemId          |        |                 | false    | File System under which access points are created.                                                                                                                                                                                                                                                                                                                                            | 
| directoryPerms        |        |                 | false    | Directory permissions for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation.                                                                                                                                                                                                                       |
| uid                   |        |                 | true     | POSIX user Id to be applied for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation.                                                                                                                                                                                                                 |
| gid                   |        |                 | true     | POSIX group Id to be applied for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation.                                                                                                                                                                                                                |
| gidRangeStart         |        | 50000           | true     | Start range of the POSIX group Id to be applied for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation. Not used if uid/gid is set.                                                                                                                                                                 |
| gidRangeEnd           |        | 7000000         | true     | End range of the POSIX group Id. Not used if uid/gid is set.                                                                                                                                                                                                                                                                                                                                  |
| basePath              |        |                 | true     | Path under which access points for dynamic provisioning is created. If this parameter is not specified, access points are created under the root directory of the file system                                                                                                                                                                                                                 |
| subPathPattern        |        | `/${.PV.name}`  | true     | The template used to construct the subPath under which each of the access points created under Dynamic Provisioning. Can be made up of fixed strings and limited variables, is akin to the 'subPathPattern' variable on the [nfs-subdir-external-provisioner](https://github.com/kubernetes-sigs/nfs-subdir-external-provisioner) chart. Supports `.PVC.name`,`.PVC.namespace` and `.PV.name` |
| ensureUniqueDirectory |        | true            | true     | **NOTE: Only set this to false if you're sure this is the behaviour you want**.<br/> Used when dynamic provisioning is enabled, if set to true, appends the a UID to the pattern specified in `subPathPattern` to ensure that access points will not accidentally point at the same directory.                                                                                                |
| az                    |        | ""              | true     | Used for cross-account mount. `az` under storage class parameter is optional. If specified, mount target associated with the az will be used for cross-account mount. If not specified, a random mount target will be picked for cross account mount                                                                                                                                          |
| reuseAccessPoint      |        | false           | true     | When set to true, it creates the Access Point client-token from the provided PVC name. So that the AccessPoint can be replicated from a different cluster if same PVC name and storageclass configuration are used.                                                                                                                                                                                    |

**Note**
* Custom Posix group Id range for Access Point root directory must include both `gidRangeStart` and `gidRangeEnd` parameters. These parameters are optional only if both are omitted. If you specify one, the other becomes mandatory.
* When using a custom Posix group ID range, there is a possibility for the driver to run out of available POSIX group Ids. We suggest ensuring custom group ID range is large enough or create a new storage class with a new file system to provision additional volumes. 
* `az` under storage class parameter is not be confused with efs-utils mount option `az`. The `az` mount option is used for cross-az mount or efs one zone file system mount within the same aws account as the cluster.
* Using dynamic provisioning, [user identity enforcement]((https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-identity-access-points)) is always applied.
 * When user enforcement is enabled, Amazon EFS replaces the NFS client's user and group IDs with the identity configured on the access point for all file system operations.
 * The uid/gid configured on the access point is either the uid/gid specified in the storage class, a value in the gidRangeStart-gidRangeEnd (used as both uid/gid) specified in the storage class, or is a value selected by the driver is no uid/gid or gidRange is specified.
 * We suggest using [static provisioning](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/examples/kubernetes/static_provisioning/README.md) if you do not wish to use user identity enforcement.

If you want to pass any other mountOptions to Amazon EFS CSI driver while mounting, they can be passed in through the Persistent Volume or the Storage Class objects, depending on whether static or dynamic provisioning is used. The following are examples of some mountOptions that can be passed:
* **lookupcache**: Specifies how the kernel manages its cache of directory entries for a given mount point. Mode can be one of all, none, pos, or positive. Each mode has different functions and for more information you can refer to this [link](https://linux.die.net/man/5/nfs).
* **iam**: Use the CSI Node Pod's IAM identity to authenticate with Amazon EFS.

### Default Mount Options
When using the EFS CSI driver, be aware that the `noresvport` mount option is enabled by default. This means the client can use any available source port for communication, not just the reserved ports.

### Encryption In Transit
One of the advantages of using Amazon EFS is that it provides [encryption in transit](https://aws.amazon.com/blogs/aws/new-encryption-of-data-in-transit-for-amazon-efs/) support using TLS. Using encryption in transit, data will be encrypted during its transition over the network to the Amazon EFS service. This provides an extra layer of defence-in-depth for applications that requires strict security compliance.

Encryption in transit is enabled by default in the master branch version of the driver. To disable it and mount volumes using plain NFSv4, set the `volumeAttributes` field `encryptInTransit` to `"false"` in your persistent volume manifest. For an example manifest, see the [encryption in transit example](../examples/kubernetes/encryption_in_transit/specs/pv.yaml).

**Note**  
Kubernetes version 1.13 or later is required if you are using this feature in Kubernetes.

## Amazon EFS CSI Driver on Kubernetes
The following sections are Kubernetes specific. If you are a Kubernetes user, use this for driver features, installation steps, and examples.

### Kubernetes Version Compability Matrix
| Amazon EFS CSI Driver \ Kubernetes Version | maturity | v1.11 | v1.12 | v1.13 | v1.14 | v1.15 | v1.16 | v1.17+ |
|--------------------------------------------|----------|-------|-------|-------|-------|-------|-------|--------|
| master branch                              | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v2.1.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v2.0.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.7.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.6.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.5.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.4.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.3.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.2.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.1.x                                     | GA       | no    | no    | no    | yes   | yes   | yes   | yes    |
| v1.0.x                                     | GA       | no    | no    | no    | yes   | yes   | yes   | yes    |
| v0.3.0                                     | beta     | no    | no    | no    | yes   | yes   | yes   | yes    |
| v0.2.0                                     | beta     | no    | no    | no    | yes   | yes   | yes   | yes    |
| v0.1.0                                     | alpha    | yes   | yes   | yes   | no    | no    | no    | no     |

### Container Images
| Amazon EFS CSI Driver Version | Image                            |
|-------------------------------|----------------------------------|
| master branch                 | amazon/aws-efs-csi-driver:master |
| v2.1.12                       | amazon/aws-efs-csi-driver:v2.1.12 |
| v2.1.11                       | amazon/aws-efs-csi-driver:v2.1.11|
| v2.1.10                       | amazon/aws-efs-csi-driver:v2.1.10|
| v2.1.9                        | amazon/aws-efs-csi-driver:v2.1.9 |
| v2.1.8                        | amazon/aws-efs-csi-driver:v2.1.8 |
| v2.1.7                        | amazon/aws-efs-csi-driver:v2.1.7 |
| v2.1.6                        | amazon/aws-efs-csi-driver:v2.1.6 |
| v2.1.5                        | amazon/aws-efs-csi-driver:v2.1.5 |
| v2.1.4                        | amazon/aws-efs-csi-driver:v2.1.4 |
| v2.1.3                        | amazon/aws-efs-csi-driver:v2.1.3 |
| v2.1.2                        | amazon/aws-efs-csi-driver:v2.1.2 |
| v2.1.1                        | amazon/aws-efs-csi-driver:v2.1.1 |
| v2.1.0                        | amazon/aws-efs-csi-driver:v2.1.0 |
| v2.0.9                        | amazon/aws-efs-csi-driver:v2.0.9 |
| v2.0.8                        | amazon/aws-efs-csi-driver:v2.0.8 |
| v2.0.7                        | amazon/aws-efs-csi-driver:v2.0.7 |
| v2.0.6                        | amazon/aws-efs-csi-driver:v2.0.6 |
| v2.0.5                        | amazon/aws-efs-csi-driver:v2.0.5 |
| v2.0.4                        | amazon/aws-efs-csi-driver:v2.0.4 |
| v2.0.3                        | amazon/aws-efs-csi-driver:v2.0.3 |
| v2.0.2                        | amazon/aws-efs-csi-driver:v2.0.2 |
| v2.0.1                        | amazon/aws-efs-csi-driver:v2.0.1 |
| v2.0.0                        | amazon/aws-efs-csi-driver:v2.0.0 |
| v1.7.7                        | amazon/aws-efs-csi-driver:v1.7.7 |
| v1.7.6                        | amazon/aws-efs-csi-driver:v1.7.6 |
| v1.7.5                        | amazon/aws-efs-csi-driver:v1.7.5 |
| v1.7.4                        | amazon/aws-efs-csi-driver:v1.7.4 |
| v1.7.3                        | amazon/aws-efs-csi-driver:v1.7.3 |
| v1.7.2                        | amazon/aws-efs-csi-driver:v1.7.2 |
| v1.7.1                        | amazon/aws-efs-csi-driver:v1.7.1 |
| v1.7.0                        | amazon/aws-efs-csi-driver:v1.7.0 |
| v1.6.0                        | amazon/aws-efs-csi-driver:v1.6.0 |
| v1.5.9                        | amazon/aws-efs-csi-driver:v1.5.9 |
| v1.5.8                        | amazon/aws-efs-csi-driver:v1.5.8 |
| v1.5.7                        | amazon/aws-efs-csi-driver:v1.5.7 |                                  
| v1.5.6                        | amazon/aws-efs-csi-driver:v1.5.6 |
| v1.5.5                        | amazon/aws-efs-csi-driver:v1.5.5 |
| v1.5.4                        | amazon/aws-efs-csi-driver:v1.5.4 |                                  
| v1.5.3                        | amazon/aws-efs-csi-driver:v1.5.3 |
| v1.5.2                        | amazon/aws-efs-csi-driver:v1.5.2 |
| v1.5.1                        | amazon/aws-efs-csi-driver:v1.5.1 |
| v1.5.0                        | amazon/aws-efs-csi-driver:v1.5.0 |
| v1.4.9                        | amazon/aws-efs-csi-driver:v1.4.9 |
| v1.4.8                        | amazon/aws-efs-csi-driver:v1.4.8 |
| v1.4.7                        | amazon/aws-efs-csi-driver:v1.4.7 |
| v1.4.6                        | amazon/aws-efs-csi-driver:v1.4.6 |
| v1.4.5                        | amazon/aws-efs-csi-driver:v1.4.5 |
| v1.4.4                        | amazon/aws-efs-csi-driver:v1.4.4 |
| v1.4.3                        | amazon/aws-efs-csi-driver:v1.4.3 |
| v1.4.2                        | amazon/aws-efs-csi-driver:v1.4.2 |
| v1.4.1                        | amazon/aws-efs-csi-driver:v1.4.1 |
| v1.4.0                        | amazon/aws-efs-csi-driver:v1.4.0 |
| v1.3.8                        | amazon/aws-efs-csi-driver:v1.3.8 |
| v1.3.7                        | amazon/aws-efs-csi-driver:v1.3.7 |
| v1.3.6                        | amazon/aws-efs-csi-driver:v1.3.6 |
| v1.3.5                        | amazon/aws-efs-csi-driver:v1.3.5 |
| v1.3.4                        | amazon/aws-efs-csi-driver:v1.3.4 |
| v1.3.3                        | amazon/aws-efs-csi-driver:v1.3.3 |
| v1.3.2                        | amazon/aws-efs-csi-driver:v1.3.2 |
| v1.3.1                        | amazon/aws-efs-csi-driver:v1.3.1 |
| v1.3.0                        | amazon/aws-efs-csi-driver:v1.3.0 |
| v1.2.1                        | amazon/aws-efs-csi-driver:v1.2.1 |
| v1.2.0                        | amazon/aws-efs-csi-driver:v1.2.0 |
| v1.1.1                        | amazon/aws-efs-csi-driver:v1.1.1 |
| v1.1.0                        | amazon/aws-efs-csi-driver:v1.1.0 |
| v1.0.0                        | amazon/aws-efs-csi-driver:v1.0.0 |
| v0.3.0                        | amazon/aws-efs-csi-driver:v0.3.0 |
| v0.2.0                        | amazon/aws-efs-csi-driver:v0.2.0 |
| v0.1.0                        | amazon/aws-efs-csi-driver:v0.1.0 |

### ECR Image
| Driver Version | [ECR](https://gallery.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver) Image |
|----------------|-------------------------------------------------------------------------------|
| v2.1.12        | public.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver:v2.1.12               |

**Note**  
You can find previous efs-csi-driver versions' images from [here](https://gallery.ecr.aws/efs-csi-driver/amazon/aws-efs-csi-driver)

### Features
* Static provisioning - Amazon EFS file system needs to be created manually first, then it could be mounted inside container as a persistent volume (PV) using the driver.
* Dynamic provisioning - Uses a persistent volume claim (PVC) to dynamically provision a persistent volume (PV). On Creating a PVC, kuberenetes requests Amazon EFS to create an Access Point in a file system which will be used to mount the PV.
* Mount Options - Mount options can be specified in the persistent volume (PV) or storage class for dynamic provisioning to define how the volume should be mounted.
* Encryption of data in transit - Amazon EFS file systems are mounted with encryption in transit enabled by default in the master branch version of the driver.
* Cross account mount - Amazon EFS file systems from different aws accounts can be mounted from an Amazon EKS cluster.
* Multiarch - Amazon EFS CSI driver image is now multiarch on ECR

**Note**  
Since Amazon EFS is an elastic file system, it doesn't really enforce any file system capacity. The actual storage capacity value in persistent volume and persistent volume claim is not used when creating the file system. However, since the storage capacity is a required field by Kubernetes, you must specify the value and you can use any valid value for the capacity.

### Installation

**Considerations**
+ The Amazon EFS CSI Driver isn't compatible with Windows\-based container images.
+ You can't use dynamic persistent volume provisioning with Fargate nodes, but you can use static provisioning.
+ Dynamic provisioning requires `1.2` or later of the driver. You can statically provision persistent volumes using version `1.1` of the driver on any [supported Amazon EKS cluster version](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html).
+ Version `1.3.2` or later of this driver supports the Arm64 architecture, including Amazon EC2 Graviton\-based instances.
+ Version `1.4.2` or later of this driver supports using FIPS for mounting file systems. For more information on how to enable FIPS, see [Helm](#-helm-).
+ Take note of the resource quotas for Amazon EFS. For example, there's a quota of 10000 access points that can be created for each Amazon EFS file system. For more information, see [https://docs.aws.amazon.com/efs/latest/ug/limits.html#limits-efs-resources-per-account-per-region](https://docs.aws.amazon.com/efs/latest/ug/limits.html#limits-efs-resources-per-account-per-region).

### Configure node startup taint
There are potential race conditions on node startup (especially when a node is first joining the cluster) where pods/processes that rely on the EFS CSI Driver can act on a node before the EFS CSI Driver is able to startup up and become fully ready. To combat this, the EFS CSI Driver contains a feature to automatically remove a taint from the node on startup. This feature was introduced from version v1.7.2 of the EFS CSI Driver and version v2.5.2 of its Helm chart. Users can taint their nodes when they join the cluster and/or on startup, to prevent other pods from running and/or being scheduled on the node prior to the EFS CSI Driver becoming ready.

This feature is activated by default, and cluster administrators should use the taint `efs.csi.aws.com/agent-not-ready:NoExecute` (any effect will work, but `NoExecute` is recommended). For example, EKS Managed Node Groups [support automatically tainting nodes](https://docs.aws.amazon.com/eks/latest/userguide/node-taints-managed-node-groups.html).

**Prerequisites**
+ An existing AWS Identity and Access Management \(IAM\) OpenID Connect \(OIDC\) provider for your cluster. To determine whether you already have one, or to create one, see [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html).
+ The AWS CLI installed and configured on your device or AWS CloudShell. To install the latest version, see [Installing, updating, and uninstalling the AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-install.html) and [Quick configuration with `aws configure`](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-quickstart.html#cli-configure-quickstart-config) in the AWS Command Line Interface User Guide. The AWS CLI version installed in the AWS CloudShell may also be several versions behind the latest version. To update it, see [Installing AWS CLI to your home directory](https://docs.aws.amazon.com/cloudshell/latest/userguide/vm-specs.html#install-cli-software) in the AWS CloudShell User Guide.
+ The `kubectl` command line tool is installed on your device or AWS CloudShell. The version can be the same as or up to one minor version earlier or later than the Kubernetes version of your cluster. To install or upgrade `kubectl`, see [Installing or updating `kubectl`](install-kubectl.md).

**Note**  
A Pod running on AWS Fargate automatically mounts an Amazon EFS file system, without needing the manual driver installation steps described on this page.

#### Set up driver permission
The driver requires IAM permission to talk to Amazon EFS to manage the volume on user's behalf. There are several methods to grant driver IAM permission:
* Using the EKS Pod Identity Add-on - [Install the EKS Pod Identity add-on to your EKS cluster](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-agent-setup.html). This doesn't need the efs-csi-driver to be installed through EKS add-on, it can be used no matter the method of installation of the efs-csi-driver. If this installation method is used, the ```AmazonEFSCSIDriverPolicy``` policy has to be added to the cluster's node group's IAM role. 
* Using IAM role for service account – Create an [IAM Role for service accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) with the required permissions in [iam-policy-example.json](./iam-policy-example.json). Uncomment annotations and put the IAM role ARN in the [service-account manifest](../deploy/kubernetes/base/controller-serviceaccount.yaml). For example steps, see [Create an IAM policy and role for Amazon EKS](./iam-policy-create.md).
* Using IAM [instance profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) – Grant all the worker nodes with [required permissions](./iam-policy-example.json) by attaching the policy to the instance profile of the worker.

------

#### Deploy the driver

There are several options for deploying the driver. The following are some examples.

------
##### [ Helm ]

This procedure requires Helm V3 or later. To install or upgrade Helm, see [Using Helm with Amazon EKS](https://docs.aws.amazon.com/eks/latest/userguide/helm.html).

**To install the driver using Helm**

1. Add the Helm repo.

   ```sh
   helm repo add aws-efs-csi-driver https://kubernetes-sigs.github.io/aws-efs-csi-driver/
   ```

2. Update the repo.

   ```sh
   helm repo update aws-efs-csi-driver
   ```

3. Install a release of the driver using the Helm chart.

   ```sh
   helm upgrade --install aws-efs-csi-driver --namespace kube-system aws-efs-csi-driver/aws-efs-csi-driver
   ```

   To specify an image repository, add the following argument. Replace the repository address with the cluster's [container image address](https://docs.aws.amazon.com/eks/latest/userguide/add-ons-images.html).
   ```sh
   --set image.repository=602401143452.dkr.ecr.region-code.amazonaws.com/eks/aws-efs-csi-driver
   ```

   If you already created a service account by following [Create an IAM policy and role for Amazon EKS](./iam-policy-create.md), then add the following arguments.
   ```sh
   --set controller.serviceAccount.create=false \
   --set controller.serviceAccount.name=efs-csi-controller-sa
   ```
   
   If you don't have outbound access to the Internet, add the following arguments.
   ```sh
   --set sidecars.livenessProbe.image.repository=602401143452.dkr.ecr.region-code.amazonaws.com/eks/livenessprobe \
   --set sidecars.node-driver-registrar.image.repository=602401143452.dkr.ecr.region-code.amazonaws.com/eks/csi-node-driver-registrar \
   --set sidecars.csiProvisioner.image.repository=602401143452.dkr.ecr.region-code.amazonaws.com/eks/csi-provisioner
   ```

   To force the Amazon EFS CSI driver to use FIPS for mounting the file system, add the following argument.
   ```sh
   --set useFIPS=true
   ```
**Note**  
`hostNetwork: true` (should be added under spec/deployment on kubernetes installations where AWS metadata is not reachable from pod network. To fix the following error `NoCredentialProviders: no valid providers in chain` this parameter should be added.)

------
##### [ Manifest \(private registry\) ]

If you want to download the image with a manifest, we recommend first trying these steps to pull secured images from the private Amazon ECR registry.

**To install the driver using images stored in the private Amazon ECR registry**

1. Download the manifest. Replace `release-X.X` with your desired branch. We recommend using the latest released version. For a list of active branches, see [Branches](../../../branches/active).

   ```sh
   kubectl kustomize \
       "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/ecr/?ref=release-2.X" > private-ecr-driver.yaml
   ```
   **Note**  
   If you encounter an issue that you aren't able to resolve by adding IAM permissions, try the [Manifest \(public registry\)](#-manifest-public-registry-) steps instead.

2. In the following command, replace `region-code` with the AWS Region that your cluster is in. Then run the modified command to replace `us-west-2` in the file with your AWS Region.

   ```sh
   sed -i.bak -e 's|us-west-2|region-code|' private-ecr-driver.yaml
   ```

3. Replace `account` in the following command with the account from [Amazon container image registries](add-ons-images.md) for the AWS Region that your cluster is in and then run the modified command to replace `602401143452` in the file.

   ```sh
   sed -i.bak -e 's|602401143452|account|' private-ecr-driver.yaml
   ```

4. If you already created a service account by following [Create an IAM policy and role for Amazon EKS](./iam-policy-create.md), then edit the `private-ecr-driver.yaml` file. Remove the following lines that create a Kubernetes service account.

   ```
   apiVersion: v1
   kind: ServiceAccount
   metadata:
     labels:
       app.kubernetes.io/name: aws-efs-csi-driver
     name: efs-csi-controller-sa
     namespace: kube-system
   ---
   ```

5. Apply the manifest.

   ```sh
   kubectl apply -f private-ecr-driver.yaml
   ```

------
##### [ Manifest \(public registry\) ]

For some situations, you may not be able to add the necessary IAM permissions to pull from the private Amazon ECR registry. One example of this scenario is if your [IAM principal](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_terms-and-concepts.html) isn't allowed to authenticate with someone else's account. When this is true, you can use the public Amazon ECR registry.

**To install the driver using images stored in the public Amazon ECR registry**

1. Download the manifest. Replace `release-X.X` with your desired branch. We recommend using the latest released version. For a list of active branches, see [Branches](../../../branches/active).

   ```sh
   kubectl kustomize \
       "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=release-2.X" > public-ecr-driver.yaml
   ```

2. If you already created a service account by following [Create an IAM policy and role](./iam-policy-create.md), then edit the `public-ecr-driver.yaml` file. Remove the following lines that create a Kubernetes service account.

   ```sh
   apiVersion: v1
   kind: ServiceAccount
   metadata:
     labels:
       app.kubernetes.io/name: aws-efs-csi-driver
     name: efs-csi-controller-sa
     namespace: kube-system
   ---
   ```

3. Apply the manifest.

   ```sh
   kubectl apply -f public-ecr-driver.yaml
   ```
------

After deploying the driver, you can continue to these sections:
* [Create an Amazon EFS file system for Amazon EKS](./efs-create-filesystem.md)
* [Examples](#examples)

### Container Arguments for efs-plugin of efs-csi-node daemonset
| Parameters                  | Values | Default | Optional | Description                                                                                                                                                                                                                             |
|-----------------------------|--------|---------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| vol-metrics-opt-in          |        | false   | true     | Opt in to emit volume metrics.                                                                                                                                                                                                          |
| vol-metrics-refresh-period  |        | 240     | true     | Refresh period for volume metrics in minutes.                                                                                                                                                                                           |
| vol-metrics-fs-rate-limit   |        | 5       | true     | Volume metrics routines rate limiter per file system.                                                                                                                                                                                   |



##### Understanding the Impact of vol-metrics-opt-in:
Enabling the vol-metrics-opt-in parameter activates the gathering of inode and disk usage data. This functionality, particularly in scenarios with larger file systems, may result in an uptick in memory usage due to the detailed aggregation of file system information. We advise users with large-scale file systems to consider this aspect when utilizing this feature.


### Container Arguments for deployment(controller) 
| Parameters                  | Values | Default | Optional | Description                                                                                                                                                                                                                                                   |
|-----------------------------|--------|---------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| delete-access-point-root-dir|        | false  | true     | Opt in to delete access point root directory by DeleteVolume. By default, DeleteVolume will delete the access point behind Persistent Volume and deleting access point will not delete the access point root directory or its contents.                       |
| adaptive-retry-mode         |        | true   | true     | Opt out to use standard sdk retry mode for EFS API calls. By default, Driver will use adaptive mode for the sdk retry configuration which heavily rate limits EFS API requests to reduce throttling if throttling is observed.                                |
| tags                         |       |         | true     | Space separated key:value pairs which will be added as tags for Amazon EFS resources. For example, '--tags=name:efs-tag-test date:Jan24'. To include a ':' in the tag name or value, use \ as an escape character, for example '--tags="tag\:name:tag\:value" |
### Upgrading the Amazon EFS CSI Driver


### Important Notes on EFS CSI Driver v2 Upgrade

Starting with version 2.X.X, the EFS CSI driver incorporates efs-utils v2.X.X, which introduces a significant change in how TLS encryption for mounts is handled. The previous stunnel component has been replaced by efs-proxy, a custom-built AWS component. To take advantage of the enhanced performance offered by efs-proxy, it's necessary to re-mount any existing file systems after upgrading.


#### Upgrade to the latest version:
If you want to update to latest released version:
```sh
kubectl apply -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=release-2.0"
```

#### Upgrade to a specific version:
If you want to update to a specific version, first customize the driver yaml file locally:
```sh
kubectl kustomize "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=release-2.0" > driver.yaml
```

Then, update all lines referencing `image: amazon/aws-efs-csi-driver` to the desired version (e.g., to `image: amazon/aws-efs-csi-driver:v2.1.12`) in the yaml file, and deploy driver yaml again:
```sh
kubectl apply -f driver.yaml
```

### Uninstalling the Amazon EFS CSI Driver

Note: While the aws-efs-csi-driver daemonsets and controller are deleted from the cluster no new EFS PVCs will be able to be created, new pods that are created which use an EFS PV volume will not function (because the PV will not mount), and any existing pods with mounted PVs will not be able to access EFS until the driver is successfully re-installed (either manually, or through the [EKS add-on system](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html#efs-install-driver)).

Uninstall the self-managed EFS CSI Driver with either Helm or Kustomize, depending on your installation method. If you are using the driver as a managed EKS add-on, see the [EKS Documentation](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html#efs-install-driver).

**Helm**

```
helm uninstall aws-efs-csi-driver --namespace kube-system
```

**Kustomize**

```
kubectl delete -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=<YOUR-CSI-DRIVER-RELEASE-OR-TAG>"
```

### Examples
Before following the examples, you need to:
* Get yourself familiar with how to setup Kubernetes on AWS and how to [create Amazon EFS file system](https://docs.aws.amazon.com/efs/latest/ug/getting-started.html).
* When creating an Amazon EFS file system, make sure it is accessible from the Kubernetes cluster. This can be achieved by creating the file system inside the same VPC as the Kubernetes cluster or using VPC peering.
* Install Amazon EFS CSI driver following the [Installation](README.md#Installation) steps.

#### Example links
* [Static provisioning](../examples/kubernetes/static_provisioning/README.md)
* [Dynamic provisioning](../examples/kubernetes/dynamic_provisioning/README.md)
* [Encryption in transit](../examples/kubernetes/encryption_in_transit/README.md)
* [Accessing the file system from multiple pods](../examples/kubernetes/multiple_pods/README.md)
* [Consume Amazon EFS in StatefulSets](../examples/kubernetes/statefulset/README.md)
* [Mount subpath](../examples/kubernetes/volume_path/README.md)
* [Use Access Points](../examples/kubernetes/access_points/README.md)

## Resource limits
The controller container has different memory / CPU requirements based on the workload scale, concurrency, and configurations. When configuring your controller with `delete-access-point-root-dir=true`, we recommend setting higher resource limits if your workload requires many concurrent volume deletions. For example, for a workload that requires 100 concurrent PVC deletions, we recommend setting a minimum CPU limit of 3000m and a minimum memory limit of 2.5 GiB. 

Alternatively, if you would prefer not to allocate these resources to your controller container, we advise lowering concurrency by lowering the `--worker-threads` argument of the [external-provisioner](https://github.com/kubernetes-csi/external-provisioner).

## Timeouts
For most highly concurrent workloads, we recommend increasing the default timeout argument set in the [external-provisioner](https://github.com/kubernetes-csi/external-provisioner) from 15 seconds to 60 seconds. This will avoid provisioning failures due to throttling and resource contention in the controller container. 


## Using botocore to retrieve mount target ip address when dns name cannot be resolved
* Amazon EFS CSI driver supports using botocore to retrieve mount target ip address when dns name cannot be resolved, e.g., when user is mounting a file system in another VPC, botocore comes preinstalled on efs-csi-driver which can solve this DNS issue.
* IAM policy prerequisites to use this feature :  
  Allow ```elasticfilesystem:DescribeMountTargets``` and ```ec2:DescribeAvailabilityZones``` actions in your policy attached to the Amazon EKS service account role, refer to example policy [here](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/docs/iam-policy-example.json#L9-L10).

## Development
* Please go through [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and [Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs) to get some basic understanding of CSI driver before you start.

* If you are about to update iam policy file, please also update efs policy in weaveworks/eksctl
https://github.com/weaveworks/eksctl/blob/main/pkg/cfn/builder/statement.go
*/

### Requirements
* Golang 1.13.4+

### Dependency
Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, to build the project run: `make`

### Testing
To execute all unit tests, run: `make test`

### Troubleshooting
To pull logs and troubleshoot the driver, see [troubleshooting/README.md](../troubleshooting/README.md).

## License
This library is licensed under the Apache 2.0 License.
