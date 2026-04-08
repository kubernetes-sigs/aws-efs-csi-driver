## S3 Files AZ id
Similar to [static provisioning example](../static_provisioning). For S3 Files, `efs-utils` relies on Instance Metadata Service (IMDS) to automatically determine the Availability Zone ID. However, if IMDS is not available in your environment (e.g., IMDS is disabled or restricted), you will need to manually specify the AZ ID via mountOptions as shown in this example.

### Edit [Persistence Volume Spec](./specs/example.yaml)
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
  mountOptions:
    - azid=[AZID]
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: efs.csi.aws.com
    volumeHandle: s3files:[FileSystemId]
```
Replace `FileSystemId` with the S3 Files filesystem ID that needs to be mounted. And replace `AZID` with the Availability Zone ID where your S3 Files Mount Target is located.

You can find FileSystemId using AWS CLI:
```sh
>> aws s3files list-file-systems --query "fileSystems[*].fileSystemId"
```

You can find AZ ID of you Mount Target using AWS CLI:
```sh
>> aws s3files list-mount-targets --file-system-id [FileSystemId] --query 'mountTargets[*].{"mountTargetId": mountTargetId, "azid": availabilityZoneId}'
```

### Deploy the Example Application
Create PV, persistence volume claim (PVC) and storage class:
```sh
>> kubectl apply -f examples/kubernetes/s3files/azid/specs/example.yaml
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
