## Dynamic Provisioning
This example shows how to create a dynamically provisioned volume created through [EFS access points](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html) and Persistent Volume Claim (PVC) and consume it from a pod.

**Note**: this example requires Kubernetes v1.17+ and driver version >= 1.2.0.

### Edit [StorageClass](./specs/storageclass.yaml)

```
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: efs-sc
provisioner: efs.csi.aws.com
mountOptions:
  - tls
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-92107410
  directoryPerms: "700"
  gidRangeStart: "1000"
  gidRangeEnd: "2000"
  basePath: "/dynamic_provisioning"
```
* provisioningMode - The type of volume to be provisioned by efs. Currently, only access point based provisioning is supported `efs-ap`.
* fileSystemId - The file system under which Access Point is created.
* directoryPerms - Directory Permissions of the root directory created by Access Point.
* gidRangeStart (Optional) - Starting range of Posix Group ID to be applied onto the root directory of the access point. Default value is 50000. 
* gidRangeEnd (Optional) - Ending range of Posix Group ID. Default value is 7000000.
* basePath (Optional) - Path on the file system under which access point root directory is created. If path is not provided, access points root directory are created under the root of the file system.

### Deploy the Example
Create storage class, persistent volume claim (PVC) and the pod which consumes PV:
```sh
>> kubectl apply -f examples/kubernetes/dynamic_provisioning/specs/storageclass.yaml
>> kubectl apply -f examples/kubernetes/dynamic_provisioning/specs/pod.yaml
```

### Check EFS filesystem is used
After the objects are created, verify that pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written onto EFS filesystem:

```sh
>> kubectl exec -ti efs-app -- tail -f /data/out
```
### Note:
When you want to delete an access point in a file system when deleting PVC, you should specify `elasticfilesystem:ClientRootAccess` to the file system access policy to provide the root permissions. 