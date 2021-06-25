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
  csi.storage.k8s.io/provisioner-secret-name: x-account
  csi.storage.k8s.io/provisioner-secret-namespace: kube-system
```

### Prerequisite setup
Lets say you have an EKS cluster in aws account `A` & you wish to mount your file system in another aws account `B` using aws-efs-csi-driver, you'll need to perform the following steps before you proceed with cross account mount between accounts `A` & `B` 
1. Perform [vpc-peering](https://docs.aws.amazon.com/vpc/latest/peering/working-with-vpc-peering.html) between EKS cluster `vpc` in aws account `A` and EFS `vpc` in another aws account `B`.
2. Create an IAM role, say `EFSCrossAccountAccessRole` in Account `B` which has a [trust relationship](./iam-policy-examples/trust-relationship-example.json) with Account `A` and add an inline EFS policy with [permissions](./iam-policy-examples/describe-mount-target-example.json) to call `DescribeMountTargets`. This role will be used by CSI-Driver's Controller service running on EKS cluster in account `A` to determine the mount targets for your file system in account `B`. 
3. In aws account `A`, attach an inline policy to IAM role of efs-csi-driver's controller service account with necessary [permissions](./iam-policy-examples/cross-account-assume-policy-example.json) to perform `sts assume role` on the IAM role created in step 2.  
4. Create a kubernetes secret with `awsRoleArn` as the key and the role from step 2 as the value. For example, `kubectl create secret generic x-account --namespace=default --from-literal=awsRoleArn='arn:aws:iam::123456789012:role/EFSCrossAccountAccessRole'`.
5. Create an IAM role for service accounts for EKS cluster in account `A` with required [permissions](./iam-policy-examples/node-deamonset-iam-policy-example.json) for EFS client mount. Alternatively, you can find this policy under AWS managed policy as `AmazonElasticFileSystemClientFullAccess`.  
6. Attach the service account from step 5 to node daemonset.
7. Create a [file system policy](https://docs.aws.amazon.com/efs/latest/ug/iam-access-control-nfs-efs.html#file-sys-policy-examples) for file system in account `B` which allows account `A` to perform mount on it.

#### Note: 
In dynamic provisioning, if you wish to enable delete access points root directory by setting `delete-access-point-root-dir=true`, you must attach the IAM policy from step 5 above to controller service account's IAM role. 

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

