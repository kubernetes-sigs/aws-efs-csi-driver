# Frequently Asked Questions

## Resource limits
The controller container has different memory / CPU requirements based on the workload scale, concurrency, and configurations. When configuring your controller with `delete-access-point-root-dir=true`, we recommend setting higher resource limits if your workload requires many concurrent volume deletions. For example, for a workload that requires 100 concurrent PVC deletions, we recommend setting a minimum CPU limit of 3000m and a minimum memory limit of 2.5 GiB. 

Alternatively, if you would prefer not to allocate these resources to your controller container, we advise lowering concurrency by lowering the `--worker-threads` argument of the [external-provisioner](https://github.com/kubernetes-csi/external-provisioner).

## Timeouts
For most highly concurrent workloads, we recommend increasing the default timeout argument set in the [external-provisioner](https://github.com/kubernetes-csi/external-provisioner) from 15 seconds to 60 seconds. This will avoid provisioning failures due to throttling and resource contention in the controller container. 


## Using botocore to retrieve mount target ip address when dns name cannot be resolved
* Amazon EFS CSI driver supports using botocore to retrieve mount target ip address when dns name cannot be resolved, e.g., when user is mounting a file system in another VPC, botocore comes preinstalled on efs-csi-driver which can solve this DNS issue.
* IAM policy prerequisites to use this feature :  
  Allow ```elasticfilesystem:DescribeMountTargets``` and ```ec2:DescribeAvailabilityZones``` actions in your policy attached to the Amazon EKS service account role, refer to example policy [here](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/docs/iam-policy-example.json#L9-L10).