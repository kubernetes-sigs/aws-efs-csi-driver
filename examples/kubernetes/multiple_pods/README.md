## Multiple Pods Read Write Many 
This example shows how to create a static provisioned Amazon EFS persistent volume (PV) and access it from multiple pods with the `ReadWriteMany` (RWX) access mode. This mode allows the PV to be read and written from multiple pods.

1. Clone the [Amazon EFS Container Storage Interface \(CSI\) driver](https://github.com/kubernetes-sigs/aws-efs-csi-driver) GitHub repository to your local system.

   ```sh
   git clone https://github.com/kubernetes-sigs/aws-efs-csi-driver.git
   ```

1. Navigate to the `multiple_pods` example directory.

   ```sh
   cd aws-efs-csi-driver/examples/kubernetes/multiple_pods/
   ```

1. Retrieve your Amazon EFS file system ID. You can find this in the Amazon EFS console, or use the following AWS CLI command.

   ```sh
   aws efs describe-file-systems --query "FileSystems[*].FileSystemId" --output text
   ```

   The example output is as follows.

   ```
   fs-582a03f3
   ```

1. Edit the [`specs/pv.yaml`](./specs/pv.yaml) file and replace the `volumeHandle` value with your Amazon EFS file system ID.

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
       volumeHandle: fs-582a03f3
   ```
**Note**  
`spec.capacity` is ignored by the Amazon EFS CSI driver because Amazon EFS is an elastic file system. The actual storage capacity value in persistent volumes and persistent volume claims isn't used when creating the file system. However, because storage capacity is a required field in Kubernetes, you must specify a valid value, such as, `5Gi` in this example. This value doesn't limit the size of your Amazon EFS file system.



1. Deploy the `efs-sc` storage class, the `efs-claim` PVC, and the `efs-pv` PV from the `specs` directory.

   ```sh
   kubectl apply -f specs/pv.yaml
   kubectl apply -f specs/claim.yaml
   kubectl apply -f specs/storageclass.yaml
   ```

1. List the persistent volumes in the default namespace. Look for a persistent volume with the `default/efs-claim` claim.

   ```sh
   kubectl get pv -w
   ```

   The example output is as follows.

   ```
   NAME     CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM               STORAGECLASS   REASON   AGE
   efs-pv   5Gi        RWX            Retain           Bound    default/efs-claim   efs-sc                  2m50s
   ```

   Don't proceed to the next step until the `STATUS` is `Bound`.

1. Deploy the `app1` and `app2` sample applications from the `specs` directory. Both `pod1` and `pod2` consume the PV and write to the same Amazon EFS filesystem at the same time.

   ```sh
   kubectl apply -f specs/pod1.yaml
   kubectl apply -f specs/pod2.yaml
   ```

1. Watch the Pods in the default namespace and wait for the `app1` and `app2` Pods' `STATUS` to become `Running`.

   ```sh
   kubectl get pods --watch
   ```
**Note**  
It may take a few minutes for the Pods to reach the `Running` status.

1. Describe the persistent volume.

   ```sh
   kubectl describe pv efs-pv
   ```

   The example output is as follows.

   ```
   Name:            efs-pv
   Labels:          none
   Annotations:     kubectl.kubernetes.io/last-applied-configuration:
                      {"apiVersion":"v1","kind":"PersistentVolume","metadata":{"annotations":{},"name":"efs-pv"},"spec":{"accessModes":["ReadWriteMany"],"capaci...
                    pv.kubernetes.io/bound-by-controller: yes
   Finalizers:      [kubernetes.io/pv-protection]
   StorageClass:    efs-sc
   Status:          Bound
   Claim:           default/efs-claim
   Reclaim Policy:  Retain
   Access Modes:    RWX
   VolumeMode:      Filesystem
   Capacity:        5Gi
   Node Affinity:   none
   Message:
   Source:
       Type:              CSI (a Container Storage Interface (CSI) volume source)
       Driver:            efs.csi.aws.com
       VolumeHandle:      fs-582a03f3
       ReadOnly:          false
       VolumeAttributes:  none
   Events:                none
   ```

   The Amazon EFS file system ID is listed as the `VolumeHandle`.

1. Verify that the `app1` Pod is successfully writing data to the volume.

   ```sh
   kubectl exec -ti app1 -- tail -f /data/out1.txt
   ```

   The example output is as follows.

   ```
   [...]
   Mon Mar 22 18:18:22 UTC 2021
   Mon Mar 22 18:18:27 UTC 2021
   Mon Mar 22 18:18:32 UTC 2021
   Mon Mar 22 18:18:37 UTC 2021
   [...]
   ```

1. Verify that the `app2` Pod shows the same data in the volume that `app1` wrote to the volume.

   ```sh
   kubectl exec -ti app2 -- tail -f /data/out2.txt
   ```

   The example output is as follows.

   ```
   [...]
   Mon Mar 22 18:18:22 UTC 2021
   Mon Mar 22 18:18:27 UTC 2021
   Mon Mar 22 18:18:32 UTC 2021
   Mon Mar 22 18:18:37 UTC 2021
   [...]
   ```

1. When you finish experimenting, delete the resources for this sample application to clean up.

   ```sh
   kubectl delete -f specs/
   ```

   You can also manually delete the file system and security group that you created.
