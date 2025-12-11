# Storage Class Parameters for Dynamic Provisioning
| Parameters            | Values | Default         | Optional | Description                                                                                                                                                                                                                                                                                                                                                                                   |
|-----------------------|--------|-----------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| provisioningMode      | efs-ap |                 | false    | Type of volume provisioned by efs. Currently, Access Points are supported.                                                                                                                                                                                                                                                                                                                    |
| fileSystemId          |        |                 | true*    | File System under which access points are created. See footnote for usage details.                                                                                                                                                                                                                                             |
| fileSystemIdConfigRef |        |                 | true*    | Reference to a ConfigMap containing the filesystem ID in format `namespace/name/key`. See footnote for usage details.                                                                                                                                                                                                           |
| fileSystemIdSecretRef |        |                 | true*    | Reference to a Secret containing the filesystem ID in format `namespace/name/key`. See footnote for usage details.                                                                                                                                                                                                                |
| directoryPerms        |        |                 | false    | Directory permissions for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation.                                                                                                                                                                                                                       |
| uid                   |        |                 | true     | POSIX user Id to be applied for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation.                                                                                                                                                                                                                 |
| gid                   |        |                 | true     | POSIX group Id to be applied for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation.                                                                                                                                                                                                                |
| gidRangeStart         |        | 50000           | true     | Start range of the POSIX group Id to be applied for [Access Point root directory](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-root-directory-access-point) creation. Not used if uid/gid is set.                                                                                                                                                                 |
| gidRangeEnd           |        | 7000000         | true     | End range of the POSIX group Id. Not used if uid/gid is set.                                                                                                                                                                                                                                                                                                                                  |
| basePath              |        |                 | true     | Path under which access points for dynamic provisioning is created. If this parameter is not specified, access points are created under the root directory of the file system                                                                                                                                                                                                                 |
| subPathPattern        |        | `/${.PV.name}`  | true     | The template used to construct the subPath under which each of the access points created under Dynamic Provisioning. Can be made up of fixed strings and limited variables, is akin to the 'subPathPattern' variable on the [nfs-subdir-external-provisioner](https://github.com/kubernetes-sigs/nfs-subdir-external-provisioner) chart. Supports `.PVC.name`,`.PVC.namespace` and `.PV.name` |
| ensureUniqueDirectory |        | true            | true     | **NOTE: Only set this to false if you're sure this is the behaviour you want**.<br/> Used when dynamic provisioning is enabled, if set to true, appends the a UID to the pattern specified in `subPathPattern` to ensure that access points will not accidentally point at the same directory.                                                                                                |
| az                    |        | ""              | true     | Used for cross-account mount. `az` under storage class parameter is optional. If specified, mount target associated with the az will be used for cross-account mount. If not specified, a random mount target will be picked for cross account mount                                                                                                                                          |
| enableZoneConstraints | true/false | false       | true     | When set to `true`, queries the availability zone of the EFS filesystem and returns CSI topology constraints. For One Zone EFS filesystems, this ensures pods are scheduled only in the zone where the filesystem is located. Regional EFS filesystems (multi-AZ) will have no topology constraints. Supports both `volumeBindingMode` values: `Immediate` and `WaitForFirstConsumer`. |
| reuseAccessPoint      |        | false           | true     | When set to true, it creates the Access Point client-token from the provided PVC name. So that the AccessPoint can be replicated from a different cluster if same PVC name and storageclass configuration are used. This feature is currently only supported for a single filesystem per account/region. If attempting to reuse access points across multiple clusters and filesystems within the same region, volume provisioning will fail. If you wish to use the same EFS accesspoint across different clusters for multiple filesystems in a single region, we recommend manually creating the access points and [statically provisioning](https://github.com/kubernetes-sigs/aws-efs-csi-driver/tree/master/examples/kubernetes/access_points) those volumes.                                                                                                                                                                            |

**Note**
* **Filesystem ID Source (marked with \*)**: Exactly one of `fileSystemId`, `fileSystemIdConfigRef`, or `fileSystemIdSecretRef` must be specified to provide the EFS filesystem ID. For detailed usage guide, see the [ConfigMap and Secret Resolution Guide](./filesystem-id-resolution.md).
* Custom Posix group Id range for Access Point root directory must include both `gidRangeStart` and `gidRangeEnd` parameters. These parameters are optional only if both are omitted. If you specify one, the other becomes mandatory.
* When using a custom Posix group ID range, there is a possibility for the driver to run out of available POSIX group Ids. We suggest ensuring custom group ID range is large enough or create a new storage class with a new file system to provision additional volumes. 
* `az` under storage class parameter is not be confused with efs-utils mount option `az`. The `az` mount option is used for cross-az mount or efs one zone file system mount within the same aws account as the cluster.
* Using dynamic provisioning, [user identity enforcement]((https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#enforce-identity-access-points)) is always applied.
 * When user enforcement is enabled, Amazon EFS replaces the NFS client's user and group IDs with the identity configured on the access point for all file system operations.
 * The uid/gid configured on the access point is either the uid/gid specified in the storage class, a value in the gidRangeStart-gidRangeEnd (used as both uid/gid) specified in the storage class, or is a value selected by the driver is no uid/gid or gidRange is specified.
 * We suggest using [static provisioning](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/examples/kubernetes/static_provisioning/README.md) if you do not wish to use user identity enforcement.

---
If you want to pass any other mountOptions to Amazon EFS CSI driver while mounting, they can be passed in through the Persistent Volume or the Storage Class objects, depending on whether static or dynamic provisioning is used. The following are examples of some mountOptions that can be passed:
* **lookupcache**: Specifies how the kernel manages its cache of directory entries for a given mount point. Mode can be one of all, none, pos, or positive. Each mode has different functions and for more information you can refer to this [link](https://linux.die.net/man/5/nfs).
* **iam**: Use the CSI Node Pod's IAM identity to authenticate with Amazon EFS.

### Default Mount Options
When using the EFS CSI driver, be aware that the `noresvport` mount option is enabled by default. This means the client can use any available source port for communication, not just the reserved ports.

### Encryption In Transit
One of the advantages of using Amazon EFS is that it provides [encryption in transit](https://aws.amazon.com/blogs/aws/new-encryption-of-data-in-transit-for-amazon-efs/) support using TLS. Using encryption in transit, data will be encrypted during its transition over the network to the Amazon EFS service. This provides an extra layer of defence-in-depth for applications that requires strict security compliance.

Encryption in transit is enabled by default in the master branch version of the driver. To disable it and mount volumes using plain NFSv4, set the `volumeAttributes` field `encryptInTransit` to `"false"` in your persistent volume manifest. For an example manifest, see the [encryption in transit example](../examples/kubernetes/encryption_in_transit/specs/pv.yaml).

**Note**  
Kubernetes version 1.13 or later is required if you are using this feature in Kubernetes.

-----

# Container Arguments for efs-plugin of efs-csi-node daemonset
| Parameters                  | Values | Default | Optional | Description                                                                                                                                                                                                                             |
|-----------------------------|--------|---------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| vol-metrics-opt-in          |        | false   | true     | Opt in to emit volume metrics.                                                                                                                                                                                                          |
| vol-metrics-refresh-period  |        | 240     | true     | Refresh period for volume metrics in minutes.                                                                                                                                                                                           |
| max-inflight-mount-calls-opt-in   |        | false       | true     | Opt in to use max inflight mount calls limit.                                                                                                                                                                                   |
| max-inflight-mount-calls   |        | -1       | true     | New NodePublishVolume operation will be blocked if maximum number of inflight calls is reached. If maxInflightMountCallsOptIn is true, it has to be set to a positive value.                                                                                                                                                                                  |
| volume-attach-limit-opt-in   |        | false       | true     | Opt in to use volume attach limit.                                                                                                                                                                                   |
| volume-attach-limit   |        |  -1      | true     | Maximum number of volumes that can be attached to a node. If volumeAttachLimitOptIn is true, it has to be set to a positive value.                                                                                                                                                                 |
| force-unmount-after-timeout   |        |  false      | true     | Enable force unmount if normal unmount times out during NodeUnpublishVolume                                                                                                                                                                 |
| unmount-timeout   |        |  30s      | true     | Timeout for unmounting a volume during NodePublishVolume when forceUnmountAfterTimeout is true. If the timeout is reached, the volume will be forcibly unmounted. The default value is 30 seconds.                                                                                                                                                                 |

## Force Unmount After Timeout
The `force-unmount-after-timeout` feature addresses issues when `NodeUnpublishVolume` gets called infinite times and hangs indefinitely due to broken NFS connections. When enabled, if a normal unmount operation exceeds the configured timeout, the driver will forcibly unmount the volume to prevent indefinite hanging and allow the operation to complete.

## Suggestion for setting max-inflight-mount-calls and volume-attach-limit

To prevent out-of-memory (OOM) issues in the efs-plugin container, configure these parameters based on your container's memory limit:

- Each EFS volume consumes **~12 MiB** of memory (for the efs-proxy process)
- Each concurrent mount operation consumes **~30 MiB** of memory during peak usage
  - A single mount operation typically takes **~100 milliseconds** to complete
  - For example, concurrent mount operations can occur when multiple pods are being scheduled simultaneously and need to mount EFS volumes

### Recommended formula
```
Container Memory Limit = ((volume-attach-limit × 12) + (max-inflight-mount-calls × 30)) × 1.5 MiB
```

### Example calculation
- For 50 volumes and 10 concurrent mounts: `((50 × 12) + (10 × 30)) × 1.5 = 1,350 MiB`
- Set container memory limit to at least 1.4 GiB

> **Note:** The 1.5x multiplier provides a safety buffer for other container processes and memory fluctuations.


### Understanding the Impact of vol-metrics-opt-in:
Enabling the vol-metrics-opt-in parameter activates the gathering of inode and disk usage data. This functionality, particularly in scenarios with larger file systems, may result in an uptick in memory usage due to the detailed aggregation of file system information. We advise users with large-scale file systems to consider this aspect when utilizing this feature.


# Container Arguments for deployment(controller) 
| Parameters                  | Values | Default | Optional | Description                                                                                                                                                                                                                                                   |
|-----------------------------|--------|---------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| delete-access-point-root-dir|        | false  | true     | Opt in to delete access point root directory by DeleteVolume. By default, DeleteVolume will delete the access point behind Persistent Volume and deleting access point will not delete the access point root directory or its contents.                       |
| adaptive-retry-mode         |        | true   | true     | Opt out to use standard sdk retry mode for EFS API calls. By default, Driver will use adaptive mode for the sdk retry configuration which heavily rate limits EFS API requests to reduce throttling if throttling is observed.                                |
| tags                         |       |         | true     | Space separated key:value pairs which will be added as tags for Amazon EFS resources. For example, '--tags=name:efs-tag-test date:Jan24'. To include a ':' or ' ' in the tag name or value, use \ as an escape character, for example '--tags="tag\:name:tag\:value" |
