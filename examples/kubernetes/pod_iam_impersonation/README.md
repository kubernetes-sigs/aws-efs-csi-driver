## CSI Driver POD IMPERSONATION

This example shows how you can use the pod impersonation feature available 
in the CSI Driver to enforce access control when mounting EFS Access point protected with EFS resource policy.
This feature is for EKS only and leverages IAM roles for service accounts (IRSA), your EKS cluster must be enabled for IRSA. 
You can follow this [link](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) on how to setup and use IRSA.

**Note:** This feature only works when installed with Helm and `podIAMAuthorization` is set to `true`. 

### IMPORTANT
This feature will not work if you are upgrading the Helm chart. It MUST be a clean install as the CSI Driver object is immutable. 
The field `podIAMAuthorization` in the HELM chart is set to keep backward compatibility in case you need to upgrade/patch an already existing 
deployment of the EFS CSI Driver or do not want to use the Pod impersonation feature.

## Example

This example assumes you have an access point set up and is protected with a resource policy. 
If you would like to learn how to configure IAM policies for access points you can follow this [link](https://docs.aws.amazon.com/efs/latest/ug/efs-access-points.html#access-points-iam-policy).

### Create the Persistent Volume Spec

```
kind: PersistentVolume
metadata:
  name: efs-pv1
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
    volumeHandle: [FileSystemId]::[AccessPointId]
    volumeAttributes:
      podIAMAuthorization: "true"
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: efs-claim1
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: efs-sc
  resources:
    requests:
      storage: 5Gi
```

Here it is important to set the property `podIAMAuthorization` in `volumeAttribute` to `true`. 
This indicates to the EFS CSI Driver when mounting the Access Point to use 
the role annotated to the service account associated with the `pod`.

### Deploy an Application

You can use the sample application in  `examples/kubernetes/pod_iam_impersonation/specs/example.yaml` 
to create a Persistent Volume (PV), Persistent Volume Claim (PVC) and a sample application. 

**Note:** The pod defined in the example has a service account associated with it. 
In the example the service account is `efs-app-sa`. The service account needs to be create and annotated with a role 
that is allowed in the EFS resource policy associated with Access Pointed referenced above in the PV.

Once you have created the service account you can apply the following command:

```sh
>> kubectl apply -f examples/kubernetes/pod_iam_impersonation/specs/example.yaml
```

### Check EFS filesystem is used
After the objects are created, verify the pod is running:

```sh
>> kubectl get pods
```

You can also verify that data is being written into the EFS file system.

```sh
>> kubectl exec -ti efs-app-1 -- tail -f /data-dir1/out.txt
```
