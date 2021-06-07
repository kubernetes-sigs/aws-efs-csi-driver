## Dynamic Provisioning
This example shows how to create a dynamically provisioned volume created through [EFS access points](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html) and Persistent Volume Claim (PVC) and consume it from a pod.

**Note**: this example requires Kubernetes v1.17+ and driver version >= 1.2.0.

### Edit [StorageClass](./specs/storageclass.yaml)

```
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: efs-sc
provisioner: efs.csi.aws.com
mountOptions:
  - tls
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-1234abcd
  directoryPerms: "700"
  gidRangeStart: "1000"
  gidRangeEnd: "2000"
  basePath: "/dynamic_provisioning"
  az: us-east-1a
```

### Prerequisite setup
1. Perform [vpc-peering](https://docs.aws.amazon.com/vpc/latest/peering/working-with-vpc-peering.html) between EKS cluster `vpc` in aws account `A` and EFS `vpc` in another aws account `B` .
2. Create an IAM role in Account `B` which has a trust relationship with Account `A` and add an EFS policy with permissions to call `DescribeMountTargets`. This role will be used by CSI-Driver to determine the mount targets for file system in account `B`
3. Create kubenetes secret with `awsRoleArn` as the key and the role from step 2 as the value. For example, `kubectl create secret generic x-account --namespace=default --from-literal=awsRoleArn='arn:aws:iam::1234567890:role/EFSCrossAccountAccessRole'`
4. Create a service account with IAM role for the EKS cluster and attach it node-deamonset. Attach this IAM role with EFS client mount permission policy.
5. Create a [file system policy](https://docs.aws.amazon.com/efs/latest/ug/iam-access-control-nfs-efs.html#file-sys-policy-examples) for file system in account `B` which allows account `A` to perform mount on it.

### Deploy the Example
Create storage class, persistent volume claim (PVC) and the pod which consumes PV:
```sh
>> kubectl apply -f examples/kubernetes/dynamic_provisioning/specs/storageclass.yaml
>> kubectl apply -f examples/kubernetes/dynamic_provisioning/specs/pod.yaml
```

### Check EFS filesystem is used
After the objects are created, verify that pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written onto EFS filesystem:

```sh
>> kubectl exec -ti efs-app -- tail -f /data/out
```
