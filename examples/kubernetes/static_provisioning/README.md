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


## Cross Account Static Provisioning
### Prerequisites
* Create an EKS cluster in VPC A in account A with the EFS CSI Driver installed, and an EFS instance in VPC B in account B.
* Create a VPC connection between VPC A & VPC B; To set up a connection, follow the official documentation to set up the peering connection and configure route tables to send and receive traffic. 
    * The route tables must be configured for each of the EKS nodes as well the EFS Mount Targets’ Subnets.


* Create subnets in VPC B in each of the Availability Zones of the account A EKS nodes.
* Create EFS Mount Targets in each of the Availability Zones from the above step in VPC B.
* Attach an IAM Security Group to each of the EFS Mount Targets which allows inbound NFS access from VPC A’s CIDR block.

### Edit [Persistent Volume Spec](./specs/pv.yaml)
* From Account B, take note of the EFS Mount Target IP address & Filesystem ID.
* Replace [FILESYSTEM ID] and [MOUNT TARGET IP ADDRESS] with their respective values.
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
### Deploy the Example Application
Create PV and persistent volume claim (PVC):
```sh
>> kubectl apply -f examples/kubernetes/static_provisioning/specs/storageclass.yaml
>> kubectl apply -f examples/kubernetes/static_provisioning/specs/pv.yaml
>> kubectl apply -f examples/kubernetes/static_provisioning/specs/claim.yaml
>> kubectl apply -f examples/kubernetes/static_provisioning/specs/pod.yaml
```
### Verify Static Provisioning Was Successful
After the objects are created, verify that pod is running:

```sh
>> kubectl get pods
```

Also you can verify that data is written onto EFS filesystem:

```sh
>> kubectl exec -ti efs-app -- tail -f /data/out.txt
```
