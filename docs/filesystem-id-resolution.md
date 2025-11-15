# Dynamic Filesystem ID Resolution

The EFS CSI driver supports dynamic filesystem ID resolution from Kubernetes ConfigMaps and Secrets. This enables workflows where tools provision EFS filesystems and write the filesystem ID to Kubernetes resources, which the CSI driver reads automatically during volume provisioning.

**Reference Format:** `namespace/name/key`

**Parameter Requirements:** Exactly one of `fileSystemId`, `fileSystemIdConfigRef`, or `fileSystemIdSecretRef` must be specified in StorageClass parameters.

**Example with ConfigMap:**
```yaml
# ConfigMap created
apiVersion: v1
kind: ConfigMap
metadata:
  name: efs-config
  namespace: kube-system
data:
  fileSystemId: fs-02604354c13d0316d
---
# StorageClass references the ConfigMap
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: efs-sc
provisioner: efs.csi.aws.com
parameters:
  provisioningMode: efs-ap
  fileSystemIdConfigRef: "kube-system/efs-config/fileSystemId"
  directoryPerms: "700"
```

**Enabling RBAC Permissions (Required):**

This feature requires additional RBAC permissions for the controller service account to read ConfigMaps and Secrets. When installing via Helm, enable with:
```bash
helm install aws-efs-csi-driver ./charts/aws-efs-csi-driver \
  --set controller.fileSystemIdRefs.enabled=true
```

For static manifest installations, manually apply the RBAC resources from the Helm chart templates. 