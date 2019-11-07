## EFS Volume Path
Similar to [static provisioning example](../static_provisioning). A sub directory of EFS can be mounted inside container. This gives cluster operator the flexibility to restrict the amount of data being accessed from different containers on EFS.

**Note**: this feature requires the sub directory to mount precreated on EFS before consuming the volume from pod.

### Edit [Persistence Volume Spec](./specs/example.yaml)
```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: efs-pv1
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: efs-sc
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId]:[Path]
```
Replace `FileSystemId` of the EFS filesystem ID that needs to be mounted. And replace `Path` with a existing path on the filesystem.

You can find it using AWS CLI:
```sh
>> aws efs describe-file-systems --query "FileSystems[*].FileSystemId"
```

### Deploy the Example Application
Create PV, persistence volume claim (PVC) and storage class:
```sh
>> kubectl apply -f examples/kubernetes/volume_path/specs/example.yaml
```

### Check EFS filesystem is used
After the objects are created, verify that pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written onto EFS filesystem:

```sh
>> kubectl exec -ti efs-app -- tail -f /data-dir1/out.txt
>> kubectl exec -ti efs-app -- ls /data-dir2
```
