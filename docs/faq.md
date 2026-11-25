# Frequently Asked Questions

## Resource limits
The controller container has different memory / CPU requirements based on the workload scale, concurrency, and configurations. When configuring your controller with `delete-access-point-root-dir=true`, we recommend setting higher resource limits if your workload requires many concurrent volume deletions. For example, for a workload that requires 100 concurrent PVC deletions, we recommend setting a minimum CPU limit of 3000m and a minimum memory limit of 2.5 GiB. 

Alternatively, if you would prefer not to allocate these resources to your controller container, we advise lowering concurrency by lowering the `--worker-threads` argument of the [external-provisioner](https://github.com/kubernetes-csi/external-provisioner).

## Timeouts
For most highly concurrent workloads, we recommend increasing the default timeout argument set in the [external-provisioner](https://github.com/kubernetes-csi/external-provisioner) from 15 seconds to 60 seconds. This will avoid provisioning failures due to throttling and resource contention in the controller container. 

## Update Strategy
* The node DaemonSet uses the `RollingUpdate` strategy by default. Set [rolling update](https://kubernetes.io/docs/tasks/manage-daemon/update-daemon-set/) parameters such as `maxUnavailable` and `maxSurge` with `node.rollingUpdate`.
* To use another strategy, set the full DaemonSet `updateStrategy` with `node.updateStrategy`, for example `type: OnDelete`. `node.updateStrategy` and `node.rollingUpdate` cannot both be set.
* With `OnDelete`, a change to the node pod spec, such as new resource requests or a new image, does not restart running `efs-csi-node` pods. A node gets the new pod only when its `efs-csi-node` pod is deleted, for example when the node is replaced. This keeps existing mounts running, and nodes provisioned afterwards are sized for the new requests. Nodes that are not replaced keep running the old version.
* Chart versions 3.4.1 and older also had `node.updateStrategy`, with the same meaning. Chart 4.0.0 removed it, and chart versions 4.0.0 to 4.6.0 ignore it. If your values file still contains `node.updateStrategy` from an older chart, it takes effect again after an upgrade to this version. Remove it, or check that it is the strategy you want, before you upgrade. If it is set together with `node.rollingUpdate`, the upgrade fails with an error.

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

## fsGroup in securityContext is not respected — UID/GID is set only by the access point

**A pod's `securityContext` does not set the ownership of a dynamically provisioned
(`efs-ap`) volume.** The volume root is owned by the UID/GID on its access point,
which the driver takes from the StorageClass `uid`/`gid` parameters or allocates
from `gidRangeStart`/`gidRangeEnd`. `fsGroup` and `runAsUser` are not consulted.
This is a property of access points, not a bug; see upstream issue
[#1202](https://github.com/kubernetes-sigs/aws-efs-csi-driver/issues/1202).

With this StorageClass and pod:

```yaml
# StorageClass
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-0123456789abcdef0
  directoryPerms: "700"
  gidRangeStart: "1000"
  gidRangeEnd: "2000"
---
# Pod
securityContext:
  fsGroup: 1035
  runAsUser: 1035
```

the volume root is owned by the GID the driver allocated, not by `1035`:

```
$ ls -ld /var/lib/postgresql/data
drwx------ 2 1000 1000 6144 Jan  1 00:00 /var/lib/postgresql/data
```

This is not a permissions failure:
[user identity enforcement](https://docs.aws.amazon.com/efs/latest/ug/enforce-identity-access-points.html)
replaces the client's UID/GID with the access point's own, so access does not depend
on the UID the pod runs as. What breaks is applications that check directory
ownership before starting. For example, PostgreSQL exits with
`data directory "..." has wrong ownership`.

**To pin ownership, set `uid` and `gid` on the StorageClass:**

```yaml
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-0123456789abcdef0
  directoryPerms: "700"
  uid: "1035"
  gid: "1035"
```

The access point is then created as `1035:1035`, and a pod running as `1035` sees
the ownership it expects. `uid`/`gid` take precedence over `gidRangeStart`/
`gidRangeEnd`, which the driver does not consult when both are set. EFS applies
this ownership only when it creates the root directory; a directory that already
exists keeps its current owner, so `uid`/`gid` will not change it.

`uid`/`gid` are fixed values on a cluster-scoped object and are not templated, so
workloads that run as different UIDs need one StorageClass per UID. If you need
ownership to vary per pod, use
[static provisioning](../examples/kubernetes/efs/static_provisioning/README.md)
without an access point: nothing then overrides the client identity, so the pod's
own UID reaches EFS and an init container can `chown` the directory. This trades
away isolation — there is no access point to confine clients to a subdirectory or
to enforce an identity. Use it only within a single trust boundary, and see
[Tenant Isolation](#tenant-isolation) below.

## Tenant Isolation
Refer to the [EKS Best Practices Guide on Tenant Isolation](https://docs.aws.amazon.com/eks/latest/best-practices/tenant-isolation.html).
