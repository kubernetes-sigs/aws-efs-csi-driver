# EFS CSI Driver Design

## Table of Contents

- [Architecture](#architecture)
  - [Controller Service](#controller-service)
  - [Node Service](#node-service)
- [Provisioning Workflows](#provisioning-workflows)
  - [Static Provisioning](#static-provisioning)
  - [Dynamic Provisioning](#dynamic-provisioning)
---

## Architecture

The driver is deployed as two Kubernetes workloads: a **Controller Deployment** and a **Node DaemonSet**. Both share the same binary which registers all three CSI services (Identity, Controller, Node) on a single gRPC server.

```mermaid
graph TD
    subgraph "CSI Controller (Control Plane)"
        CC[CSI Controller Pod]
        EP1[EFS Plugin]
        CSIProv[CSI Provisioner]
        LP[Liveness Probe]

        CC --> EP1
        CC --> CSIProv
        CC --> LP
    end

    subgraph "CSI Node DaemonSet (Every Node)"
        CN[CSI Node Pod]
        EP2[EFS Plugin]
        CDR[CSI Driver Registrar]
        LP2[Liveness Probe]

        CN --> EP2
        CN --> CDR
        CN --> LP2
    end

    subgraph "Kubernetes Control Plane"
        KC[Kubernetes Controller]
        Kubelet[Kubelet]
    end

    subgraph "EFS Components"
        EFS[EFS File System]
        EU[EFS Utils]
    end

    KC --> CSIProv
    CSIProv --> EP1
    EP1 --> EFS

    Kubelet --> CDR
    Kubelet --> EP2
    EP2 --> EU
    EU --> EFS

    style EP1 fill:#e1f5fe
    style EP2 fill:#e1f5fe
    style EFS fill:#f3e5f5
```

### Controller Service

The Controller runs as a Kubernetes **Deployment** (typically 2 replicas with leader election). It handles cluster-wide volume lifecycle operations.

**Containers:**

| Container | Owner | Responsibility |
|---|---|---|
| `efs-plugin` | EFS team | Implements `CreateVolume` / `DeleteVolume` RPCs. Creates and deletes EFS access points via EFS APIs. |
| `csi-provisioner` | kubernetes-csi | Watches PVCs, translates Kubernetes events into CSI Controller RPCs. |
| `liveness-probe` | kubernetes-csi | Exposes health endpoint for Kubernetes liveness checks. |

### Node Service

The Node runs as a **DaemonSet** on every worker node. It handles the actual mount/unmount of EFS file systems into pod containers.

**Containers:**

| Container | Owner | Responsibility |
|---|---|---|
| `efs-plugin` | EFS team | Implements `NodePublishVolume` / `NodeUnpublishVolume`. Invokes efs-utils to mount/unmount. |
| `node-driver-registrar` | kubernetes-csi | Registers the CSI driver with kubelet on the node. |
| `liveness-probe` | kubernetes-csi | Exposes health endpoint for Kubernetes liveness checks. |

All EFS mounts on a given node are managed by a single `efs-csi-node` pod. If a node has 20 application pods, all mount operations and `efs-proxy` processes run within that one pod.

## Provisioning Workflows

### Static Provisioning

The administrator manually creates the EFS file system and Kubernetes PV. The CSI driver is only involved at mount time (Node Service).

```mermaid
sequenceDiagram
    participant C as Customer
    participant EFS as EFS File System
    participant K8s as Kubernetes
    participant App as Application Pod

    C->>EFS: 1. Create File System
    C->>K8s: 2. Create PV (with FS ID)
    C->>K8s: 3. Create PVC
    K8s->>K8s: 4. Bind PV to PVC
    C->>K8s: 5. Create Pod with PVC
    K8s->>App: 6. Schedule Pod
    App->>EFS: 7. Access via Mount Path

    Note over C,App: Customer manages file system<br/>and directory structure manually
```

### Dynamic Provisioning

The CSI Controller automatically provisions access points as PVCs are created. This workflow involves both Controller and Node services.

```mermaid
sequenceDiagram
    actor User
    participant K8sAPI as Kubernetes API
    participant CSIP as CSI Provisioner
    participant CSIDC as EFS CSI Controller
    participant Kubelet
    participant CSINode as EFS CSI Node
    participant EFS

    rect rgb(240, 248, 255)
    Note over User,EFS: Provisioning Phase
    User->>K8sAPI: Creates StorageClass
    User->>K8sAPI: Creates PVC
    User->>K8sAPI: Creates Pod with PVC reference
    CSIP->>CSIP: Watches PVC creation
    CSIP->>CSIDC: Calls CreateVolume RPC
    CSIDC->>CSIDC: Parse CreateVolumeRequest parameters from StorageClass
    CSIDC->>CSIDC: Determine UID/GID from StorageClass or allocate from range
    CSIDC->>EFS: CreateAccessPoint API call (includes UID/GID, directory path, permissions)
    EFS-->>CSIDC: AccessPoint created with ID
    CSIDC->>CSIDC: Generate VolumeId (fileSystemId::accessPointId)
    CSIDC-->>CSIP: Return CreateVolumeResponse with VolumeId
    CSIP->>K8sAPI: Create PV object with VolumeId
    K8sAPI->>K8sAPI: Bind PVC to PV
    K8sAPI->>Kubelet: Schedule Pod with bound PVC
    end

    rect rgb(240, 255, 240)
    Note over User,EFS: Node Publish Phase
    K8sAPI->>Kubelet: Schedule Pod to node
    Kubelet->>CSINode: Call NodePublishVolume RPC
    CSINode->>CSINode: Parse VolumeId (extract fileSystemId, accessPointId)
    CSINode->>CSINode: Process volume context and mount options
    CSINode->>EFS: Mount via efs-utils mount.efs (with accesspoint=ID, tls options)
    EFS-->>CSINode: Filesystem mounted at target path
    CSINode-->>Kubelet: NodePublishVolume success
    Kubelet->>Kubelet: Start pod containers with mounted volume
    end

    rect rgb(255, 245, 238)
    Note over User,EFS: Deletion Phase
    User->>K8sAPI: Deletes PVC
    CSIP->>CSIP: Watches PVC deletion
    CSIP->>CSIDC: Calls DeleteVolume RPC
    CSIDC->>CSIDC: Parse VolumeId to extract accessPointId
    opt deleteAccessPointRootDir enabled
        CSIDC->>EFS: Temporarily mount filesystem
        CSIDC->>CSIDC: Delete access point root directory and contents
        CSIDC->>EFS: Unmount filesystem
    end
    CSIDC->>EFS: DeleteAccessPoint API call
    EFS-->>CSIDC: Access Point deleted
    CSIDC-->>CSIP: Return DeleteVolumeResponse
    CSIP->>K8sAPI: Delete PV object
    end
```

