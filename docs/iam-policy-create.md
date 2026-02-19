## Create an IAM policy and role for Amazon EKS

The following example details how to use an IAM role for service account to talk to Amazon EFS and Amazon S3 Files.

1. Create an IAM policy that allows the CSI driver's service account to make AWS API calls on your behalf. The policy includes permissions for Amazon EFS and S3 Files operations.

   1. Download the AWS IAM policy document.

      ```sh
      curl -O https://raw.githubusercontent.com/kubernetes-sigs/aws-efs-csi-driver/master/docs/iam-policy-example.json
      ```

   1. Create a new policy `AmazonEKS_EFS_CSI_Driver_Policy` 

      ```sh
      aws iam create-policy \
          --policy-name AmazonEKS_EFS_CSI_Driver_Policy \
          --policy-document file://iam-policy-example.json
      ```

2. Create an IAM role with the IAM policy attached to it. Configure the Kubernetes service account with the IAM role ARN, and the IAM role with the Kubernetes service account name. You can create the role using `eksctl` or the AWS CLI.

------
#### [ eksctl ]

   Run the following command to create the IAM role and Kubernetes service account. It attaches the policy to the role, sets the Kubernetes service account with the IAM role ARN, and adds the Kubernetes service account name to the trust policy for the IAM role. 

   ```sh
    USER_ACCOUNT={YOUR_AWS_ACCOUNT_ID}
    CLUSTER_NAME={YOUR_CLUSTER_NAME}
    ROLE_NAME={YOUR_IAM_ROLE_NAME}
    REGION_CODE={YOUR_REGION_CODE}
    eksctl create iamserviceaccount \
       --name efs-csi-controller-sa \
       --namespace kube-system \
       --cluster $CLUSTER_NAME \
       --role-name $ROLE_NAME \
       --attach-policy-arn arn:aws:iam::$USER_ACCOUNT:policy/AmazonEKS_EFS_CSI_Driver_Policy \
       --approve \
       --region $REGION_CODE

    ROLE_ARN=arn:aws:iam::$USER_ACCOUNT:role/$ROLE_NAME
    eksctl create iamserviceaccount \
      --name efs-csi-node-sa \
      --namespace kube-system \
      --cluster $CLUSTER_NAME \
      --attach-role-arn $ROLE_ARN \
      --approve \
      --region $REGION_CODE
   ```

------
#### [ AWS CLI ]

   1. Find you cluster's OIDC provider URL to replace `{YOUR_CLUSTER_NAME}` with your value. If the output returns `None` review the **Prerequisites**.

      ```sh
      aws eks describe-cluster --name {YOUR_CLUSTER_NAME} --query "cluster.identity.oidc.issuer" --output text
      ```

      The example output is as follows.

      ```
      https://oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE
      ```

   1. Create an IAM role that grants the Kubernetes service account the `AssumeRoleWithWebIdentity` action.

      1. Copy the following example to a file named `trust-policy.json`. Replace the following fields `{YOUR_AWS_ACCOUNT_ID}`, `EXAMPLED539D4633E53DE1B71EXAMPLE` and `region-code` with your values. 

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
                   "oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE:sub": [
                        "system:serviceaccount:kube-system:efs-csi-controller-sa",
                        "system:serviceaccount:kube-system:efs-csi-node-sa"
                    ]
                 }
               }
             }
           ]
         }
         ```

      1. Create a IAM role EKS_EFS_CSI_DriverRole 

         ```sh
         aws iam create-role \
           --role-name EKS_EFS_CSI_DriverRole \
           --assume-role-policy-document file://"trust-policy.json"
         ```

   1. Attach the IAM policy to the role. 

      ```sh
      aws iam attach-role-policy \
        --policy-arn arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:policy/AmazonEKS_EFS_CSI_Driver_Policy \
        --role-name EKS_EFS_CSI_DriverRole
      ```

   1. Create a Kubernetes service account with your IAM role ARN.

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
             eks.amazonaws.com/role-arn: arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:role/EKS_EFS_CSI_DriverRole
         ---
         apiVersion: v1
         kind: ServiceAccount
         metadata:
           labels:
             app.kubernetes.io/name: aws-efs-csi-driver
           name: efs-csi-node-sa
           namespace: kube-system
           annotations:
             eks.amazonaws.com/role-arn: arn:aws:iam::{YOUR_AWS_ACCOUNT_ID}:role/EKS_EFS_CSI_DriverRole
         ```

      1. Create the Kubernetes service accounts on your cluster. The Kubernetes service accounts `efs-csi-controller-sa` and `efs-csi-node-sa` are set with the IAM role you created named `EKS_EFS_CSI_DriverRole`.

         ```sh
         kubectl apply -f efs-service-account.yaml
         ```
------

### Enable Direct S3 Read Access

Enabling direct S3 read allows the EFS CSI driver to stream objects directly from your S3 bucket to provide higher throughput. Attach the following IAM policy to your EFS CSI driver's IAM role. Replace `{YOUR_S3_BUCKET_NAME}` with your S3 bucket name. If your cluster is in the AWS GovCloud \(US\-East\) or AWS GovCloud \(US\-West\) AWS Regions, then replace `arn:aws:` with `arn:aws-us-gov:`.

> **Note:** Confirm that S3 bucket policy does not explicitly deny access for this IAM role. An explicit bucket policy deny prevents any permissions granted here. Review your S3 bucket policy in the S3 console or with the `aws s3api get-bucket-policy --bucket {YOUR_S3_BUCKET_NAME}`.

1. Save the following contents to a file named `direct-s3-read-policy.json`.

   ```json
   {
       "Version": "2012-10-17",
       "Statement": [
           {
               "Effect": "Allow",
               "Action": [
                   "s3:GetObject",
                   "s3:GetObjectVersion"
               ],
               "Resource": "arn:aws:s3:::{YOUR_S3_BUCKET_NAME}/*"
           },
           {
               "Effect": "Allow",
               "Action": "s3:ListBucket",
               "Resource": "arn:aws:s3:::{YOUR_S3_BUCKET_NAME}"
           }
       ]
   }
   ```

1. Attach the policy to your EFS CSI driver's IAM role.

   ```sh
   aws iam put-role-policy \
     --role-name EKS_EFS_CSI_DriverRole \
     --policy-name S3DirectReadAccess \
     --policy-document file://direct-s3-read-policy.json
   ```

### Publish efs-utils Logs to CloudWatch

Publishing efs-utils logs to Amazon CloudWatch provides visibility into mount operations and makes troubleshooting or monitoring easier. Attach the AWS managed policy `AmazonElasticFileSystemUtils` to your EFS CSI driver's IAM role.

```sh
aws iam attach-role-policy \
  --role-name EKS_EFS_CSI_DriverRole \
  --policy-arn arn:aws:iam::aws:policy/AmazonElasticFileSystemUtils
```
