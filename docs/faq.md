# Frequently Asked Questions

## Resource limits
The controller container has different memory / CPU requirements based on the workload scale, concurrency, and configurations. When configuring your controller with `delete-access-point-root-dir=true`, we recommend setting higher resource limits if your workload requires many concurrent volume deletions. For example, for a workload that requires 100 concurrent PVC deletions, we recommend setting a minimum CPU limit of 3000m and a minimum memory limit of 2.5 GiB. 

Alternatively, if you would prefer not to allocate these resources to your controller container, we advise lowering concurrency by lowering the `--worker-threads` argument of the [external-provisioner](https://github.com/kubernetes-csi/external-provisioner).

## Timeouts
For most highly concurrent workloads, we recommend increasing the default timeout argument set in the [external-provisioner](https://github.com/kubernetes-csi/external-provisioner) from 15 seconds to 60 seconds. This will avoid provisioning failures due to throttling and resource contention in the controller container. 

## Update Strategy
* Amazon EFS CSI driver uses the `RollingUpdate` strategy and only allows you to configure parameters under [rolling update](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#rolling-update-deployment), such as `maxUnavailable` and `maxSurge`.
* Changing the update strategy (e.g. to `OnDelete`) is not supported.

## Efs-proxy OOMKilled under heavy NFS write workloads

### Symptom
When the `efs-csi-node` pod's memory limit is configured too low for the workload, the `efs-proxy` container may be OOMKilled under sustained NFS write traffic, particularly when multiple mounts share the same node. 

This is a resource configuration issue. Under high write workloads, the kernel accumulates a write backlog that gets flushed into the proxy after it restarts, which can cause repeated OOMKills if the memory limit is not sized accordingly. Increasing the pod's memory limit to match your workload is the recommended solution in most cases.

### Mitigation
- (Recommended) Increase memory limits for the CSI node container to accommodate your workload
- Force unmount the offending mount via the EKS node
- Reduce `wsize` in mount option (1MiB by default) to lower the size of each NFS WRITE RPC, which reduces per-request memory consumption in the proxy buffer
- Use `soft` mount or `timeo`+`retrans` mount options to limit kernel retry behavior

## Using botocore to retrieve mount target ip address when dns name cannot be resolved (Amazon EFS only)
* Amazon EFS CSI driver supports using botocore to retrieve mount target ip address when dns name cannot be resolved, e.g., when user is mounting a file system in another VPC, botocore comes preinstalled on efs-csi-driver which can solve this DNS issue.
* IAM policy prerequisites to use this feature :  
  Allow ```elasticfilesystem:DescribeMountTargets``` and ```ec2:DescribeAvailabilityZones``` actions in your policy attached to the Amazon EKS service account role, refer to example policy [here](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/docs/iam-policy-example.json#L9-L10).


## Invalid STS FIPS regional endpoint workaround for non-US and Canada regions
FIPS endpoints are not supported for non-US and Canada regions since it is a US and Canadian government standard. If you have to use FIPS-enabled communication in regions without FIPS endpoints support, we provide a workaround at your own risk:

### Use non-FIPS endpoint in control plane but still enable FIPS in data plane in efs-utils
In `controller-deployment.yaml`, remove `AWS_USE_FIPS_ENDPOINT` from `env`:
```yaml
# Remove:
# - name: AWS_USE_FIPS_ENDPOINT
#   value: "true"
```

In `node-daemonset.yaml`, remove `AWS_USE_FIPS_ENDPOINT` and add `FIPS_ENABLED` in `env`:
```yaml
# Remove:
# - name: AWS_USE_FIPS_ENDPOINT
#   value: "true"

# Add:
- name: FIPS_ENABLED
  value: "true"
```

#### Helm upgrade example

```bash
helm upgrade --install aws-efs-csi-driver \
  --namespace kube-system \
  --version 4.0.1 \
  --set useFIPS=false \
  --set node.env[0].name=FIPS_ENABLED \
  --set-string node.env[0].value=true \
  aws-efs-csi-driver/aws-efs-csi-driver
```

This configuration uses standard (non-FIPS) endpoints for control plane API calls to EFS and STS (avoiding regional FIPS endpoint availability issues) while maintaining full FIPS compliance for data plane mount traffic in [efs-utils](https://github.com/aws/efs-utils?tab=readme-ov-file#enabling-fips-mode) through three layers:
- s2n-tls library in FIPS mode for FIPS-compliant TLS operations
- Security policy restricting TLS handshake negotiation to FIPS-approved algorithms only
- FIPS-validated cryptography module: AWS-LC-FIPS (CSI driver v2.1.15+) or OpenSSL compiled with FIPS-validated libcrypto and FIPS enabled at OS level (CSI driver v2.1.14 or lower versions); runtime switch between FIPS and non-FIPS is not supported for cryptograhy module. 

## Cross-account: what happens if a mount target's AZ goes down?

It depends on which provisioning mode you used (see the [cross-account mount example](../examples/kubernetes/efs/cross_account_mount/README.md) and [parameters reference](parameters.md#cross-account-provisioning)):

- **DNS mode** (`crossaccount=true` in the secret) — each node resolves a mount target via DNS at mount time, so pods on healthy AZs keep working. Existing pods on the failed AZ lose connectivity until they are rescheduled.
- **Default per-node AZ selection** (no `az`, no `crossaccount`) — the controller resolves all available mount targets at provisioning time. Each node selects its own AZ's mount target; if a node's AZ has no mount target, it falls back to any available one (logged as a warning). Pods running on healthy AZs keep working.
- **Pinned AZ** (StorageClass `az=<zone>`) — every pod uses the mount target in `<zone>`. If that AZ goes down, all pods using this StorageClass lose connectivity.

If you are still on a driver version that baked a single random mount target IP into every PV, upgrade to `v3.1.0` or newer to get the default per-node AZ selection feature.

## Tenant Isolation
Refer to the [EKS Best Practices Guide on Tenant Isolation](https://docs.aws.amazon.com/eks/latest/best-practices/tenant-isolation.html).
