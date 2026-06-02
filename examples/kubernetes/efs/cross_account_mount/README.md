# Cross-account EFS mount

This example shows how an EKS cluster in AWS account `A` can mount an EFS file system that lives in account `B`. It covers both **dynamic provisioning** and **static provisioning**, driven by a small set of provisioner-secret keys, StorageClass parameters, and PV `volumeAttributes`.

**Note**: requires Kubernetes v1.17+ and driver version >= 1.8.0.

## How the driver picks a mount target

When the provisioner secret carries `awsRoleArn`, `CreateVolume` chooses one of three branches based on the secret's `crossaccount` key and the StorageClass `az` parameter:

| Trigger | PV `volumeAttributes` written | Mount-time behavior |
|---------|-------------------------------|---------------------|
| Secret has `crossaccount=true` | `crossaccount: "true"` | Each node uses [efs-utils DNS resolution](https://github.com/aws/efs-utils?tab=readme-ov-file#crossaccount-option-prerequisites) to mount a mount target in its own AZ. No mount target IP is baked into the PV. |
| StorageClass has `az=<zone>` (and `crossaccount` unset/`false`) | `mounttargetip: <ip>` | Every node uses the single mount target IP baked into the PV. If no mount target exists in the specified AZ, the controller falls back to a random available mount target. |
| Neither set (default) | (internal AZ→IP mapping) | Each node selects the mount target in its own AZ; if absent, falls back to any available mount target (with a warning logged). |

For static provisioning, use `crossaccount: "true"` or `mounttargetip: <ip>` in the PV's `volumeAttributes`.

> Older driver versions baked a single random mount-target IP into the PV regardless of the node's AZ, which made every workload depend on one AZ even in a multi-AZ deployment. Mode A (`crossaccount=true`) and Mode C (default) avoid that.

## Shared prerequisites

Perform these once before using any mode below. EKS cluster lives in account `A`, EFS file system in account `B`.

1. Set up [VPC peering](https://docs.aws.amazon.com/vpc/latest/peering/working-with-vpc-peering.html) between the EKS VPC in account `A` and the EFS VPC in account `B`.
2. In account `B`, create an IAM role (e.g. `EFSCrossAccountAccessRole`) with a [trust relationship](./iam-policy-examples/trust-relationship-example.json) to account `A` and an inline EFS policy granting [`DescribeMountTargets`](./iam-policy-examples/describe-mount-target-example.json). The controller in account `A` assumes this role to discover mount targets in account `B`.
3. In account `A`, attach a policy to the controller's service-account IAM role granting [`sts:AssumeRole`](./iam-policy-examples/cross-account-assume-policy-example.json) on the role from step 2.
4. In account `A`, attach the [EFS client policy](./iam-policy-examples/node-deamonset-iam-policy-example.json) (or the AWS-managed `AmazonElasticFileSystemClientFullAccess`) to the node DaemonSet's service account.
5. In account `B`, attach a [file system policy](https://docs.aws.amazon.com/efs/latest/ug/iam-access-control-nfs-efs.html#file-sys-policy-examples) on the file system that allows account `A` to mount it.
6. Create the Kubernetes secret with the cross-account role ARN. Add `externalId` if the trust policy requires it; add `crossaccount=true` only for **Mode A** below.

   ```sh
   # Mode B and Mode C — secret with role ARN only
   kubectl create secret generic x-account \
     --namespace=kube-system \
     --from-literal=awsRoleArn='arn:aws:iam::123456789012:role/EFSCrossAccountAccessRole'

   # Mode A — DNS-based; add crossaccount=true
   kubectl create secret generic x-account \
     --namespace=kube-system \
     --from-literal=awsRoleArn='arn:aws:iam::123456789012:role/EFSCrossAccountAccessRole' \
     --from-literal=crossaccount='true'

   # Optional externalId
   kubectl create secret generic x-account \
     --namespace=kube-system \
     --from-literal=awsRoleArn='arn:aws:iam::123456789012:role/EFSCrossAccountAccessRole' \
     --from-literal=externalId='external-id'
   ```

> If you enable `delete-access-point-root-dir=true` on the controller, the controller mounts the file system itself during `DeleteVolume`. Make sure the EFS client policy from step 4 is also attached to the controller's service account, not just the node DaemonSet's.

## Dynamic provisioning

Each mode below uses the same Pod and PVC; only the StorageClass and the secret differ. The example pod and PVC live at [`./specs/pod.yaml`](./specs/pod.yaml) and they reference `storageClassName: efs-sc`.

### Mode A — DNS-based (recommended for AZ resilience)

Use when every node can meet the efs-utils [crossaccount option prerequisites](https://github.com/aws/efs-utils?tab=readme-ov-file#crossaccount-option-prerequisites). No mount target IP is baked into the PV; each node resolves a mount target in its own AZ at mount time.

Secret: include `crossaccount=true` (see the Mode A `kubectl create secret` command above).

StorageClass: [`./specs/storageclass-dns.yaml`](./specs/storageclass-dns.yaml) — no `az` parameter.

```sh
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/storageclass-dns.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/pod.yaml
```

### Mode B — Pinned AZ

Use when you want every pod for this StorageClass to mount a specific AZ's mount target. If no mount target exists in the named AZ, the controller falls back to a random available mount target.

Secret: omit `crossaccount` (use the Mode B/C secret command above).

StorageClass: [`./specs/storageclass.yaml`](./specs/storageclass.yaml) — sets `az` to the AZ where the desired mount target lives (replace the example value with your AZ).

```sh
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/storageclass.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/pod.yaml
```

### Mode C — Per-node AZ selection (default)

Use when you cannot meet the DNS prerequisites but still want to survive a single-AZ mount target outage. The controller resolves all available mount targets at provisioning time and each node selects the mount target in its own AZ. If no mount target exists in the node's AZ, it falls back to any available target (logged as a warning).

Secret: omit `crossaccount` (use the Mode B/C secret command above).

StorageClass: [`./specs/storageclass-default.yaml`](./specs/storageclass-default.yaml) — no `az`, no `crossaccount` in the secret.

```sh
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/storageclass-default.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/pod.yaml
```

### Verify

```sh
>> kubectl get pods
>> kubectl exec -ti efs-app -- tail -f /data/out
```

## Static provisioning

For static provisioning, the PV's `volumeAttributes` selects the mount behavior directly — there is no controller secret involved. The supported keys are `crossaccount` and `mounttargetip`. The PV does **not** accept an `az` key; to pin a static cross-account PV to a specific AZ, use `mounttargetip` with that AZ's mount target IP.

Both variants share the StorageClass at [`./specs/storageclass-static-prov.yaml`](./specs/storageclass-static-prov.yaml), the PVC at [`./specs/claim.yaml`](./specs/claim.yaml), and the pod at [`./specs/pod-static-prov.yaml`](./specs/pod-static-prov.yaml). Replace `[Filesystem ID]` and `[MOUNT TARGET IP ADDRESS]` placeholders before applying.

### DNS mode

Use when every node meets the [efs-utils crossaccount prerequisites](https://github.com/aws/efs-utils?tab=readme-ov-file#crossaccount-option-prerequisites).

PV: [`./specs/pv.yaml`](./specs/pv.yaml) — `volumeAttributes.crossaccount: "true"`.

```sh
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/storageclass-static-prov.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/pv.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/claim.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/pod-static-prov.yaml
```

### Explicit mount target IP

Use when you want to pin to a specific mount target — for example, a single-AZ workload, or when DNS prerequisites are not met.

PV: [`./specs/pv-mounttargetip.yaml`](./specs/pv-mounttargetip.yaml) — `volumeAttributes.mounttargetip: <ip>`.

```sh
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/storageclass-static-prov.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/pv-mounttargetip.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/claim.yaml
>> kubectl apply -f examples/kubernetes/efs/cross_account_mount/specs/pod-static-prov.yaml
```

### Verify

```sh
>> kubectl get pods
>> kubectl exec -ti efs-app -- tail -f /data/out.txt
```

## How DeleteVolume cleans up

When `delete-access-point-root-dir=true`, the controller mounts the file system in its own pod to remove the access point's root directory. For cross-account PVs:

- If the PV has `crossaccount=true`, the cleanup mount uses efs-utils DNS resolution.
- Otherwise, the controller calls `DescribeAvailableMountTargets` and prefers the mount target in **the controller node's own AZ**, falling back to any available target. This means cleanup traffic is sourced from the controller's AZ rather than a random AZ.
