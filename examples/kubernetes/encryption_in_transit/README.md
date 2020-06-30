## Encryption in Transit
This example shows how to make a static provisioned EFS persistent volume (PV) mounted inside container with encryption in transit configured.

**Note**: this example requires Kubernetes v1.13+ and driver version > 0.3. For driver versions <= 0.3, encryption in transit is enabled (or disabled) by adding (or omitting) mountOption "tls" to (or from) a PV.

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
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId] 
    volumeAttributes:
      encryptInTransit: "true"
```
Note that encryption in transit is configured using volume attribute `encryptInTransit`. By default, encryption in transit is enabled and there is no need to set `encryptInTransit` true. Replace `VolumeHandle` value with `FileSystemId` of the EFS filesystem that needs to be mounted.

You can find it using AWS CLI:
```sh
>> aws efs describe-file-systems --query "FileSystems[*].FileSystemId"
```

### Deploy the Example
Create PV, persistent volume claim (PVC) and storage class:
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
