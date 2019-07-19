## Multiple Pods Read Write Many 
This example shows how to create a static provisioned EFS persistence volume (PV) and access it from multiple pods with RWX access mode.

### Edit Persistent Volume
Edit persistent volume using sample [spec](./specs/pv.yaml):
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
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: efs-sc
  csi:
    driver: efs.csi.aws.com
    volumeHandle: [FileSystemId] 
```
Replace `volumeHandle` value with `FileSystemId` of the EFS filesystem that needs to be mounted. Note that the access mode is `RWX` which means the PV can be read and written from multiple pods.

You can get `FileSystemId` using AWS CLI:

```sh
>> aws efs describe-file-systems --query "FileSystems[*].FileSystemId"
```

### Deploy the Example Application
Create PV, persistence volume claim (PVC), storageclass and the pods that consume the PV:
```sh
>> kubectl apply -f examples/kubernetes/multiple_pods/specs/storageclass.yaml
>> kubectl apply -f examples/kubernetes/multiple_pods/specs/pv.yaml
>> kubectl apply -f examples/kubernetes/multiple_pods/specs/claim.yaml
>> kubectl apply -f examples/kubernetes/multiple_pods/specs/pod1.yaml
>> kubectl apply -f examples/kubernetes/multiple_pods/specs/pod2.yaml
```

In the example, both pod1 and pod2 are writing to the same EFS filesystem at the same time.

### Check the Application uses EFS filesystem
After the objects are created, verify that pod is running:

```sh
>> kubectl get pods
```

Also verify that data is written onto EFS filesystem from both pods:

```sh
>> kubectl exec -ti app1 -- tail -f /data/out1.txt
>> kubectl exec -ti app2 -- tail -f /data/out2.txt
```
