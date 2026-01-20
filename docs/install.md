# Installation

**Considerations**
+ The Amazon EFS CSI Driver isn't compatible with Windows\-based container images.
+ You can't use dynamic persistent volume provisioning with Fargate nodes, but you can use static provisioning.
+ Dynamic provisioning requires `1.2` or later of the driver. You can statically provision persistent volumes using version `1.1` of the driver on any [supported Amazon EKS cluster version](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html).
+ Version `1.3.2` or later of this driver supports the Arm64 architecture, including Amazon EC2 Graviton\-based instances.
+ Version `1.4.2` or later of this driver supports using FIPS for mounting file systems. For more information on how to enable FIPS, see [Helm](#helm).
+ Take note of the resource quotas for Amazon EFS. For example, there's a quota of 10000 access points that can be created for each Amazon EFS file system. For more information, see [https://docs.aws.amazon.com/efs/latest/ug/limits.html#limits-efs-resources-per-account-per-region](https://docs.aws.amazon.com/efs/latest/ug/limits.html#limits-efs-resources-per-account-per-region).

## Configure node startup taint
There are potential race conditions on node startup (especially when a node is first joining the cluster) where pods/processes that rely on the EFS CSI Driver can act on a node before the EFS CSI Driver is able to startup up and become fully ready. To combat this, the EFS CSI Driver contains a feature to automatically remove a taint from the node on startup. This feature was introduced from version v1.7.2 of the EFS CSI Driver and version v2.5.2 of its Helm chart. Users can taint their nodes when they join the cluster and/or on startup, to prevent other pods from running and/or being scheduled on the node prior to the EFS CSI Driver becoming ready.

This feature is activated by default, and cluster administrators should use the taint `efs.csi.aws.com/agent-not-ready:NoExecute` (any effect will work, but `NoExecute` is recommended). For example, EKS Managed Node Groups [support automatically tainting nodes](https://docs.aws.amazon.com/eks/latest/userguide/node-taints-managed-node-groups.html).

**Prerequisites**
+ An existing AWS Identity and Access Management \(IAM\) OpenID Connect \(OIDC\) provider for your cluster. To determine whether you already have one, or to create one, see [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html).
+ The AWS CLI installed and configured on your device or AWS CloudShell. To install the latest version, see [Installing, updating, and uninstalling the AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-install.html) and [Quick configuration with `aws configure`](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-quickstart.html#cli-configure-quickstart-config) in the AWS Command Line Interface User Guide. The AWS CLI version installed in the AWS CloudShell may also be several versions behind the latest version. To update it, see [Installing AWS CLI to your home directory](https://docs.aws.amazon.com/cloudshell/latest/userguide/vm-specs.html#install-cli-software) in the AWS CloudShell User Guide.
+ The `kubectl` command line tool is installed on your device or AWS CloudShell. The version can be the same as or up to one minor version earlier or later than the Kubernetes version of your cluster. To install or upgrade `kubectl`, see [Installing or updating `kubectl`](https://kubernetes.io/docs/tasks/tools/#kubectl).

**Note**  
A Pod running on AWS Fargate automatically mounts an Amazon EFS file system, without needing the manual driver installation steps described on this page.

## Set up driver permission
The driver requires IAM permission to talk to Amazon EFS to manage the volume on user's behalf. There are several methods to grant driver IAM permission:
* Using the EKS Pod Identity Add-on - [Install the EKS Pod Identity add-on to your EKS cluster](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-agent-setup.html). This doesn't need the efs-csi-driver to be installed through EKS add-on, it can be used no matter the method of installation of the efs-csi-driver. If this installation method is used, the ```AmazonEFSCSIDriverPolicy``` policy has to be added to the cluster's node group's IAM role. 
* Using IAM role for service account – Create an [IAM Role for service accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) with the required permissions in [iam-policy-example.json](./iam-policy-example.json). Uncomment annotations and put the IAM role ARN in the [service-account manifest](../deploy/kubernetes/base/controller-serviceaccount.yaml). For example steps, see [Create an IAM policy and role for Amazon EKS](./iam-policy-create.md).
* Using IAM [instance profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) – Grant all the worker nodes with [required permissions](./iam-policy-example.json) by attaching the policy to the instance profile of the worker.


## Deploy the driver

There are several options for deploying the driver. The following are some examples.

### [ Helm ]

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
### [ Manifest \(private registry\) ]

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
### [ Manifest \(public registry\) ]

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

-----
# Upgrading the Amazon EFS CSI Driver


## Important Notes on EFS CSI Driver v2 Upgrade

Starting with version 2.X.X, the EFS CSI driver incorporates efs-utils v2.X.X, which introduces a significant change in how TLS encryption for mounts is handled. The previous stunnel component has been replaced by efs-proxy, a custom-built AWS component. To take advantage of the enhanced performance offered by efs-proxy, it's necessary to re-mount any existing file systems after upgrading.


### Upgrade to the latest version:
If you want to update to latest released version:
```sh
kubectl apply -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=release-2.2"
```

### Upgrade to a specific version:
If you want to update to a specific version, first customize the driver yaml file locally:
```sh
kubectl kustomize "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=release-2.2" > driver.yaml
```

Then, update all lines referencing `image: amazon/aws-efs-csi-driver` to the desired version (e.g., to `image: amazon/aws-efs-csi-driver:v2.3.0`) in the yaml file, and deploy driver yaml again:
```sh
kubectl apply -f driver.yaml
```

Or after `v2.2.0`, we support to deploy using specific version tag:
```sh
kubectl apply -k github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable?ref=v2.3.0
```

-----
# Uninstalling the Amazon EFS CSI Driver

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