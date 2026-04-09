## Create IAM roles for Amazon EKS

The following example details how to use IAM roles for service accounts to talk to Amazon EFS and Amazon S3 Files. The EFS CSI driver uses two service accounts with separate IAM roles:

- `efs-csi-controller-sa` — used by the controller, requires `AmazonEFSCSIDriverPolicy` and `AmazonS3FilesCSIDriverPolicy`.
- `efs-csi-node-sa` — used by the node daemonset, requires:
  - `AmazonS3ReadOnlyAccess` — enables direct S3 read access so the driver can stream objects directly from S3 buckets for higher throughput.
  - `AmazonElasticFileSystemsUtils` — enables publishing efs-utils logs to Amazon CloudWatch for visibility into mount operations and easier troubleshooting.

You can assign these roles using [EKS Pod Identity](#eks-pod-identity) or [IAM Roles for Service Accounts (IRSA)](#iam-roles-for-service-accounts-irsa).

------

### EKS Pod Identity

EKS Pod Identity is the recommended way to grant IAM permissions to pods on Amazon EKS. It does not require an OIDC provider and simplifies role trust management.

#### Prerequisites

- Install the Amazon EKS Pod Identity Agent add-on on your cluster. See [Setting up the Amazon EKS Pod Identity Agent](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-agent-setup.html).

#### 1. Create the controller IAM role

1. Save the following to a file named `controller-trust-policy.json`.

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": {
           "Service": "pods.eks.amazonaws.com"
         },
         "Action": [
           "sts:AssumeRole",
           "sts:TagSession"
         ]
       }
     ]
   }
   ```

1. Create the role and attach the managed policies.

   ```sh
   aws iam create-role \
     --role-name EKS_EFS_CSI_ControllerRole \
     --assume-role-policy-document file://"controller-trust-policy.json"

   aws iam attach-role-policy \
     --role-name EKS_EFS_CSI_ControllerRole \
     --policy-arn arn:aws:iam::aws:policy/service-role/AmazonEFSCSIDriverPolicy

   aws iam attach-role-policy \
     --role-name EKS_EFS_CSI_ControllerRole \
     --policy-arn arn:aws:iam::aws:policy/service-role/AmazonS3FilesCSIDriverPolicy
   ```

1. Create the pod identity association.

   ```sh
   aws eks create-pod-identity-association \
     --cluster-name {YOUR_CLUSTER_NAME} \
     --namespace kube-system \
     --service-account efs-csi-controller-sa \
     --role-arn arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:role/EKS_EFS_CSI_ControllerRole
   ```

#### 2. Create the node IAM role

1. Save the following to a file named `node-trust-policy.json`.

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": {
           "Service": "pods.eks.amazonaws.com"
         },
         "Action": [
           "sts:AssumeRole",
           "sts:TagSession"
         ]
       }
     ]
   }
   ```

1. Create the role and attach the managed policies.

   ```sh
   aws iam create-role \
     --role-name EKS_EFS_CSI_NodeRole \
     --assume-role-policy-document file://"node-trust-policy.json"

   aws iam attach-role-policy \
     --role-name EKS_EFS_CSI_NodeRole \
     --policy-arn arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess

   aws iam attach-role-policy \
     --role-name EKS_EFS_CSI_NodeRole \
     --policy-arn arn:aws:iam::aws:policy/AmazonElasticFileSystemsUtils
   ```

1. Create the pod identity association.

   ```sh
   aws eks create-pod-identity-association \
     --cluster-name {YOUR_CLUSTER_NAME} \
     --namespace kube-system \
     --service-account efs-csi-node-sa \
     --role-arn arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:role/EKS_EFS_CSI_NodeRole
   ```

------

### IAM Roles for Service Accounts (IRSA)

You can create the roles using `eksctl` or the AWS CLI.

