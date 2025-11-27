# EFS CSI Driver Multipathing Examples

## Example 1: Basic Multipathing StorageClass

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: efs-multipath-basic
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-12345678
  multipathing: "true"
```

## Example 2: High-Performance Multipathing with Tuning

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: efs-multipath-hiperf
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-12345678
  multipathing: "true"
  maxMultipathTargets: "8"
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: efs-hiperf-pvc
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: efs-multipath-hiperf
  resources:
    storage: 100Gi
  mountOptions:
    - rsize=1048576
    - wsize=1048576
    - timeo=600
    - retrans=2
    - nconnect=4
---
apiVersion: v1
kind: Pod
metadata:
  name: efs-hiperf-pod
spec:
  containers:
  - name: app
    image: ubuntu:latest
    command: ["/bin/sleep", "3600"]
    volumeMounts:
    - name: efs-volume
      mountPath: /mnt/efs
    resources:
      requests:
        memory: "256Mi"
        cpu: "500m"
      limits:
        memory: "512Mi"
        cpu: "1000m"
  volumes:
  - name: efs-volume
    persistentVolumeClaim:
      claimName: efs-hiperf-pvc
  nodeSelector:
    node.kubernetes.io/instance-type: "c5.4xlarge"  # Instance with multiple ENIs
```

## Example 3: Multipathing with StatefulSet

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: efs-multipath-stateful
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-12345678
  multipathing: "true"
  maxMultipathTargets: "4"
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: data-processor
spec:
  serviceName: data-processor
  replicas: 3
  selector:
    matchLabels:
      app: data-processor
  template:
    metadata:
      labels:
        app: data-processor
    spec:
      containers:
      - name: processor
        image: myapp:latest
        resources:
          requests:
            memory: "512Mi"
            cpu: "1000m"
        volumeMounts:
        - name: data
          mountPath: /data
      nodeSelector:
        workload-type: compute
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes:
        - ReadWriteMany
      storageClassName: efs-multipath-stateful
      resources:
        storage: 50Gi
      mountOptions:
        - rsize=1048576
        - wsize=1048576
```

## Example 4: Multipathing with Deployment and Direct PVC Sharing

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: efs-multipath-shared
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-12345678
  multipathing: "true"
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: shared-data
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: efs-multipath-shared
  resources:
    storage: 200Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: worker-pool
spec:
  replicas: 5
  selector:
    matchLabels:
      app: worker
  template:
    metadata:
      labels:
        app: worker
    spec:
      containers:
      - name: worker
        image: myworker:latest
        env:
        - name: DATA_PATH
          value: /shared
        volumeMounts:
        - name: shared-data
          mountPath: /shared
        resources:
          requests:
            memory: "256Mi"
            cpu: "500m"
      volumes:
      - name: shared-data
        persistentVolumeClaim:
          claimName: shared-data
      affinity:
        # Spread pods across nodes for better ENI utilization
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - worker
              topologyKey: kubernetes.io/hostname
```

## Example 5: Monitoring Multipathing Performance

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: efs-monitor
spec:
  containers:
  - name: monitor
    image: ubuntu:latest
    command:
    - /bin/bash
    - -c
    - |
      apt-get update && apt-get install -y nfs-common sysstat
      while true; do
        echo "=== NFS Mount Stats ==="
        mount | grep efs
        echo ""
        echo "=== Network Connections ==="
        ss -tan | grep 2049
        echo ""
        echo "=== I/O Performance ==="
        iostat -x 1 2
        echo ""
        sleep 30
      done
    volumeMounts:
    - name: efs-volume
      mountPath: /mnt/efs
  volumes:
  - name: efs-volume
    persistentVolumeClaim:
      claimName: efs-hiperf-pvc
```

## Example 6: Disabling Multipathing (Fallback)

If you need to disable multipathing for a volume:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: efs-singlepath
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-12345678
  multipathing: "false"  # Explicitly disable multipathing
```

## Testing and Validation

### Check NFS Connections

```bash
# Verify that connections are established to multiple addresses
kubectl exec -it <pod-name> -- ss -tan | grep :2049

# Expected output (multiple entries for different mount target IPs):
# ESTAB 0 0 10.0.1.15:40826 10.0.1.10:2049
# ESTAB 0 0 10.0.2.15:40827 10.0.2.10:2049
# ESTAB 0 0 10.0.3.15:40828 10.0.3.10:2049
```

### Monitor I/O Performance

```bash
# Inside the pod
dd if=/dev/zero of=/mnt/efs/test bs=1M count=1000

# Measure read performance
dd if=/mnt/efs/test of=/dev/null bs=1M count=1000
```

### Check Driver Logs

```bash
# Check multipathing configuration in CSI driver logs
kubectl logs -n kube-system -l app=efs-csi-driver | grep -i multipath

# Expected log entries:
# Building multipath mount options for X mount targets
# Enabled multipathing with X mount targets
# Added multipath mount option: mounttargetip_1=10.0.2.10
```

## Troubleshooting

### Multipathing Not Connecting to All Targets

1. Verify mount targets are in "available" state:
   ```bash
   aws efs describe-mount-targets --file-system-id fs-12345678
   ```

2. Check security group rules allow NFS (2049) on all ENIs

3. Verify network routing is correct for all subnets

### Performance Not Improved

1. Ensure instance has multiple ENIs with good network bandwidth
2. Check NFS mount options are applied correctly
3. Monitor network utilization on each ENI

### Connection Issues

1. Verify all mount targets respond to ping:
   ```bash
   ping 10.0.1.10  # Replace with mount target IPs
   ```

2. Check NFS is accessible on all mount targets:
   ```bash
   showmount -e 10.0.1.10
   ```
