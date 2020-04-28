## EFS Access Points
Like [volume path mounts](../volume_path), mounting [EFS access points](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html) allows you to expose separate data stores with independent ownership and permissions from a single EFS volume.
In this case, the separation is managed on the EFS side rather than the kubernetes side.

**Note**: Because access point mounts require TLS, this is not supported in driver versions at or before `0.3`.

### Create Access Points (in EFS)
Following [this doc](https://docs.aws.amazon.com/efs/latest/ug/create-access-point.html), create a separate access point for each independent data store you wish to expose in your cluster, tailoring the ownership and permissions as desired.
There is no need to use different EFS volumes.

**Note**: Although it is possible to [configure IAM policies for access points](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#access-points-iam-policy), by default no additional IAM permissions are necessary.

This example assumes you are using two access points.

### Edit [Persistent Volume Spec](./specs/example.yaml)
```
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: efs-pv1
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: efs-sc
  mountOptions:
    - tls
    - accesspoint=[AccessPointId]
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId]
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: efs-pv2
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: efs-sc
  mountOptions:
    - tls
    - accesspoint=[AccessPointId]
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId]
```
In each PersistentVolume, replace both the `[FileSystemId]` in `spec.csi.volumeHandle` and the `[AccessPointId]` value of the `accesspoint` option in `spec.mountOptions`.
You can find these values using the AWS CLI:
```sh
>> aws efs describe-access-points --query 'AccessPoints[*].{"FileSystemId": FileSystemId, "AccessPointId": AccessPointId}'
```
If you are using the same underlying EFS volume, the `FileSystemId` will be the same in both PersistentVolume specs, but the `AccessPointId` will differ.

### Deploy the Example Application
Create PVs, persistent volume claims (PVCs), and storage class:
```sh
>> kubectl apply -f examples/kubernetes/volume_path/specs/example.yaml
```

### Check EFS filesystem is used
After the objects are created, verify the pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written into the EFS filesystems:

```sh
>> kubectl exec -ti efs-app -- tail -f /data-dir1/out.txt
>> kubectl exec -ti efs-app -- ls /data-dir2
```