#### [ eksctl ]

   Run the following commands to create the IAM roles and Kubernetes service accounts. Each command attaches the required AWS managed policies, sets the Kubernetes service account with the IAM role ARN, and adds the Kubernetes service account name to the trust policy for the IAM role.

   ```sh
    USER_ACCOUNT={YOUR_AWS_ACCOUNT_ID}
    CLUSTER_NAME={YOUR_CLUSTER_NAME}
    REGION_CODE={YOUR_REGION_CODE}

    # Create the controller role with AmazonEFSCSIDriverPolicy and AmazonS3FilesCSIDriverPolicy
    CONTROLLER_ROLE_NAME={YOUR_CONTROLLER_IAM_ROLE_NAME}
    eksctl create iamserviceaccount \
       --name efs-csi-controller-sa \
       --namespace kube-system \
       --cluster $CLUSTER_NAME \
       --role-name $CONTROLLER_ROLE_NAME \
       --attach-policy-arn arn:aws:iam::aws:policy/service-role/AmazonEFSCSIDriverPolicy \
       --attach-policy-arn arn:aws:iam::aws:policy/service-role/AmazonS3FilesCSIDriverPolicy \
       --approve \
       --region $REGION_CODE

    # Create the node role with AmazonS3ReadOnlyAccess and AmazonElasticFileSystemsUtils
    NODE_ROLE_NAME={YOUR_NODE_IAM_ROLE_NAME}
    eksctl create iamserviceaccount \
      --name efs-csi-node-sa \
      --namespace kube-system \
      --cluster $CLUSTER_NAME \
      --role-name $NODE_ROLE_NAME \
      --attach-policy-arn arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess \
      --attach-policy-arn arn:aws:iam::aws:policy/AmazonElasticFileSystemsUtils \
      --approve \
      --region $REGION_CODE
   ```

------
#### [ AWS CLI ]

   1. Find your cluster's OIDC provider URL. Replace `{YOUR_CLUSTER_NAME}` with your value. If the output returns `None` review the **Prerequisites**.

      ```sh
      aws eks describe-cluster --name {YOUR_CLUSTER_NAME} --query "cluster.identity.oidc.issuer" --output text
      ```

      The example output is as follows.

      ```
      https://oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE
      ```

   1. Create the IAM role for the controller service account.

      1. Copy the following example to a file named `controller-trust-policy.json`. Replace `{YOUR_AWS_ACCOUNT_ID}`, `EXAMPLED539D4633E53DE1B71EXAMPLE` and `region-code` with your values.

         ```json
         {
           "Version": "2012-10-17",
           "Statement": [
             {
               "Effect": "Allow",
               "Principal": {
                 "Federated": "arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:oidc-provider/oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE"
               },
               "Action": "sts:AssumeRoleWithWebIdentity",
               "Condition": {
                 "StringEquals": {
                   "oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE:sub": "system:serviceaccount:kube-system:efs-csi-controller-sa"
                 }
               }
             }
           ]
         }
         ```

      1. Create the IAM role.

         ```sh
         aws iam create-role \
           --role-name EKS_EFS_CSI_ControllerRole \
           --assume-role-policy-document file://"controller-trust-policy.json"
         ```

      1. Attach the AWS managed policies to the controller role.

         ```sh
         aws iam attach-role-policy \
           --role-name EKS_EFS_CSI_ControllerRole \
           --policy-arn arn:aws:iam::aws:policy/service-role/AmazonEFSCSIDriverPolicy

         aws iam attach-role-policy \
           --role-name EKS_EFS_CSI_ControllerRole \
           --policy-arn arn:aws:iam::aws:policy/service-role/AmazonS3FilesCSIDriverPolicy
         ```

   1. Create the IAM role for the node service account.

      1. Copy the following example to a file named `node-trust-policy.json`. Replace `{YOUR_AWS_ACCOUNT_ID}`, `EXAMPLED539D4633E53DE1B71EXAMPLE` and `region-code` with your values.

         ```json
         {
           "Version": "2012-10-17",
           "Statement": [
             {
               "Effect": "Allow",
               "Principal": {
                 "Federated": "arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:oidc-provider/oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE"
               },
               "Action": "sts:AssumeRoleWithWebIdentity",
               "Condition": {
                 "StringEquals": {
                   "oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE:sub": "system:serviceaccount:kube-system:efs-csi-node-sa"
                 }
               }
             }
           ]
         }
         ```

      1. Create the IAM role.

         ```sh
         aws iam create-role \
           --role-name EKS_EFS_CSI_NodeRole \
           --assume-role-policy-document file://"node-trust-policy.json"
         ```

      1. Attach the AWS managed policies to the node role.

         ```sh
         aws iam attach-role-policy \
           --role-name EKS_EFS_CSI_NodeRole \
           --policy-arn arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess

         aws iam attach-role-policy \
           --role-name EKS_EFS_CSI_NodeRole \
           --policy-arn arn:aws:iam::aws:policy/AmazonElasticFileSystemsUtils
         ```

   1. Create the Kubernetes service accounts with their respective IAM role ARNs.

      1. Save the following to a file named `efs-service-account.yaml`.
         ```yaml
         ---
         apiVersion: v1
         kind: ServiceAccount
         metadata:
           labels:
             app.kubernetes.io/name: aws-efs-csi-driver
           name: efs-csi-controller-sa
           namespace: kube-system
           annotations:
             eks.amazonaws.com/role-arn: arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:role/EKS_EFS_CSI_ControllerRole
         ---
         apiVersion: v1
         kind: ServiceAccount
         metadata:
           labels:
             app.kubernetes.io/name: aws-efs-csi-driver
           name: efs-csi-node-sa
           namespace: kube-system
           annotations:
             eks.amazonaws.com/role-arn: arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:role/EKS_EFS_CSI_NodeRole
         ```

      1. Create the Kubernetes service accounts on your cluster.

         ```sh
         kubectl apply -f efs-service-account.yaml
         ```
