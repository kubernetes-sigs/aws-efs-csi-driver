# Cluster tag isolation

With `AmazonEFSCSIDriverPolicy`, the drivers of every cluster in an AWS account have the same IAM permissions, so clusters can share file systems and access points. That suits clusters that work together, but it also lets one cluster's driver modify another cluster's access points. This page describes cluster tag isolation: tagging the resources each cluster uses with that cluster's name, and attaching an IAM policy that limits each cluster's driver to its own resources. It also covers how to choose between shared and isolated setups, and how to move an existing cluster to isolation.

The examples use EKS Pod Identity, account `111122223333` and cluster `my-cluster`. The tag-based policies need EKS Pod Identity; for other credentials, see [Other credentials](#other-credentials).

- [Choosing a policy](#choosing-a-policy)
- [Policies](#policies)
- [New clusters](#new-clusters)
- [Existing clusters](#existing-clusters)
- [Cross-account volumes](#cross-account-volumes)
- [What each mismatch does](#what-each-mismatch-does)
- [Settings](#settings)

## Choosing a policy

Clusters in one account can share file systems and access points, or be isolated from each other. Choose one of:

| Setup | IAM policy | `--cluster-tag-isolation` | Fits |
|---|---|---|---|
| Shared across every cluster in the account | [`AmazonEFSCSIDriverPolicy`](#amazonefscsidriverpolicy-file-systems-shared-between-clusters) | `disable` | File systems shared between clusters, and access points shared between clusters with `reuseAccessPoint`. |
| Shared between specific clusters | A custom policy based on the tag-based policy | `disable` | Clusters that share file systems with each other but not with the rest of the account. |
| Isolated | [The tag-based policy](#the-tag-based-policy-recommended-for-new-clusters) | `enforce` | Each cluster uses only its own file systems and access points. |

The tag-based policy is not a fit when two or more clusters share a file system or an access point: a resource carries only one cluster's tag value, and in `enforce` the controller refuses access points tagged for another cluster. For a custom policy, start from the tag-based policy and widen its file system conditions to the clusters that share them, and keep `--cluster-tag-isolation=disable`.

## Policies

### The tag-based policy: recommended for new clusters

The tag-based policies are recommended for new clusters. They check the cluster tag key `eks:eks-cluster-name` against the `eks-cluster-name` session tag that EKS Pod Identity adds, so start the controller with `--cluster-tag-key=eks:eks-cluster-name`. IAM cannot use a policy variable in a condition key, so the key is written into the policies.

| Role | Policy |
|---|---|
| Controller | [iam-policy-cluster-tag-controller-example.json](./iam-policy-cluster-tag-controller-example.json) |
| Node | [iam-policy-cluster-tag-node-example.json](./iam-policy-cluster-tag-node-example.json) |
| Controller, only while `backfill` | [iam-policy-cluster-tag-controller-backfill-example.json](./iam-policy-cluster-tag-controller-backfill-example.json) |

The controller policy goes on the controller role (`efs-csi-controller-sa`), and the node policy on the node role (`efs-csi-node-sa`). The backfill policy is described in [Existing clusters](#existing-clusters).

Each file is valid JSON that you can attach as it is. With them, as far as the EFS and S3 Files APIs allow, the driver of a cluster can only:

- create access points on file systems tagged for its own cluster, and only with its own cluster tag on the new access point;
- delete access points tagged for its own cluster;
- mount (`ClientMount`, `ClientWrite`, `ClientRootAccess`) file systems tagged for its own cluster. The node role needs this for the `iam` mount option, and the controller needs it for `--delete-access-point-root-dir`.

The policies never allow changing or removing a tag after create time. The only exception is the backfill policy.

**EKS Pod Identity.** The Pod Identity policies use the `eks-cluster-name` and `eks-cluster-arn` session tags that [EKS Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-abac.html) adds to the driver's credentials. Credentials without these tags, such as IRSA, are denied every call.

#### Other credentials

The tag-based policies need the session tags that EKS Pod Identity adds. IAM roles for service accounts (IRSA), node instance profiles and other credentials add none, so every call is denied. With those credentials, use [`AmazonEFSCSIDriverPolicy`](#amazonefscsidriverpolicy-file-systems-shared-between-clusters), or write your own policy from the tag-based one: for example, read the cluster name from a tag on the IAM role (`aws:PrincipalTag/<tag>`) instead of the `eks-cluster-name` session tag, check a resource tag key of your choice, and set `--cluster-tag-key` to that key. Add `"Null": {"aws:PrincipalTag/<tag>": "false"}` to every statement, so a role without the tag gets nothing.

#### Roles per cluster

With EKS Pod Identity, one controller role and one node role can serve every cluster in the account and still keep them isolated. The Pod Identity trust policy names no cluster, and EKS sets the `eks-cluster-name` and `eks-cluster-arn` session tags for whichever cluster's driver assumes the role, so each driver only reaches its own cluster's resources. Create the roles once, and repeat only the pod identity associations and the Helm settings for each cluster.

#### Scope

**Region.** Every statement of the Pod Identity policies has the condition `"StringLike": {"aws:PrincipalTag/eks-cluster-arn": "arn:*:eks:${aws:RequestedRegion}:*:cluster/*"}`, so the driver can only call AWS in the region of its own cluster. The driver makes no global or fixed-region calls. The one exception is `sts:AssumeRole` for the StorageClass secret `awsRoleArn`, and the policies don't grant it.

**Account and partition.** ARNs use `*` for the partition, region and account. Access is limited by the tags, not by the account in the ARN. For `s3:GetObject` (S3 Files direct reads), where a bucket policy can grant access across accounts, the node policies also require `"aws:ResourceAccount": "${aws:PrincipalAccount}"`.

**Actions that cannot be scoped by tag.** These actions still reach resources of other clusters:

| Action | Role | What it can still reach |
|---|---|---|
| `elasticfilesystem:DescribeAccessPoints`, `DescribeFileSystems`, `DescribeMountTargets` | controller, node (mount targets only) | Read-only: metadata and tags of every file system and access point in the region. They have no tag condition, because GID allocation must see every access point on a file system. |
| `s3files:ListAccessPoints`, `ListFileSystems`, `GetAccessPoint`, `GetFileSystem` | controller | Read-only: every S3 Files file system and access point in the region. |
| `ec2:DescribeAvailabilityZones` | controller, node | Read-only, and it has no resource type. |
| `elasticfilesystem:TagResource`, `s3files:TagResource` at create time | controller | These are evaluated against the file system and check only the requested cluster tag. The create action itself still requires the file system's cluster tag. |
| `logs:CreateLogGroup`, `PutRetentionPolicy`, `CreateLogStream`, `DescribeLogStreams`, `PutLogEvents` | node | The account-wide `/aws/efs/utils` log group that efs-utils writes to. It is not tagged, so a node can append to other clusters' log streams in that group. |
| `cloudwatch:PutMetricData` | node | Any metric in the `efs-utils/EFS` and `efs-utils/S3Files` namespaces. |
| `s3:GetObject` | node | Any object in any bucket of the account. Replace `arn:*:s3:::*/*` with your S3 Files bucket ARNs where you can. |
| `elasticfilesystem:TagResource`, `s3files:TagResource` in the backfill policy | controller | Any access point in the region that carries `efs.csi.aws.com/cluster=true` and no cluster tag. See [The backfill policy](#the-backfill-policy). |

**File systems need an administrator.** The driver never creates file systems. An administrator must tag every file system that a cluster uses with the cluster tag (`eks:eks-cluster-name=<cluster name>`). Otherwise `CreateVolume` fails with `AccessDenied`, and mounts with the `iam` option fail.

**Node mounts.** The node policy only takes effect for volumes mounted with the `iam` mount option, on file systems whose [file system policy](https://docs.aws.amazon.com/efs/latest/ug/iam-access-control-nfs-efs.html) does not grant access to every client. Without a file system policy, EFS allows any client that can reach a mount target.

### The backfill policy

The backfill policy is attached to the controller role together with the controller policy, only while `--cluster-tag-isolation=backfill` is set. It grants exactly one thing: `elasticfilesystem:TagResource` and `s3files:TagResource` on an access point that carries `efs.csi.aws.com/cluster=true` (this driver created it) and no cluster tag yet, with a request that contains only the cluster tag, set to the caller's own cluster name. Detach it after you set `enforce`.

`efs.csi.aws.com/cluster=true` does not name the cluster. While the backfill policy is attached, any cluster in the account whose role has it could add its own cluster tag to another cluster's untagged access point. The controller itself only tags access points that back its own PersistentVolumes. Keep the backfill window short, or tag the access points yourself (see [How to confirm backfill is complete](#how-to-confirm-backfill-is-complete)).

### `AmazonEFSCSIDriverPolicy`: file systems shared between clusters

The AWS managed policy `AmazonEFSCSIDriverPolicy` (with `AmazonS3FilesCSIDriverPolicy` for S3 Files) and the example policy [iam-policy-example.json](./iam-policy-example.json), described in [Create IAM roles](./iam-policy-create.md), keep working. Use them when two or more clusters must provision access points on the same file system. The tag-based policies can't do that, because a file system carries only one cluster's tag value.

These policies only check the `efs.csi.aws.com/cluster` tag on access points, which every cluster's driver writes with the value `true`. They check no tag on file systems, so the driver can create access points on any file system in the account, tagged or not. They do not isolate clusters: they grant one cluster's controller IAM permissions on other clusters' resources in the same account. For example, the controller of cluster `team-a` has IAM permissions to delete the dynamically provisioned access points of cluster `team-b` (`elasticfilesystem:DeleteAccessPoint` is allowed for any access point with `efs.csi.aws.com/cluster`) and to create access points on `team-b`'s file systems.

`AmazonEFSCSIDriverPolicy` also allows no tag key other than `efs.csi.aws.com/cluster` on `CreateAccessPoint`. Do not set `--cluster-tag-key`, `--extra-tags` or `--tags` with it: access point creation fails with `AccessDenied` (see [Custom tags require additional IAM permissions](./install.md#custom-tags-require-additional-iam-permissions)).

### Isolation covers the whole account

A cluster's resources are protected only when the driver role of every cluster in the account uses the tag-based policies. A cluster whose driver still uses `AmazonEFSCSIDriverPolicy` has IAM permissions to modify other clusters' access points, including those of clusters that use the tag-based policies.

### Trust policy

Use the standard [EKS Pod Identity trust policy](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-role.html).

## New clusters

No backfill is needed, because every access point is tagged at create time.

1. Tag each file system the cluster uses:

   ```sh
   aws efs tag-resource --resource-id fs-0123456789abcdef0 --tags Key=eks:eks-cluster-name,Value=my-cluster
   ```

1. Attach the controller and node policies from [the table above](#the-tag-based-policy-recommended-for-new-clusters) to the driver roles, and associate the roles with the service accounts ([Set up driver permission](./install.md#set-up-driver-permission)).

1. Install the driver with the cluster name, the key and `enforce`:

   ```sh
   helm upgrade --install aws-efs-csi-driver aws-efs-csi-driver/aws-efs-csi-driver --namespace kube-system \
     --set controller.clusterName=my-cluster \
     --set controller.clusterTagKey=eks:eks-cluster-name \
     --set controller.clusterTagIsolation=enforce
   ```

   Without Helm, set these controller arguments:

   ```
   --cluster-name=my-cluster --cluster-tag-key=eks:eks-cluster-name --cluster-tag-isolation=enforce
   ```

1. Use the `iam` mount option in your StorageClasses or PersistentVolumes (`mountOptions: [tls, iam]`), and give each file system a file system policy, so that the node policy takes effect.

Do not set `enableTagging: "false"` in a StorageClass. When `--cluster-name` and `--cluster-tag-key` are set, the controller rejects such a request with `InvalidArgument`, because the access point would be created without the cluster tag.

## Existing clusters

Access points created before the cluster tag was configured carry only `efs.csi.aws.com/cluster=true`. Set `--cluster-tag-isolation` to match the policy attached to the controller:

| State | IAM attached to the controller | Controller behavior |
|---|---|---|
| `disable` (default) | `AmazonEFSCSIDriverPolicy` or [iam-policy-example.json](./iam-policy-example.json) | No isolation, plus the cluster tag on new access points when `--cluster-name` and `--cluster-tag-key` are set. |
| `backfill` | Tag-based policy and backfill policy, alongside `AmazonEFSCSIDriverPolicy` during the rollout | Requires `--cluster-name` and `--cluster-tag-key`. Tags existing access points this cluster owns, at startup and every hour. Does not filter discovery. |
| `enforce` | Tag-based policy (and `AmazonEFSCSIDriverPolicy` until the last rollout step) | Requires `--cluster-name` and `--cluster-tag-key`. Ignores discovered access points that lack this cluster's tag value. Does not backfill. |

The cluster is isolated only once `AmazonEFSCSIDriverPolicy` is detached. IAM allows an action when any attached policy allows it, so while both are attached, the managed policy still allows what it always did.

In `backfill`, the controller sweeps every access point and file system referenced by its EFS PersistentVolumes and by the `fileSystemId` of its StorageClasses:

- It owns an access point that backs a PersistentVolume it provisioned, carries `efs.csi.aws.com/cluster=true`, and comes from a StorageClass without `reuseAccessPoint`. It adds `<cluster-tag-key>=<cluster name>` to each such access point in a `TagResource` request that contains only that tag.
- It does not tag anything else, and lists it in `needsAdministratorResources` on the sweep summary line:
  - file systems;
  - access points of statically provisioned volumes;
  - access points created with `enableTagging: "false"`;
  - access points of StorageClasses with `reuseAccessPoint: "true"`, or whose StorageClass no longer exists. Other clusters can reuse these access points, so the controller cannot tell whether this cluster owns them;
  - resources tagged for another cluster;
  - resources the controller's own credentials cannot find, for example because they were deleted while a PersistentVolume still references them.

The sweep leaves [cross-account volumes](#cross-account-volumes) out entirely.

In `enforce`, the controller refuses to reuse (`reuseAccessPoint`) or adopt an existing access point that is not tagged for its cluster, and never removes the root directory (`--delete-access-point-root-dir`) of such an access point. These requests fail with `FailedPrecondition`, and the controller logs `ignoring resource not tagged for this cluster` at info level with `resource`, `tagKey`, `clusterName` and `tagValue` (the value found, empty when the access point has no cluster tag).

The value of `--cluster-tag-isolation` (`controller.clusterTagIsolation`) is a string and must be exactly `disable`, `backfill` or `enforce`; an empty value means `disable`. The controller and the chart reject any other value. At startup it logs one line with its settings: `cluster tag isolation settings`, with `clusterName`, `tagKey`, `clusterTagIsolation` and, in `backfill`, `backfillInterval`.

Roll out in this order. `CreateVolume` keeps working at every step:

1. Tag each file system the cluster uses with `eks:eks-cluster-name=<cluster name>`, as in [New clusters](#new-clusters).
1. Attach the tag-based controller policy and the backfill policy to the controller role, alongside `AmazonEFSCSIDriverPolicy` (or your copy of [iam-policy-example.json](./iam-policy-example.json)). The backfill policy is needed even with the managed policy attached, because `AmazonEFSCSIDriverPolicy` allows `TagResource` only when an access point is created.
1. Set `backfill`:

   ```sh
   helm upgrade aws-efs-csi-driver aws-efs-csi-driver/aws-efs-csi-driver --namespace kube-system --reuse-values \
     --set controller.clusterName=my-cluster \
     --set controller.clusterTagKey=eks:eks-cluster-name \
     --set controller.clusterTagIsolation=backfill
   ```

   Without Helm: `--cluster-name=my-cluster --cluster-tag-key=eks:eks-cluster-name --cluster-tag-isolation=backfill`.

1. Wait until every controller replica has logged `cluster tag backfill complete`, and tag every resource listed in `needsAdministratorResources` (see [How to confirm backfill is complete](#how-to-confirm-backfill-is-complete)). The controller checks again every hour; restart the controller to check immediately.
1. Set `controller.clusterTagIsolation=enforce` (`--cluster-tag-isolation=enforce`).
1. Detach `AmazonEFSCSIDriverPolicy` (or your copy of iam-policy-example.json) and the backfill policy. The cluster is isolated from this step on. Then replace the node role's policies with the tag-based node policy.

From step 3 on, new access points carry the cluster tag, which `AmazonEFSCSIDriverPolicy` does not allow; the tag-based policy allows the create only on file systems tagged in step 1.

If you change `--cluster-tag-key`, for example to move to your own policy, the cluster's resources still carry the old key. Run the backfill again with the new key and policies before you set `enforce`.

### How to confirm backfill is complete

Each controller replica runs its own sweep and logs exactly one info line per sweep, `cluster tag backfill sweep finished`:

| Field | Meaning |
|---|---|
| `tagged` | `TagResource` calls that succeeded in this sweep. |
| `failed` | `TagResource` calls that failed in this sweep. |
| `needsAdministrator` | Distinct resources this cluster uses that an administrator must tag. |
| `remaining` | Resources, or whole listings, the sweep could not check: the tags of a resource could not be read, or listing PersistentVolumes or StorageClasses failed (counts 1). |
| `needsAdministratorResources` | The first 20 resource IDs counted in `needsAdministrator`, with `needsAdministratorTotal` when there are more. Omitted when empty. |
| `failedResources` | The first 20 resource IDs counted in `failed`, with `failedTotal` when there are more. Omitted when empty. |
| `firstError` | The first error of the sweep. Only present when `failed` or `remaining` is above zero. |

The lines for single resources (`added cluster tag to existing resource`, `failed to add cluster tag to existing resource` and `resource needs cluster tag from an administrator`, with `resource` and `tagKey`) are logged only at verbosity 4 (`controller.logLevel: 4`).

`cluster tag backfill complete` is logged once per controller process, in each replica, after the first sweep that ends with `failed`, `needsAdministrator` and `remaining` all zero. Later sweeps still log their summary line, and never log `complete` again. Backfill is complete when every replica has logged it. To read the summary and completion lines of every replica:

```sh
kubectl -n kube-system logs -l app=efs-csi-controller -c efs-plugin --prefix --tail=-1 |
  grep -E 'cluster tag backfill (complete|sweep finished)'
```

`--tail=-1` is needed because `kubectl logs -l` prints only the last 10 lines of each pod by default, and `--prefix` shows which pod each line comes from. If a summary line has `needsAdministratorTotal`, more than 20 resources are waiting: tag the listed ones and wait for the next sweep, set `controller.logLevel: 4` to log each one as `resource needs cluster tag from an administrator`, or use the check below.

These log lines come from a test rollout. They were captured with an earlier log format and are shown here in the current one. The first sweep tagged one access point and reported an access point and a file system for an administrator:

```
I1021 21:52:50.996402       1 cluster_tag.go:96] "cluster tag isolation settings" clusterName="my-cluster" tagKey="eks:eks-cluster-name" clusterTagIsolation="backfill" backfillInterval="1h0m0s"
I1021 21:52:51.664817       1 cluster_tag_backfill.go:235] "cluster tag backfill sweep finished" tagged=1 failed=0 needsAdministrator=2 remaining=0 needsAdministratorResources=["fsap-031a1805db3b0b528","fs-02ecc0fd7fac01bd6"]
```

When the backfill policy is not attached, the `TagResource` call fails, and the summary line shows it in `failed`, `failedResources` and `firstError`:

```
I1021 21:52:07.026966       1 cluster_tag_backfill.go:235] "cluster tag backfill sweep finished" tagged=0 failed=1 needsAdministrator=2 remaining=0 needsAdministratorResources=["fsap-031a1805db3b0b528","fs-02ecc0fd7fac01bd6"] failedResources=["fsap-04d74ca338925f3c0"] firstError="Access denied: operation error EFS: TagResource, https response error StatusCode: 403, RequestID: 51ac3921-d0d7-4d96-a474-81f9f970830d, api error AccessDeniedException: User: arn:aws:sts::111122223333:assumed-role/EKS_EFS_CSI_ControllerRole/eks-my-cluster-efs-csi-co-61b1b2ee-008c-4b9c-ba5c-d63754b3cf09 is not authorized to perform: elasticfilesystem:TagResource on the specified resource"
```

After the administrator tagged the reported resources, the next sweep logged completion:

```
I1021 21:53:26.933217       1 cluster_tag_backfill.go:235] "cluster tag backfill sweep finished" tagged=0 failed=0 needsAdministrator=0 remaining=0
I1021 21:53:26.933241       1 cluster_tag_backfill.go:238] "cluster tag backfill complete" clusterName="my-cluster" tagKey="eks:eks-cluster-name"
```

The commands below use two shell variables. Set `TAG_KEY` to the `--cluster-tag-key` value:

```sh
TAG_KEY=eks:eks-cluster-name
CLUSTER=my-cluster
```

To list the resources that still need tagging, check every file system and access point referenced by the cluster's EFS and S3 Files PersistentVolumes, and print only those without `$TAG_KEY=$CLUSTER`, with the value found (`None` when the resource has no cluster tag). Empty output means every resource is tagged; `describe failed` means the call failed, for example because the resource was deleted. The IDs are described one at a time, because a single missing ID fails the whole call. The S3 Files API returns tags as lowercase `key` and `value`, so its queries differ from the EFS ones. File systems that are named only in a StorageClass `fileSystemId` are not listed; the backfill sweep reports those.

```sh
kubectl get pv -o jsonpath='{range .items[?(@.spec.csi.driver=="efs.csi.aws.com")]}{.spec.csi.volumeHandle}{"\n"}{end}' |
  awk -F: '{t = "efs"; if ($1 == "efs" || $1 == "s3files") { t = $1; $0 = substr($0, length($1) + 2) }
    sub(/\..*/, "", $1); print t, $1; if ($3 != "") print t, $3}' |
  sort -u | while read -r type id; do
    efs_missing="[?!(Tags[?Key=='$TAG_KEY' && Value=='$CLUSTER'])]"
    efs_value="Tags[?Key=='$TAG_KEY']|[0].Value"
    s3files_missing="[?!(tags[?key=='$TAG_KEY' && value=='$CLUSTER'])]"
    s3files_value="tags[?key=='$TAG_KEY']|[0].value"
    case "$type:$id" in
      efs:fsap-*) aws efs describe-access-points --access-point-id "$id" --output text \
        --query "AccessPoints$efs_missing.[AccessPointId, $efs_value]" ;;
      efs:*) aws efs describe-file-systems --file-system-id "$id" --output text \
        --query "FileSystems$efs_missing.[FileSystemId, $efs_value]" ;;
      s3files:fsap-*) aws s3files get-access-point --access-point-id "$id" --output text \
        --query "[@]$s3files_missing.['$id', $s3files_value]" ;;
      s3files:*) aws s3files get-file-system --file-system-id "$id" --output text \
        --query "[@]$s3files_missing.['$id', $s3files_value]" ;;
    esac || printf '%s\tdescribe failed\n' "$id"
  done
```

A resource that already carries another cluster's value is shared with, or belongs to, that cluster: decide which cluster owns it before you change the tag. Administrators tag resources with `aws efs tag-resource` (EFS) or `aws s3files tag-resource` (S3 Files):

```sh
aws efs tag-resource --resource-id fsap-0123456789abcdef0 --tags "Key=$TAG_KEY,Value=$CLUSTER"
aws s3files tag-resource --resource-id fs-0123456789abcdef0 --tags "key=$TAG_KEY,value=$CLUSTER"
```

## Cross-account volumes

A StorageClass whose provisioner secret sets `awsRoleArn` provisions access points in another account: for that StorageClass, the controller assumes the role and makes every EFS call in the other account (see [cross-account mount](../examples/kubernetes/efs/cross_account_mount/README.md)). Only EFS supports this. Isolation in the other account is up to that account's IAM policy for the role, so cluster tag isolation leaves these volumes out:

- Access points created through the role do not get the cluster tag, in any `--cluster-tag-isolation` state, so the role's policy needs no extra tag key. `enableTagging: "false"` stays allowed on these StorageClasses.
- `enforce` does not check their tags: reusing an access point and removing its root directory work as they do in `disable`.
- The backfill sweep skips PersistentVolumes and file systems of StorageClasses that set `csi.storage.k8s.io/provisioner-secret-name`, so they are neither tagged nor reported, and do not keep `cluster tag backfill complete` from appearing.

The controller role in the cluster's account still needs `sts:AssumeRole` on the other account's role. The tag-based policies do not grant it; keep the statement from the cross-account example when you replace `AmazonEFSCSIDriverPolicy`.

## What each mismatch does

The controller cannot check which policy is attached:

| Setting | Policy attached | Result |
|---|---|---|
| `backfill` | Backfill policy missing | Every `TagResource` call fails: each sweep's summary line counts the legacy access points in `failed`, lists them in `failedResources`, and shows `AccessDeniedException` in `firstError`. `cluster tag backfill complete` never appears. New access points are still created with the cluster tag. |
| `enforce` | Backfill policy still attached | No effect on the controller, which does not backfill in `enforce`. The role can still tag untagged access points of other clusters; detach the policy. |
| `backfill` or `enforce` | `AmazonEFSCSIDriverPolicy` or iam-policy-example.json only | With [iam-policy-example.json](./iam-policy-example.json) or another policy that allows extra tag keys, everything works, but nothing is isolated. With `AmazonEFSCSIDriverPolicy`, `CreateVolume` fails with `AccessDenied`, because that policy allows no tag key other than `efs.csi.aws.com/cluster`. |
| `backfill` or `enforce` | Tag-based policy alongside `AmazonEFSCSIDriverPolicy` | Everything works on file systems tagged for the cluster, but nothing is isolated until the managed policy is detached. |
| `backfill` or `enforce` | Tag-based policy, file system not tagged for the cluster | `CreateVolume` on that file system fails with `AccessDenied`, because the tag-based policy allows `CreateAccessPoint` only on file systems tagged for the caller's cluster. |
| `disable` | Tag-based policy without the cluster name set | `CreateVolume` fails with `AccessDenied`, because new access points lack the cluster tag. Deleting access points that are not tagged for the cluster also fails with `AccessDenied`. |
| Any | `--cluster-tag-key` is not `eks:eks-cluster-name` | `CreateVolume` fails with `AccessDenied`, because the request carries a different tag key than the policy checks. |
| Any | Tag-based policy, credentials without Pod Identity session tags (IRSA, instance profile) | Every call is denied. |
| Any | Tag-based policy, `--cluster-name` differs from the EKS cluster name | `CreateVolume` fails with `AccessDenied`, and every backfill `TagResource` call fails. |

## Settings

| Flag | Helm value | Default | Description |
|---|---|---|---|
| `--cluster-name` | `controller.clusterName` | empty | Name of the Kubernetes cluster. With `--cluster-tag-key`, every access point the controller creates is tagged `<cluster-tag-key>=<cluster-name>`, except [cross-account](#cross-account-volumes) ones. With the tag-based policies, it must be the EKS cluster name. |
| `--cluster-tag-key` | `controller.clusterTagKey` | empty | Tag key whose value is `--cluster-name`. With `--cluster-name`, it is added to every access point the controller creates (except [cross-account](#cross-account-volumes) ones), in every `--cluster-tag-isolation` state. The tag-based policies use `eks:eks-cluster-name`. It cannot be `efs.csi.aws.com/cluster` or start with `aws:`. |
| `--cluster-tag-isolation` | `controller.clusterTagIsolation` | `disable` | `disable`, `backfill` or `enforce`, as described in [Existing clusters](#existing-clusters). `backfill` and `enforce` fail at startup without `--cluster-name` and `--cluster-tag-key`. In `disable`, setting only one of the two logs `only one of --cluster-name and --cluster-tag-key is set; no cluster tag is written` at startup. |
| `--extra-tags` | `controller.extraTags` (map) | empty | Tags (`k1=v1,k2=v2`) added to every AWS resource the controller creates. Not used to find, filter or isolate resources, and not the cluster tag. Overrides no other tag source and is not overridden: it is merged with `--tags`, and a key that `--tags` sets to a different value, or that equals `efs.csi.aws.com/cluster` or `--cluster-tag-key`, fails at startup. |
| `--tags` (deprecated) | `controller.tags` (map) | empty | Deprecated: use `--extra-tags`. Space-separated `key:value` pairs. It still works, logs a deprecation warning at startup, and overrides the default `efs.csi.aws.com/cluster` tag. It cannot set `--cluster-tag-key`. |

The driver takes no environment variables for these settings. The node DaemonSet needs none of them.
