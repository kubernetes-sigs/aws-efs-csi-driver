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

   2. Download a `StorageClass` manifest for Amazon EFS.

      ```sh
      curl -O https://raw.githubusercontent.com/kubernetes-sigs/aws-efs-csi-driver/master/examples/kubernetes/dynamic_provisioning/specs/storageclass.yaml
      ```

   3. Edit [the file](./specs/storageclass.yaml). Find the following line, and replace the value for `fileSystemId` with your file system ID.

      ```
      fileSystemId: fs-582a03f3
      ```
      Modify the other values as needed:
      * `provisioningMode` - The type of volume to be provisioned by Amazon EFS. Currently, only access point based provisioning is supported (`efs-ap`).
      * `fileSystemId` - The file system under which the access point is created.
      * `directoryPerms` - The directory permissions of the root directory created by the access point.
      * `gidRangeStart` (Optional) - The starting range of the Posix group ID to be applied onto the root directory of the access point. The default value is `50000`. 
      * `gidRangeEnd` (Optional) - The ending range of the Posix group ID. The default value is `7000000`.
      * `basePath` (Optional) - The path on the file system under which the access point root directory is created. If the path isn't provided, the access points root directory is created under the root of the file system.
      * `subPathPattern` (Optional) - A pattern that describes the subPath under which an access point should be created. So if the pattern were `${.PVC.namespace}/${PVC.name}`, the PVC namespace is `foo` and the PVC name is `pvc-123-456`, and the `basePath` is `/dynamic_provisioner` the access point would be
        created at `/dynamic_provisioner/foo/pvc-123-456`.
      * `ensureUniqueDirectory` (Optional) - A boolean that ensures that, if set, a UUID is appended to the final element of
        any dynamically provisioned path, as in the above example. This can be turned off but this requires you as the
        administrator to ensure that your storage classes are set up correctly. Otherwise, it's possible that 2 pods could
        end up writing to the same directory by accident. **Please think very carefully before setting this to false!**

   4. Deploy the storage class.

      ```sh
      kubectl apply -f storageclass.yaml
      ```

2. Test automatic provisioning by deploying a Pod that makes use of the PVC: 

   1. Download a manifest that deploys a Pod and a PVC.

      ```sh
      curl -O https://raw.githubusercontent.com/kubernetes-sigs/aws-efs-csi-driver/master/examples/kubernetes/dynamic_provisioning/specs/pod.yaml
      ```

   2. Deploy the Pod with a sample app and the PVC used by the Pod.

      ```sh
      kubectl apply -f pod.yaml
      ```
3. Determine the names of the Pods running the controller.
   ```sh
   kubectl get pods -n kube-system | grep efs-csi-controller
   ```

   The example output is as follows.

   ```
   efs-csi-controller-74ccf9f566-q5989   3/3     Running   0          40m
   efs-csi-controller-74ccf9f566-wswg9   3/3     Running   0          40m
   ```

4. After few seconds, you can observe the controller picking up the change \(edited for readability\). Replace `74ccf9f566-q5989` with a value from one of the Pods in your output from the previous command.

   ```sh
   kubectl logs efs-csi-controller-74ccf9f566-q5989 \
       -n kube-system \
       -c csi-provisioner \
       --tail 10
   ```

   The example output is as follows.

   ```
   [...]
   1 controller.go:737] successfully created PV pvc-5983ffec-96cf-40c1-9cd6-e5686ca84eca for PVC efs-claim and csi volume name fs-95bcec92::fsap-02a88145b865d3a87
   ```

   If you don't see the previous output, run the previous command using one of the other controller Pods.

5. Confirm that a persistent volume was created with a status of `Bound` to a `PersistentVolumeClaim`:

   ```sh
   kubectl get pv
   ```

   The example output is as follows.

   ```
   NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM               STORAGECLASS   REASON   AGE
   pvc-5983ffec-96cf-40c1-9cd6-e5686ca84eca   20Gi       RWX            Delete           Bound    default/efs-claim   efs-sc                  7m57s
   ```

6. View details about the `PersistentVolumeClaim` that was created.

   ```sh
   kubectl get pvc
   ```

   The example output is as follows.

   ```
   NAME        STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
   efs-claim   Bound    pvc-5983ffec-96cf-40c1-9cd6-e5686ca84eca   20Gi       RWX            efs-sc         9m7s
   ```

7. View the sample app Pod's status until the `STATUS` becomes `Running`.

   ```sh
   kubectl get pods -o wide
   ```

   The example output is as follows.

   ```
   NAME          READY   STATUS    RESTARTS   AGE   IP               NODE                                             NOMINATED NODE   READINESS GATES
   efs-app       1/1     Running   0          10m   192.168.78.156   ip-192-168-73-191.region-code.compute.internal   <none>           <none>
   ```
**Note**  
If a Pod doesn't have an IP address listed, make sure that you added a mount target for the subnet that your node is in \(as described at the end of [Create an Amazon EFS file system](#efs-create-filesystem)\). Otherwise the Pod won't leave `ContainerCreating` status. When an IP address is listed, it may take a few minutes for a Pod to reach the `Running` status.

1. Confirm that the data is written to the volume.

   ```sh
   kubectl exec efs-app -- bash -c "cat data/out"
   ```

   The example output is as follows.

   ```
   [...]
   Tue Mar 23 14:29:16 UTC 2021
   Tue Mar 23 14:29:21 UTC 2021
   Tue Mar 23 14:29:26 UTC 2021
   Tue Mar 23 14:29:31 UTC 2021
   [...]
   ```

2. \(Optional\) Terminate the Amazon EKS node that your Pod is running on and wait for the Pod to be re\-scheduled. Alternately, you can delete the Pod and redeploy it. Complete the previous step again, confirming that the output includes the previous output.

**Note**  
When you want to delete an access point in a file system when deleting PVC, you should specify `elasticfilesystem:ClientRootAccess` to the file system access policy to provide the root permissions. 
