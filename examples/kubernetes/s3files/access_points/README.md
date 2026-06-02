## S3 Files Access Points
Like [volume path mounts](../volume_path), mounting S3 Files Access Points allows you to expose separate data stores with independent ownership and permissions from a single S3 Files file system.
In this case, the separation is managed on the S3 Files side rather than the kubernetes side.

### Create Access Points (in S3 Files)
Following [this doc](https://docs.aws.amazon.com/s3files/latest/ug/create-access-point.html), create a separate access point for each independent data store you wish to expose in your cluster, tailoring the ownership and permissions as desired.
There is no need to use different S3 Files file systems.

**Note**: Although it is possible to [configure IAM policies for access points](https://docs.aws.amazon.com/s3files/latest/ug/s3files-access-points.html#access-points-iam-policy), by default no additional IAM permissions are necessary.

This example assumes you are using two access points.

### Edit [Persistent Volume Spec](./specs/example.yaml)
```
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: s3files-pv1
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: s3files-sc
  csi:
    driver: efs.csi.aws.com
    volumeHandle: s3files:[FileSystemId]::[AccessPointId]
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: s3files-pv2
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: s3files-sc
  csi:
    driver: efs.csi.aws.com
    volumeHandle: s3files:[FileSystemId]::[AccessPointId]
```
In each PersistentVolume, replace both the `[FileSystemId]` and the `[AccessPointId]` in `spec.csi.volumeHandle`.
You can find these values using the AWS CLI:
```sh
>> aws s3files list-access-points --query 'accessPoints[*].{"FileSystemId": fileSystemId, "AccessPointId": accessPointId}'
```
If you are using the same underlying S3 Files file system, the `FileSystemId` will be the same in both PersistentVolume specs, but the `AccessPointId` will differ.

**Note:** The double colon in the `volumeHandle` is intentional.
The middle field indicates the subpath; if omitted, no subpath is used.
See [below](#other-details) for more information.

### Deploy the Example Application
Create PVs, persistent volume claims (PVCs), and storage class:
```sh
>> kubectl apply -f examples/kubernetes/s3files/access_points/specs/example.yaml
```

### Check S3 Files filesystem is used
After the objects are created, verify the pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written into the S3 Files filesystems:

```sh
>> kubectl exec -ti s3files-app -- tail -f /data-dir1/out.txt
>> kubectl exec -ti s3files-app -- ls /data-dir2
```

### Other Details
- It is possible to use a combination of [volume path](../volume_path) and access points
  with a `volumeHandle` of the form `s3files:[FileSystemId]:[Subpath]:[AccessPointId]`, e.g.
  `volumeHandle: s3files:fs-e8a95a42:/my/subpath:fsap-19f752f0068c22464`. In this case:
  - The `[Subpath]` will be _under_ the configured access point directory. For example,
    if you configured your access point with `/ap1`, the above would mount to
    `/ap1/my/subpath`.
  - As with normal volume path, the `[Subpath]` must already exist prior to consuming
    the volume from a pod.

- S3 Files file systems always use encryption in transit by default and cannot be disabled.
- `awscredsuri` mount option is not supported through efs-csi-driver as it's designed and used by ECS tasks.
