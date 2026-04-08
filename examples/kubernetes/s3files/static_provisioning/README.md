## Static Provisioning
This example shows how to make a static provisioned S3 Files persistent volume (PV) mounted inside container.

### Edit [Persistent Volume Spec](./specs/pv.yaml)

```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: s3files-pv
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  storageClassName: s3files-sc
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: efs.csi.aws.com
    volumeHandle: s3files:[FileSystemId] 
```
Replace `VolumeHandle` value with `FileSystemId` of the S3 Files filesystem that needs to be mounted.

You can find it using AWS CLI:
```sh
>> aws s3files list-file-systems --query "fileSystems[*].fileSystemId"
```

### Deploy the Example Application
Create PV and persistent volume claim (PVC):
```sh
>> kubectl apply -f examples/kubernetes/s3files/static_provisioning/specs/storageclass.yaml
>> kubectl apply -f examples/kubernetes/s3files/static_provisioning/specs/pv.yaml
>> kubectl apply -f examples/kubernetes/s3files/static_provisioning/specs/claim.yaml
```

List the persistent volumes in the default namespace. Look for a persistent volume with the default/s3files-claim claim.

```sh
kubectl get pv -w
```

The example output is as follows.

```
$ kubectl get pv -w
NAME     CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM               STORAGECLASS   REASON   AGE
s3files-pv   5Gi        RWO            Retain           Bound    default/s3files-claim                           3m31s
```

Don't proceed to the next step until the `STATUS` is `Bound`.

Deploy the `app` sample applications
```
>> kubectl apply -f examples/kubernetes/s3files/static_provisioning/specs/pod.yaml
```

### Check S3 Files filesystem is used
After the objects are created, verify that pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written onto S3 Files filesystem:

```sh
>> kubectl exec -ti s3files-app -- tail -f /data/out.txt
```


**Note**

In certain use cases, it may be useful to provide the S3 Files Mount Target IP Address when performing static provisioning. This can optionally be specified in the PersistentVolume object specification ```volumeAttributes``` section. This allows the CSI Driver to mount via IP address directly without additional communication with a DNS server. Example:
```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: s3files-pv
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  storageClassName: s3files-sc
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: efs.csi.aws.com
    volumeHandle: s3files:[FILESYSTEM ID]
    volumeAttributes:
      mounttargetip: "[MOUNT TARGET IP ADDRESS]"
```
