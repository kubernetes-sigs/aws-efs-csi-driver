## Static Provisioning
This example shows how to make a static provisioned EFS PV mounted inside container.

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
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId] 
```
Replace `VolumeHandle` with `FileSystemId` of the EFS filesystem that needs to be mounted.

You can find it using AWS CLI:
```
aws efs describe-file-systems 
```

### Deploy the Example Application
Create PV, persistence volume claim (PVC) and storage class:
```
kubectl apply -f examples/kubernetes/static_provisioning/storageclass.yaml
kubectl apply -f examples/kubernetes/static_provisioning/pv.yaml
kubectl apply -f examples/kubernetes/static_provisioning/claim.yaml
kubectl apply -f examples/kubernetes/static_provisioning/pod.yaml
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
