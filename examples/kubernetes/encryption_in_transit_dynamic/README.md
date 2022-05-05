## Encryption in Transit
This example shows how to make a dynamically provisioned EFS persistent volume (PV) mounted inside container with encryption in transit configured.

**Note**: this example requires Kubernetes v1.13+ and driver version > 0.3. For driver versions <= 0.3, encryption in transit is enabled (or disabled) by setting `encryptInTransit` as required on the StorageClass Parameters.

### Edit [Persistence Volume Spec](./specs/pv.yaml) 

```
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: efs-sc
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemId: [FileSystemId]
  directoryPerms: "700"
  encryptInTransit: "false"
```
Replace `FileSystemId` with the ID of the EFS filesystem that
needs to be mounted. The following table illustrates how the setting of
`encryptInTransit` determines whether encryption in transit is enabled or not:

|  | encryptInTransit is unset | encryptInTransit is true | encryptInTransit is false |
| ------------- | ------------- | ------------- | ------------- |
| "tls" is in mountOptions  | encryption + deprecation warning  | encryption + deprecation warning | error |
| "tls" isn't in mountOptions | encryption  | encryption | NO encryption |

You can find it using AWS CLI:
```sh
>> aws efs describe-file-systems --query "FileSystems[*].FileSystemId"
```

### Deploy the Example
Create PV, persistent volume claim (PVC) and storage class:
```sh
>> kubectl apply -f examples/kubernetes/encryption_in_transit/specs/storageclass.yaml
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