------

> **Note:** The `AmazonS3ReadOnlyAccess` policy grants read access to all S3 buckets. To constrain access to specific buckets, you can detach it and replace it with a tag-based inline policy. For example, to allow access only to buckets tagged with a specific key/value pair, replace `{YOUR_TAG_KEY}` and `{YOUR_TAG_VALUE}` below. If your cluster is in the AWS GovCloud \(US\-East\) or AWS GovCloud \(US\-West\) AWS Regions, then replace `arn:aws:` with `arn:aws-us-gov:`.
>
> ```sh
> # Detach the broad managed policy
> aws iam detach-role-policy \
>   --role-name EKS_EFS_CSI_NodeRole \
>   --policy-arn arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess
> ```
>
> Save the following to `s3-tag-read-policy.json`:
>
> ```json
> {
>     "Version": "2012-10-17",
>     "Statement": [
>         {
>             "Effect": "Allow",
>             "Action": [
>                 "s3:GetObject",
>                 "s3:GetObjectVersion"
>             ],
>             "Resource": "arn:aws:s3:::*/*",
>             "Condition": {
>                 "StringEquals": {
>                     "s3:ExistingObjectTag/{YOUR_TAG_KEY}": "{YOUR_TAG_VALUE}"
>                 }
>             }
>         },
>         {
>             "Effect": "Allow",
>             "Action": [
>                 "s3:ListBucket",
>                 "s3:GetBucketLocation"
>             ],
>             "Resource": "arn:aws:s3:::*",
>             "Condition": {
>                 "StringEquals": {
>                     "aws:ResourceTag/{YOUR_TAG_KEY}": "{YOUR_TAG_VALUE}"
>                 }
>             }
>         }
>     ]
> }
> ```
>
> Then attach it to the node role:
>
> ```sh
> NODE_ROLE_NAME={YOUR_NODE_IAM_ROLE_NAME}
> aws iam put-role-policy \
>   --role-name $NODE_ROLE_NAME \
>   --policy-name S3TagBasedReadAccess \
>   --policy-document file://s3-tag-read-policy.json
> ```
