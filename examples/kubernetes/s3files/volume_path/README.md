## S3 Files Volume Path
Similar to [static provisioning example](../static_provisioning). A sub directory of S3 Files can be mounted inside container. This gives cluster operator the flexibility to restrict the amount of data being accessed from different containers on S3 Files.

**Note**: this feature requires the sub directory to mount precreated on S3 Files before consuming the volume from pod.

### Edit [Persistence Volume Spec](./specs/example.yaml)
```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: s3files-pv1
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: s3files-sc
  csi:
    driver: efs.csi.aws.com
    volumeHandle: s3files:[FileSystemId]:[Path]
```
Replace `FileSystemId` of the S3 Files filesystem ID that needs to be mounted. And replace `Path` with a existing path on the filesystem.

You can find it using AWS CLI:
```sh
>> aws s3files list-file-systems --query "fileSystems[*].fileSystemId"
```

### Deploy the Example Application
Create PV, persistence volume claim (PVC) and storage class:
```sh
>> kubectl apply -f examples/kubernetes/s3files/volume_path/specs/example.yaml
```

### Check S3 Files filesystem is used
After the objects are created, verify that pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written onto S3 Files filesystem:

```sh
>> kubectl exec -ti s3files-app -- tail -f /data-dir1/out.txt
>> kubectl exec -ti s3files-app -- ls /data-dir2
```
