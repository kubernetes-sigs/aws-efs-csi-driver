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


## Invalid STS FIPS regional endpoint workaround for non-US and Canada regions
FIPS endpoints are not supported for non-US and Canada regions since it is a US and Canadian government standard. If you have to use FIPS-enabled communication in regions without FIPS endpoints support, we provide a workaround at your own risk:

### Use non-FIPS endpoint in control plane but still enable FIPS in data plane in efs-utils
In `controller-deployment.yaml` and `node-daemonset.yaml`, remove the `AWS_USE_FIPS_ENDPOINT` environment variable and add `FIPS_ENABLED` with value `true`. 

```yaml
# Remove env: 
# -  name: AWS_USE_FIPS_ENDPOINT
#    value: "true"
# Add:
- name: FIPS_ENABLED
  value: "true" 
```

This configuration uses standard (non-FIPS) endpoints for control plane API calls to EFS and STS (avoiding regional FIPS endpoint availability issues) while maintaining full FIPS compliance for data plane mount traffic in [efs-utils](https://github.com/aws/efs-utils?tab=readme-ov-file#enabling-fips-mode) through three layers:
- s2n-tls library in FIPS mode for FIPS-compliant TLS operations
- Security policy restricting TLS handshake negotiation to FIPS-approved algorithms only
- FIPS-validated cryptography module: AWS-LC-FIPS (CSI driver v2.1.15+) or OpenSSL compiled with FIPS-validated libcrypto and FIPS enabled at OS level (CSI driver v2.1.14 or lower versions); runtime switch between FIPS and non-FIPS is not supported for cryptograhy module. 