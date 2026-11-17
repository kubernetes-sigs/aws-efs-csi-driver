# Additional node DaemonSets

The Helm chart can render more than one node `DaemonSet`. Use it when different groups of nodes
need different node settings, for example a different memory request or port range for nodes
that run many EFS mounts.

`.Values.node` configures the default `DaemonSet`, `efs-csi-node`. Every entry of
`.Values.additionalDaemonSets` renders one more `DaemonSet`, named `efs-csi-node-<entry>`. An
entry has the structure of `.Values.node` and is merged over it, so `.Values.node` holds the
defaults for all `DaemonSet`s.

**Every node must run exactly one `efs-csi-node` pod.** All node `DaemonSet`s register the same
CSI driver on the node. Give every `DaemonSet`, including the default one, a `nodeSelector` or
`affinity` so that their nodes do not overlap. The default `DaemonSet` tolerates all taints
unless `node.tolerations` is changed, so it also lands on tainted nodes.

## How the merge works

| Value type | Result in an additional `DaemonSet` |
|---|---|
| a key the entry does not set | taken from `.Values.node` |
| a scalar (string, number, bool) | the entry's value, `false` and `0` included |
| a map, for example `nodeSelector`, `resources` | merged key by key; the entry's keys win |
| a list, for example `tolerations`, `env` | **replaced** by the entry's list, not extended |

Because maps are merged, a key in `.Values.node.nodeSelector` is also in every additional
`DaemonSet` unless the entry sets the same key. Use the same label key with a different value in
each `DaemonSet`, as in the example below.

All `DaemonSet`s use the node service account, `.Values.node.serviceAccount`.

## Example

Two kinds of nodes: nodes with up to 110 pods and many EFS mounts per pod, and nodes that each
run one workspace with 13 EFS mounts. Each `efs-proxy` uses memory and one local TCP port, so
both are sized by the number of mounts (pods x EFS volumes per pod) on a node.

```yaml
node:
  nodeSelector:
    efs-profile: general
  # 110 pods x 15 mounts = 1,650 local ports
  efsUtilsConfOverrides: "mount:port_range_lower_bound=20049,mount:port_range_upper_bound=21699"
  resources:
    requests:
      memory: 8Gi

additionalDaemonSets:
  workspace:
    nodeSelector:
      efs-profile: workspace
    efsUtilsConfOverrides: "mount:efs_proxy_worker_threads=2"
    resources:
      requests:
        memory: 1536Mi
```

The result:

| `DaemonSet` | Nodes | Port range | Memory request |
|---|---|---|---|
| `efs-csi-node` | `efs-profile: general` | 20049–21699 | 8Gi |
| `efs-csi-node-workspace` | `efs-profile: workspace` | the efs-utils default | 1536Mi |

`efs-csi-node-workspace` takes every other setting from `.Values.node`. Its
`efsUtilsConfOverrides` replaces the default's string as a whole, so it does not carry the port
range.

The labels used here, `efs-profile`, are an example. Any node label works, for example a label
that the node group or NodePool sets on its nodes.
