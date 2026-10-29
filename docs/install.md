# Installation

**Considerations**
+ The Amazon EFS CSI Driver isn't compatible with Windows\-based container images.
+ You can't use dynamic persistent volume provisioning with Fargate nodes, but you can use static provisioning.
+ Dynamic provisioning requires `1.2` or later of the driver. You can statically provision persistent volumes using version `1.1` of the driver on any [supported Amazon EKS cluster version](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html).
+ Version `1.3.2` or later of this driver supports the Arm64 architecture, including Amazon EC2 Graviton\-based instances.
+ Version `1.4.2` or later of this driver supports using FIPS for mounting file systems. For more information on how to enable FIPS, see [Helm](#helm).
+ Take note of the resource quotas for Amazon EFS. For example, there's a quota of 10000 access points that can be created for each Amazon EFS file system. For more information, see [https://docs.aws.amazon.com/efs/latest/ug/limits.html#limits-efs-resources-per-account-per-region](https://docs.aws.amazon.com/efs/latest/ug/limits.html#limits-efs-resources-per-account-per-region).
+ Amazon S3 Files is supported from version `3.0.0` or later of this driver.

## Configure node startup taint
There are potential race conditions on node startup (especially when a node is first joining the cluster) where pods/processes that rely on the EFS CSI Driver can act on a node before the EFS CSI Driver is able to startup up and become fully ready. To combat this, the EFS CSI Driver contains a feature to automatically remove a taint from the node on startup. This feature was introduced from version v1.7.2 of the EFS CSI Driver and version v2.5.2 of its Helm chart. Users can taint their nodes when they join the cluster and/or on startup, to prevent other pods from running and/or being scheduled on the node prior to the EFS CSI Driver becoming ready.

This feature is activated by default, and cluster administrators should use the taint `efs.csi.aws.com/agent-not-ready:NoExecute` (any effect will work, but `NoExecute` is recommended). For example, EKS Managed Node Groups [support automatically tainting nodes](https://docs.aws.amazon.com/eks/latest/userguide/node-taints-managed-node-groups.html).

**Prerequisites**
+ Only for IAM roles for service accounts: an existing AWS Identity and Access Management \(IAM\) OpenID Connect \(OIDC\) provider for your cluster. EKS Pod Identity does not need one. To determine whether you already have one, or to create one, see [Creating an IAM OIDC provider for your cluster](https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html).
+ The AWS CLI installed and configured on your device or AWS CloudShell. To install the latest version, see [Installing, updating, and uninstalling the AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-install.html) and [Quick configuration with `aws configure`](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-quickstart.html#cli-configure-quickstart-config) in the AWS Command Line Interface User Guide. The AWS CLI version installed in the AWS CloudShell may also be several versions behind the latest version. To update it, see [Installing AWS CLI to your home directory](https://docs.aws.amazon.com/cloudshell/latest/userguide/vm-specs.html#install-cli-software) in the AWS CloudShell User Guide.
+ The `kubectl` command line tool is installed on your device or AWS CloudShell. The version can be the same as or up to one minor version earlier or later than the Kubernetes version of your cluster. To install or upgrade `kubectl`, see [Installing or updating `kubectl`](https://kubernetes.io/docs/tasks/tools/#kubectl).

**Note**  
A Pod running on AWS Fargate automatically mounts an Amazon EFS file system or Amazon S3 file system, without needing the manual driver installation steps described on this page.

## Set up driver permission
The driver requires IAM permissions to talk to Amazon EFS or Amazon S3 Files to manage the volume on the user's behalf. The controller (`efs-csi-controller-sa`) creates and deletes access points. The node DaemonSet (`efs-csi-node-sa`) needs its own role for the `iam` mount option, S3 Files and efs-utils logs. Give each service account its own role.

The examples below attach the tag-based policies, which limit each cluster's driver to the file systems and access points tagged for that cluster. See [Cluster tag isolation](./cluster-tag-isolation.md) for how the policies work. The AWS managed policies (`AmazonEFSCSIDriverPolicy` and the others in [Create IAM roles](./iam-policy-create.md)) don't isolate clusters: they give each cluster's driver IAM permissions to modify the EFS access points of other clusters in the account. Use them when several clusters share one file system (see [`AmazonEFSCSIDriverPolicy`: file systems shared between clusters](./cluster-tag-isolation.md#amazonefscsidriverpolicy-file-systems-shared-between-clusters)). If your cluster is in the AWS GovCloud Regions, replace `arn:aws:` with `arn:aws-us-gov:`. Likewise, if your cluster is in the AWS China Regions, replace `arn:aws:` with `arn:aws-cn:`.

> [!IMPORTANT]
> These steps are for new clusters. A cluster that already has access points created by the driver must migrate first:
> those access points have no cluster tag, so `enforce` ignores them and the tag-based policy denies deleting them.
> Follow [Existing clusters](./cluster-tag-isolation.md#existing-clusters) to backfill the cluster tag before you set `enforce`.

Every file system the cluster uses must carry the cluster tag before you provision volumes on it:

```sh
aws efs tag-resource --resource-id fs-0123456789abcdef0 --tags Key=eks:eks-cluster-name,Value=my-cluster
```

### Amazon EKS: EKS Pod Identity (Recommended)
With Pod Identity, one controller role and one node role can be shared by every cluster in the account, and each cluster's driver is still limited to its own resources. Create the roles once and repeat only the associations (step 3) and step 4 for each cluster. See [Roles per cluster](./cluster-tag-isolation.md#roles-per-cluster).

1. Install the [EKS Pod Identity Agent](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-agent-setup.html) add-on, if it is not already installed. The EFS CSI driver itself can be installed any way (Helm, manifests or the EKS add-on).
   ```sh
   aws eks create-addon --cluster-name my-cluster --addon-name eks-pod-identity-agent
   ```

2. Create the controller and node roles with the standard [EKS Pod Identity trust policy](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-role.html), and attach the tag-based policies:
   ```sh
   aws iam create-role --role-name efs-csi-controller \
     --assume-role-policy-document file://pod-identity-trust-policy.json
   aws iam put-role-policy --role-name efs-csi-controller --policy-name efs-csi-cluster-tag \
     --policy-document file://docs/iam-policy-cluster-tag-controller-example.json

   aws iam create-role --role-name efs-csi-node \
     --assume-role-policy-document file://pod-identity-trust-policy.json
   aws iam put-role-policy --role-name efs-csi-node --policy-name efs-csi-cluster-tag-node \
     --policy-document file://docs/iam-policy-cluster-tag-node-example.json
   ```

3. Associate the roles with the service accounts:
   ```sh
   aws eks create-pod-identity-association --cluster-name my-cluster --namespace kube-system \
     --service-account efs-csi-controller-sa \
     --role-arn arn:aws:iam::111122223333:role/efs-csi-controller
   aws eks create-pod-identity-association --cluster-name my-cluster --namespace kube-system \
     --service-account efs-csi-node-sa \
     --role-arn arn:aws:iam::111122223333:role/efs-csi-node
   ```

4. When you [deploy the driver](#deploy-the-driver), set the cluster tag options:
   ```sh
   helm upgrade --install aws-efs-csi-driver aws-efs-csi-driver/aws-efs-csi-driver --namespace kube-system \
     --set controller.clusterName=my-cluster \
     --set controller.clusterTagKey=eks:eks-cluster-name \
     --set controller.clusterTagIsolation=enforce
   ```
   Without Helm, pass the same values as controller flags:
   `--cluster-name=my-cluster --cluster-tag-key=eks:eks-cluster-name --cluster-tag-isolation=enforce`.

### Other credentials: IAM roles for service accounts
The included tag-based policies need the session tags that EKS Pod Identity adds. With [IAM roles for service accounts (IRSA)](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html), including on self-managed clusters through the [amazon-eks-pod-identity-webhook](https://github.com/aws/amazon-eks-pod-identity-webhook), follow [Create IAM roles](./iam-policy-create.md#iam-roles-for-service-accounts-irsa) to attach the AWS managed policies, and set `--cluster-tag-isolation=disable`. To isolate clusters without Pod Identity, you can write your own policy from the tag-based one (see [Other credentials](./cluster-tag-isolation.md#other-credentials)).

### Custom tags require additional IAM permissions

If you configure the controller with custom tags (the `--tags` flag, or `controller.tags` in the Helm chart), the IAM policy attached to the controller service account must allow those tag keys on `elasticfilesystem:CreateAccessPoint`. Otherwise dynamic provisioning fails: `CreateAccessPoint` is denied, and PVCs stay stuck in `Pending` with a permission error in the controller logs.

This most commonly affects clusters using the AWS-managed `AmazonEFSCSIDriverPolicy`. That policy gates access point creation with a condition that only allows the default `efs.csi.aws.com/cluster` tag key:

```json
"Condition": {
  "ForAllValues:StringEquals": {
    "aws:TagKeys": "efs.csi.aws.com/cluster"
  }
}
```

Because `ForAllValues:StringEquals` requires *every* tag key in the request to be in the allow-list, adding any custom key (for example `environment`) makes the condition fail. To use custom tags, attach a customer-managed policy (or an inline policy) that grants `elasticfilesystem:CreateAccessPoint` without restricting the tag keys. The example policy in this repo, [iam-policy-example.json](./iam-policy-example.json), already does this — it only requires the default tag to be present and does not restrict additional keys:

```json
{
  "Effect": "Allow",
  "Action": "elasticfilesystem:CreateAccessPoint",
  "Resource": "*",
  "Condition": {
    "StringLike": {
      "aws:RequestTag/efs.csi.aws.com/cluster": "true"
    }
  }
}
```

If you must keep an `aws:TagKeys` restriction, extend the allow-list to include each of your custom tag keys.


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
       "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/ecr/?ref=release-3.X" > private-ecr-driver.yaml
   ```
   **Note**  
   If you encounter an issue that you aren't able to resolve by adding IAM permissions, try the [Manifest \(public registry\)](#-manifest-public-registry-) steps instead.

2. In the following command, replace `region-code` with the AWS Region that your cluster is in. Then run the modified command to replace `us-west-2` in the file with your AWS Region.

   ```sh
   sed -i.bak -e 's|us-west-2|region-code|' private-ecr-driver.yaml
   ```

3. Replace `account` in the following command with the account from [Amazon container image registries](https://docs.aws.amazon.com/eks/latest/userguide/add-ons-images.html) for the AWS Region that your cluster is in and then run the modified command to replace `602401143452` in the file.

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
       "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=release-3.X" > public-ecr-driver.yaml
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
* [Create an Amazon S3 file system for Amazon EKS](./s3files-create-filesystem.md)
* [Examples](#examples)

-----
# Upgrading the Amazon EFS CSI Driver

## Important Notes on EFS CSI Driver v3 Upgrade

Starting with version 3.X.X, the EFS CSI driver adds support for Amazon S3 file systems. There are no breaking changes if you are upgrading from latest v2.X.X version.

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

Then, update all lines referencing `image: amazon/aws-efs-csi-driver` to the desired version (e.g., to `image: amazon/aws-efs-csi-driver:v3.5.3`) in the yaml file, and deploy driver yaml again:
```sh
kubectl apply -f driver.yaml
```

Or after `v2.2.0`, we support to deploy using specific version tag:
```sh
kubectl apply -k github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable?ref=v3.5.3
```

-----
# Uninstalling the Amazon EFS CSI Driver

Note: While the aws-efs-csi-driver daemonsets and controller are deleted from the cluster no new EFS PVCs will be able to be created, new pods that are created which use an EFS or S3 Files PV volume will not function (because the PV will not mount), and any existing pods with mounted PVs will not be able to access EFS or S3 Files until the driver is successfully re-installed (either manually, or through the [EKS add-on system](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html#efs-install-driver)).

Uninstall the self-managed EFS CSI Driver with either Helm or Kustomize, depending on your installation method. If you are using the driver as a managed EKS add-on, see the [EKS Documentation](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html#efs-install-driver).

**Helm**

```
helm uninstall aws-efs-csi-driver --namespace kube-system
```

**Kustomize**

```
kubectl delete -k "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=<YOUR-CSI-DRIVER-RELEASE-OR-TAG>"
```
