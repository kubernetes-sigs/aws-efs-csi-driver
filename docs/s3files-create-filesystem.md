## Create an Amazon S3 file system for Amazon EKS

This topic gives example steps for creating an Amazon S3 file system for Amazon EKS. You can also refer to [Getting started with Amazon S3 Files](https://docs.aws.amazon.com/s3files/latest/userguide/getting-started.html).

The Amazon EFS CSI driver supports [Amazon S3 Files access points](https://docs.aws.amazon.com/s3files/latest/userguide/access-points.html), which are application\-specific entry points into an Amazon S3 file system that make it easier to share a file system between multiple Pods. Access points can enforce a user identity for all file system requests that are made through the access point, and enforce a root directory for each Pod. For more information, see [Amazon S3 Files access points](../examples/kubernetes/s3files/access_points/README.md).

**Important**  
You must complete the following steps in the same terminal because variables are set and used across the steps.

**To create an Amazon S3 file system for your Amazon EKS cluster**

1. Retrieve the VPC ID that your cluster is in and store it in a variable for use in a later step. Replace `my-cluster` with your cluster name.

   ```
   vpc_id=$(aws eks describe-cluster \
       --name my-cluster \
       --query "cluster.resourcesVpcConfig.vpcId" \
       --output text)
   ```

1. Retrieve the CIDR range for your cluster's VPC and store it in a variable for use in a later step. Replace `region-code` with the AWS Region that your cluster is in.

   ```
   cidr_range=$(aws ec2 describe-vpcs \
       --vpc-ids $vpc_id \
       --query "Vpcs[].CidrBlock" \
       --output text \
       --region region-code)
   ```

1. Create a security group with an inbound rule that allows inbound NFS traffic for your Amazon S3 Files mount points.

   1. Create a security group. Replace the *`example values`* with your own.

      ```
      security_group_id=$(aws ec2 create-security-group \
          --group-name MyS3FilesSecurityGroup \
          --description "My S3 Files security group" \
          --vpc-id $vpc_id \
          --output text)
      ```

   1. Create an inbound rule that allows inbound NFS traffic from the CIDR for your cluster's VPC.

      ```
      aws ec2 authorize-security-group-ingress \
          --group-id $security_group_id \
          --protocol tcp \
          --port 2049 \
          --cidr $cidr_range
      ```
**Important**  
To further restrict access to your file system, you can use the CIDR for your subnet instead of the VPC.

1. Create an Amazon S3 file system for your Amazon EKS cluster.

   1. Create a file system. Replace `region-code` with the AWS Region that your cluster is in. Replace `amzn-s3-demo-bucket` with your S3 bucket name. Replace `s3files-role-arn` with IAM Role ARN for Amazon S3 Files to access your bucket.

      ```
      file_system_id=$(aws s3files create-file-system \
          --region region-code \
          --bucket arn:aws:s3:::amzn-s3-demo-bucket \
          --client-token my-cluster \
          --role-arn s3files-role-arn \
          --query 'FileSystemId' \
          --output text)
      ```

   1. Create mount targets.

      1. Determine the IP address of your cluster nodes.

         ```
         kubectl get nodes
         ```

         The example output is as follows.

         ```
         NAME                                         STATUS   ROLES    AGE   VERSION
         ip-192-168-56-0.region-code.compute.internal   Ready    <none>   19m   v1.XX.X-eks-49a6c0
         ```

      1. Determine the IDs of the subnets in your VPC and which Availability Zone the subnet is in.

         ```
         aws ec2 describe-subnets \
             --filters "Name=vpc-id,Values=$vpc_id" \
             --query 'Subnets[*].{SubnetId: SubnetId,AvailabilityZone: AvailabilityZone,CidrBlock: CidrBlock}' \
             --output table
         ```

         The example output is as follows.

         ```
         |                           DescribeSubnets                          |
         +------------------+--------------------+----------------------------+
         | AvailabilityZone |     CidrBlock      |         SubnetId           |
         +------------------+--------------------+----------------------------+
         |  region-codec    |  192.168.128.0/19  |  subnet-EXAMPLE6e421a0e97  |
         |  region-codeb    |  192.168.96.0/19   |  subnet-EXAMPLEd0503db0ec  |
         |  region-codec    |  192.168.32.0/19   |  subnet-EXAMPLEe2ba886490  |
         |  region-codeb    |  192.168.0.0/19    |  subnet-EXAMPLE123c7c5182  |
         |  region-codea    |  192.168.160.0/19  |  subnet-EXAMPLE0416ce588p  |
         +------------------+--------------------+----------------------------+
         ```

      1. Add mount targets for the subnets that your nodes are in. From the output in the previous two steps, the cluster has one node with an IP address of `192.168.56.0`. That IP address is within the `CidrBlock` of the subnet with the ID `subnet-EXAMPLEe2ba886490`. As a result, the following command creates a mount target for the subnet the node is in. If there were more nodes in the cluster, you'd run the command once for a subnet in each AZ that you had a node in, replacing `subnet-EXAMPLEe2ba886490` with the appropriate subnet ID.

         ```
         aws s3files create-mount-target \
             --file-system-id $file_system_id \
             --subnet-id subnet-EXAMPLEe2ba886490 \
             --security-groups $security_group_id
         ```
