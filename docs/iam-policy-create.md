## Create an IAM policy and role for Amazon EKS

The following steps give an example of using an IAM role for service account to talk to Amazon EFS.

1. Create an IAM policy that allows the CSI driver's service account to make calls to AWS APIs on your behalf.

   1. Download the IAM policy document.

      ```sh
      curl -O https://raw.githubusercontent.com/kubernetes-sigs/aws-efs-csi-driver/master/docs/iam-policy-example.json
      ```

   1. Create the policy. You can change `EKS_EFS_CSI_Driver_Policy` to a different name, but if you do, make sure to change it in later steps too.

      ```sh
      aws iam create-policy \
          --policy-name EKS_EFS_CSI_Driver_Policy \
          --policy-document file://iam-policy-example.json
      ```

1. Create an IAM role and attach the IAM policy to it. Annotate the Kubernetes service account with the IAM role ARN and the IAM role with the Kubernetes service account name. You can create the role using `eksctl` or the AWS CLI.

------
#### [ eksctl ]

   Run the following command to create the IAM role and Kubernetes service account. It also attaches the policy to the role, annotates the Kubernetes service account with the IAM role ARN, and adds the Kubernetes service account name to the trust policy for the IAM role. Replace `my-cluster` with your cluster name and `111122223333` with your account ID. Replace `region-code` with the AWS Region that your cluster is in. If your cluster is in the AWS GovCloud \(US\-East\) or AWS GovCloud \(US\-West\) AWS Regions, then replace `arn:aws:` with `arn:aws-us-gov:`.

   ```sh
   eksctl create iamserviceaccount \
       --cluster my-cluster \
       --namespace kube-system \
       --name efs-csi-controller-sa \
       --attach-policy-arn arn:aws:iam::111122223333:policy/EKS_EFS_CSI_Driver_Policy \
       --approve \
       --region region-code
   ```

------
#### [ AWS CLI ]

   1. Determine your cluster's OIDC provider URL. Replace `my-cluster` with your cluster name. If the output from the command is `None`, review the **Prerequisites**.

      ```sh
      aws eks describe-cluster --name my-cluster --query "cluster.identity.oidc.issuer" --output text
      ```

      The example output is as follows.

      ```
      https://oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE
      ```

   1. Create the IAM role, granting the Kubernetes service account the `AssumeRoleWithWebIdentity` action.

      1. Copy the following contents to a file named `trust-policy.json`. Replace `111122223333` with your account ID. Replace `EXAMPLED539D4633E53DE1B71EXAMPLE` and `region-code` with the values returned in the previous step. If your cluster is in the AWS GovCloud \(US\-East\) or AWS GovCloud \(US\-West\) AWS Regions, then replace `arn:aws:` with `arn:aws-us-gov:`.

         ```
         {
           "Version": "2012-10-17",
           "Statement": [
             {
               "Effect": "Allow",
               "Principal": {
                 "Federated": "arn:aws:iam::111122223333:oidc-provider/oidc.eks.region-code.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE"
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

      1. Create the role. You can change `EKS_EFS_CSI_DriverRole` to a different name, but if you do, make sure to change it in later steps too.

         ```sh
         aws iam create-role \
           --role-name EKS_EFS_CSI_DriverRole \
           --assume-role-policy-document file://"trust-policy.json"
         ```

   1. Attach the IAM policy to the role with the following command. Replace `111122223333` with your account ID. If your cluster is in the AWS GovCloud \(US\-East\) or AWS GovCloud \(US\-West\) AWS Regions, then replace `arn:aws:` with `arn:aws-us-gov:`.

      ```sh
      aws iam attach-role-policy \
        --policy-arn arn:aws:iam::111122223333:policy/EKS_EFS_CSI_Driver_Policy \
        --role-name EKS_EFS_CSI_DriverRole
      ```

   1. Create a Kubernetes service account that's annotated with the ARN of the IAM role that you created.

      1. Save the following contents to a file named `efs-service-account.yaml`. Replace `111122223333` with your account ID. If your cluster is in the AWS GovCloud \(US\-East\) or AWS GovCloud \(US\-West\) AWS Regions, then replace `arn:aws:` with `arn:aws-us-gov:`.

         ```
         ---
         apiVersion: v1
         kind: ServiceAccount
         metadata:
           labels:
             app.kubernetes.io/name: aws-efs-csi-driver
           name: efs-csi-controller-sa
           namespace: kube-system
           annotations:
             eks.amazonaws.com/role-arn: arn:aws:iam::111122223333:role/EKS_EFS_CSI_DriverRole
         ```

      1. Create the Kubernetes service account on your cluster. The Kubernetes service account named `efs-csi-controller-sa` is annotated with the IAM role that you created named `EKS_EFS_CSI_DriverRole`.

         ```sh
         kubectl apply -f efs-service-account.yaml
         ```
------
