## Static Provisioning
This example shows how to make a static provisioned EFS persistent volume (PV) mounted inside container.

### Edit [Persistent Volume Spec](./specs/pv.yaml)

```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: efs-pv
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  storageClassName: efs-sc
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId] 
```
Replace `VolumeHandle` value with `FileSystemId` of the EFS filesystem that needs to be mounted.

You can find it using AWS CLI:
```sh
>> aws efs describe-file-systems --query "FileSystems[*].FileSystemId"
```

### Deploy the Example Application
Create PV and persistent volume claim (PVC):
```sh
>> kubectl apply -f examples/kubernetes/static_provisioning/specs/storageclass.yaml
>> kubectl apply -f examples/kubernetes/static_provisioning/specs/pv.yaml
>> kubectl apply -f examples/kubernetes/static_provisioning/specs/claim.yaml
```

List the persistent volumes in the default namespace. Look for a persistent volume with the default/efs-claim claim.

```sh
kubectl get pv -w
```

The example output is as follows.

```
$ kubectl get pv -w
NAME     CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM               STORAGECLASS   REASON   AGE
efs-pv   5Gi        RWO            Retain           Bound    default/efs-claim                           3m31s
```

Don't proceed to the next step until the `STATUS` is `Bound`.

Deploy the `app` sample applications
```
>> kubectl apply -f examples/kubernetes/static_provisioning/specs/pod.yaml
```

### Check EFS filesystem is used
After the objects are created, verify that pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written onto EFS filesystem:

```sh
>> kubectl exec -ti efs-app -- tail -f /data/out.txt
```


**Note**

In certain use cases, it may be useful to provide the EFS Mount Target IP Address when performing static provisioning. This can optionally be specified in the PersistentVolume object specification ```volumeAttributes``` section. This allows the CSI Driver to mount via IP address directly without additional communication with a DNS server. Example:
```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: efs-pv
spec:
  capacity:
    storage: 5Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  storageClassName: efs-sc
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: efs.csi.aws.com
    volumeHandle:[FILESYSTEM ID]
    volumeAttributes:
      mounttargetip: "[MOUNT TARGET IP ADDRESS]"
```
