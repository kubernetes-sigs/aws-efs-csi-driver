## Encryption in Transit
This example shows how to make a static provisioned EFS PV mounted inside container with encryption in transit enabled.

**Note**: this example required Kubernetes v1.13+

### Edit [Persistence Volume Spec](pv.yaml) 

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
  persistentVolumeReclaimPolicy: Recycle
  storageClassName: efs-sc
  mountOptions:
    - tls
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId] 
```
Note that encryption in transit is enabled using mount option `tls`. Replace `VolumeHandle` with `FileSystemId` of the EFS filesystem that needs to be mounted.

You can find it using AWS CLI:
```
aws efs describe-file-systems 
```

### Deploy the Example
Create PV, persistence volume claim (PVC) and storage class:
```
kubectl apply -f examples/kubernetes/encryption_in_transit/storageclass.yaml
kubectl apply -f examples/kubernetes/encryption_in_transit/pv.yaml
kubectl apply -f examples/kubernetes/encryption_in_transit/claim.yaml
kubectl apply -f examples/kubernetes/encryption_in_transit/pod.yaml
```

### Check EFS filesystem is used
After the objects are created, verify that pod is running:

```
kubectl get pods
```

Also you can verify that data is written onto EFS filesystem:

```
kubectl exec -ti app -- tail -f /data/out.txt
```
