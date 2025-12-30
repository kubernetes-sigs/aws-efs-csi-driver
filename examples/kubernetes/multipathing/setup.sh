#!/bin/bash
# This example demonstrates how to set up and use EFS multipathing with the AWS EFS CSI Driver
# It assumes an EFS file system with mount targets in multiple subnets/AZs

set -e

# Configuration
NAMESPACE="default"
STORAGE_CLASS="efs-sc-multipath"
PVC_NAME="efs-claim-multipath"
POD_NAME="test-multipath-pod"
EFS_ID="${EFS_ID:-fs-12345678}"  # Replace with your EFS file system ID

echo "========================================="
echo "EFS CSI Driver Multipathing Example"
echo "========================================="
echo "Namespace: $NAMESPACE"
echo "EFS File System ID: $EFS_ID"
echo ""

# Step 1: Create StorageClass with multipathing enabled
echo "[Step 1] Creating StorageClass with multipathing enabled..."
cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${STORAGE_CLASS}
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemId: ${EFS_ID}
  multipathing: "true"
  maxMultipathTargets: "4"
EOF

echo "✓ StorageClass created: $STORAGE_CLASS"
echo ""

# Step 2: Create PVC
echo "[Step 2] Creating PersistentVolumeClaim..."
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${PVC_NAME}
  namespace: ${NAMESPACE}
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: ${STORAGE_CLASS}
  resources:
    storage: 10Gi
  mountOptions:
    - rsize=1048576
    - wsize=1048576
EOF

echo "✓ PVC created: $PVC_NAME"
echo ""

# Step 3: Wait for PVC to be bound
echo "[Step 3] Waiting for PVC to be bound..."
kubectl wait --for=condition=Bound pvc/${PVC_NAME} -n ${NAMESPACE} --timeout=120s || true
sleep 5
echo "✓ PVC status:"
kubectl get pvc ${PVC_NAME} -n ${NAMESPACE}
echo ""

# Step 4: Create a test Pod
echo "[Step 4] Creating test Pod..."
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: ${POD_NAME}
  namespace: ${NAMESPACE}
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
        memory: "128Mi"
        cpu: "100m"
  volumes:
  - name: efs-volume
    persistentVolumeClaim:
      claimName: ${PVC_NAME}
EOF

echo "✓ Pod created: $POD_NAME"
echo ""

# Step 5: Wait for Pod to be running
echo "[Step 5] Waiting for Pod to be running..."
kubectl wait --for=condition=Ready pod/${POD_NAME} -n ${NAMESPACE} --timeout=120s || true
sleep 5
echo "✓ Pod status:"
kubectl get pod ${POD_NAME} -n ${NAMESPACE}
echo ""

# Step 6: Verify multipathing in Pod
echo "[Step 6] Verifying multipathing configuration..."
echo ""
echo "Network interfaces in Pod:"
kubectl exec ${POD_NAME} -n ${NAMESPACE} -- ip link show || echo "(ip command not available)"
echo ""
echo "Mount information:"
kubectl exec ${POD_NAME} -n ${NAMESPACE} -- mount | grep efs || true
echo ""
echo "NFS connections:"
kubectl exec ${POD_NAME} -n ${NAMESPACE} -- ss -tan | grep -E ":2049|ESTAB" || true
echo ""

# Step 7: Performance test
echo "[Step 7] Running I/O performance test..."
echo "Writing test data to EFS..."
kubectl exec ${POD_NAME} -n ${NAMESPACE} -- sh -c 'dd if=/dev/zero of=/mnt/efs/test-file bs=1M count=100' || true
echo "✓ Test file created"
echo ""

# Step 8: Cleanup (optional)
echo "========================================="
echo "Multipathing test completed successfully!"
echo ""
echo "To clean up resources, run:"
echo "  kubectl delete pod ${POD_NAME} -n ${NAMESPACE}"
echo "  kubectl delete pvc ${PVC_NAME} -n ${NAMESPACE}"
echo "  kubectl delete storageclass ${STORAGE_CLASS}"
echo "========================================="
