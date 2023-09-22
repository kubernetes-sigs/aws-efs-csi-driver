# Additional Node DaemonSets Feature

In some situations, it is desirable to create multiple node `DaemonSet`s of the EFS CSI Driver. For example, when different AWS nodes in the cluster have different kubelet directory locations.

The EFS CSI Driver Helm chart supports the creation of additional `DaemonSet`s via the `.additionalDaemonSets` parameter. Node configuration from the values supplied to `.node` are taken as a default, with the values supplied in the `.additionalDaemonSets` configuration as overrides. An additional `DaemonSet` will be rendered for each entry in `additionalDaemonSets`.

**WARNING: The EFS CSI Driver does not support running multiple node pods on the same node. If you use this feature, ensure that all nodes are targeted by no more than one `DaemonSet`s.**

## Example

For example, the following configuration would produce two `DaemonSet`s:

```yaml
node:
  # Number for the log level verbosity
  logLevel: 2
  volMetricsOptIn: false
  volMetricsRefreshPeriod: 240
  volMetricsFsRateLimit: 5
  
additionalDaemonSets:
  nodeGroup1:
    kubelet: /mnt/resource/kubelet
    tolerations:
      - operator: Exists
    nodeSelector: { }
    affinity:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
            - matchExpressions:
                - key: eks.amazonaws.com/compute-type
                  operator: NotIn
                  values:
                    - fargate
                - key: kubelet.kubernetes.io/directory-location
                  operator: In
                  values:
                    - mnt-resource-kubelet
                - key: kubernetes.io/os
                  operator: In
                  values:
                    - linux
```

The `DaemonSet`s would be configured as follows:

- `efs-csi-node` (the default `DaemonSet`)  
Will be configured to use default /var/lib/kubelet as the KubeletDirectory.
- `efs-csi-node-nodeGroup1`  
Will be configured to use configured /mnt/resource/kubelet as the KubeletDirectory.
Note how the other config values is inherited from the `.node` configuration because this config does not specify them. This way, `.node` can be used to set defaults for all the `DaemonSet`s.