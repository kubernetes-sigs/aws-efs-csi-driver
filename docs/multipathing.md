# EFS CSI Driver Multipathing Support

## Overview

The AWS EFS CSI Driver now supports **multipathing** to improve throughput and resilience on EC2 instances with multiple network interfaces (ENIs). By binding to multiple mount targets across different network interfaces, you can:

- **Increase I/O throughput**: Distribute load across multiple ENIs for parallel I/O operations
- **Improve resilience**: Provide failover capability if one ENI becomes unavailable
- **Leverage session trunking**: Support RFC 5661 NFS session trunking for advanced multipathing

## Prerequisites

- EFS file system with mount targets in multiple availability zones or subnets
- EC2 instance with multiple ENIs (network interfaces)
- Kubernetes cluster with EFS CSI Driver supporting multipathing

## Configuration

### Enable Multipathing

To enable multipathing for a StorageClass, add the `multipathing` parameter:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: efs-sc-multipath
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemId: fs-12345678
  multipathing: "true"
  maxMultipathTargets: "4"
```

### Parameters

- **`multipathing`** (boolean): Enable multipathing support. Default: `false`
- **`maxMultipathTargets`** (integer): Maximum number of mount targets to bind to. Default: `0` (use all available)

### Example PVC and Pod

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: efs-claim-multipath
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: efs-sc-multipath
  resources:
    storage: 1Gi
---
apiVersion: v1
kind: Pod
metadata:
  name: test-multipath-pod
spec:
  containers:
  - name: app
    image: nginx:latest
    volumeMounts:
    - name: efs-volume
      mountPath: /mnt/efs
  volumes:
  - name: efs-volume
    persistentVolumeClaim:
      claimName: efs-claim-multipath
```

## How Multipathing Works

### Architecture

1. **Mount Target Discovery**: When a volume is created, the driver queries AWS EFS to find all available mount targets for the file system

2. **ENI Detection**: The driver detects available network interfaces (ENIs) on the EC2 instance

3. **Optimal Selection**: Mount targets are selected based on:
   - Availability Zone distribution
   - Number of ENIs per AZ
   - Optional maximum target limit

4. **Session Trunking**: NFS mount options are configured to bind the connection to multiple addresses, enabling session trunking for improved performance

### Mount Options

When multipathing is enabled, the driver generates NFS mount options that specify multiple addresses:

```
addr=10.0.1.10,mounttargetip_1=10.0.2.10,mounttargetip_2=10.0.3.10
```

These options tell the NFS client to establish connections across multiple network paths.

## Best Practices

1. **Instance Placement**: Ensure your EC2 instances are launched with multiple ENIs in different subnets
2. **Mount Target Distribution**: Create EFS mount targets across multiple subnets and availability zones
3. **Network Configuration**: Ensure proper security group rules allow NFS traffic (port 2049) on all ENIs
4. **Monitoring**: Monitor NFS connection metrics to verify multipathing is active

## Troubleshooting

### Multipathing Not Working

Check the CSI driver logs for multipathing-related messages:

```bash
kubectl logs -n kube-system -l app=efs-csi-driver --tail=100 | grep -i multipath
```

Expected log entries:
- "Building multipath mount options for X mount targets"
- "Added multipath mount option"
- "Enabled multipathing with X mount targets"

### Mount Failures

If mounting fails with multipathing enabled:

1. Verify all mount targets are in "available" state
2. Check security groups allow NFS traffic on all ENIs
3. Review subnet routing configuration
4. Fall back to single mount target by setting `multipathing: "false"`

## Performance Tuning

### NFS Tuning Parameters

You can add additional NFS tuning options through the PVC mountOptions:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: efs-claim-tuned
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: efs-sc-multipath
  resources:
    storage: 1Gi
  mountOptions:
    - rsize=1048576
    - wsize=1048576
    - timeo=600
```

### Recommended Settings

- `rsize=1048576` - Read buffer size (1MB)
- `wsize=1048576` - Write buffer size (1MB)
- `timeo=600` - Timeout in deciseconds (60 seconds)
- `retrans=2` - Number of retransmissions

## Limitations

1. **Single AZ Limitation**: Currently, mount targets must be reachable from the same network namespace
2. **Cross-Account**: Multipathing requires standard credentials (cross-account with custom DNS is not yet supported)
3. **NFS Version**: Session trunking features may vary by NFS client version

## Migration

### From Single Path to Multipath

To migrate existing volumes to multipath:

1. Update StorageClass with multipathing parameters
2. Create new PVC using updated StorageClass
3. Migrate data from old PVC to new PVC (if needed)
4. Delete old PVC

## Examples

### High-Performance Workload with Multipathing

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: efs-sc-hiperf
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
  name: efs-claim-hiperf
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: efs-sc-hiperf
  resources:
    storage: 100Gi
  mountOptions:
    - rsize=1048576
    - wsize=1048576
    - timeo=600
    - retrans=2
```

## Feedback and Contributing

To report issues or contribute improvements to multipathing support, please open an issue in the [aws-efs-csi-driver](https://github.com/kubernetes-sigs/aws-efs-csi-driver) repository.
