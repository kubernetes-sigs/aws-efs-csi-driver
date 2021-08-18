# Helm chart

# v2.1.4
* Add node.serviceAccount values for creating and/or specifying daemonset service account

# v2.1.3
* Bump app/driver version to `v1.3.2` 

# v2.1.2
* Add extra-create-metadata

# v2.1.1
* Update app/driver version to `v1.3.1`

# v2.1.0

## New features
* Update app/driver version to `v1.3.0`

## Bug fixes
* Put comments back in place inside the values file ([#475](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/475), [@pierluigilenoci](https://github.com/pierluigilenoci))

# v2.0.1

## Bug fixes
* Helm chart: fix reclaimPolicy and volumeBindingMode ([#464](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/464), [@devinsmith911](https://github.com/devinsmith911))


# v2.0.0

## Breaking changes

Multiple changes in values file at `sidecars`, `controller` and `node`

---
```yaml
sidecars:
  xxxxxxxxx:
    repository:
    tag:
```

Moving to

```yaml
sidecars:
  xxxxxxxxx:
    image:
      repository:
      tag:
```

---
```yaml
podAnnotations:
resources:
nodeSelector:
tolerations:
affinity:
```

Moving to

```yaml
controller:
  podAnnotations:
  resources:
  nodeSelector:
  tolerations:
  affinity:
```

---
```yaml
hostAliases:
dnsPolicy:
dnsConfig:
```

Moving to

```yaml
node:
  hostAliases:
  dnsPolicy:
  dnsConfig:
```

---
```yaml
serviceAccount:
  controller:
```

Moving to

```yaml
controller:
  serviceAccount:
```

## New features

* Chart API `v2` (requires Helm 3)
* Set `resources` and `imagePullPolicy` fields independently for containers
* Set `logLevel`, `affinity`, `nodeSelector`, `podAnnotations` and `tolerations` fields independently
for Controller deployment and Node daemonset
* Set `reclaimPolicy` and `volumeBindingMode` fields in storage class

## Fixes

* Fixing Controller deployment using `podAnnotations` and `tolerations` values from Node daemonset
* Let the user define the whole `tolerations` array, default to `- operator: Exists`
* Default `logLevel` lowered from `5` to `2`
* Default `imagePullPolicy` everywhere set to `IfNotPresent`
