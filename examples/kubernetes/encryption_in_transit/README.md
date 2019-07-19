## Encryption in Transit
This example shows how to make a static provisioned EFS persistence volume (PV) mounted inside container with encryption in transit enabled.

**Note**: this example requires Kubernetes v1.13+

### Edit [Persistence Volume Spec](./specs/pv.yaml) 

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
  persistentVolumeReclaimPolicy: Retain
  storageClassName: efs-sc
  mountOptions:
    - tls
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId] 
```
Note that encryption in transit is enabled using mount option `tls`. Replace `VolumeHandle` value with `FileSystemId` of the EFS filesystem that needs to be mounted.

You can find it using AWS CLI:
```sh
>> aws efs describe-file-systems --query "FileSystems[*].FileSystemId"
```

### Deploy the Example
Create PV, persistence volume claim (PVC) and storage class:
```sh
>> kubectl apply -f examples/kubernetes/encryption_in_transit/specs/storageclass.yaml
>> kubectl apply -f examples/kubernetes/encryption_in_transit/specs/pv.yaml
>> kubectl apply -f examples/kubernetes/encryption_in_transit/specs/claim.yaml
>> kubectl apply -f examples/kubernetes/encryption_in_transit/specs/pod.yaml
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
