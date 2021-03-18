# v1.2

## Notable changes
- efs-csi-driver now supports dynamic provisioning 

### New features
* Implement dynamic provisioning ([#274](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/274), [#297](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/297), [@kbasv](https://github.com/kbasv))
* Add tags to efs resources provisioned by driver ([#309](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/309), [@kbasv](https://github.com/kbasv))
  
### Improvements
* Bump efs-utils version to 1.29.1-1 ([#366](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/366), [@kbasv](https://github.com/kbasv))
* Daemonset Affinity for fargate nodes ([#329](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/329), [@benmccown-amz](https://github.com/benmccown-amz))


# v1.1.1

### Bug fixes
* Bump AL2 to 20210126.0 ([#326](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/326), [@wongma7](https://github.com/wongma7))

# v1.1.0

## Notable changes
- The hostPath directory where the driver DaemonSet Pods write state files to their respective Node hosts has changed to fix the driver not working on Bottlerocket OS Nodes. No matter what OS your Nodes are running, you must use a supported method like helm or kustomize to update the driver. If not, i.e. if you only change the image tag of your DaemonSet, the migration from old to new directory won't succeed. See "change config dir location" below for details.

### New Features
* Implement NodeGetVolumeStats ([#238](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/238), [@kbasv](https://github.com/kbasv))

### Bug fixes
* change config dir location ([#286](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/286), [@webern](https://github.com/webern))

# v1.0.0
[Documentation](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/v1.0.0/docs/README.md)

filename  | sha512 hash
--------- | ------------
[v1.0.0.zip](https://github.com/kubernetes-sigs/aws-efs-csi-driver/archive/v1.0.0.zip) | `ecce6558e9a5ea3a94578eefc6cbe4fedc452fefeb60ff1b7f07257143c7a03b2410ad1ffc2b2bbc2065b255cc4b35ec56a09bb3f1561ebe90f8a763da12b19d`
[v1.0.0.tar.gz](https://github.com/kubernetes-sigs/aws-efs-csi-driver/archive/v1.0.0.tar.gz) | `e31defba0d0975a8a3ba4661881638b4cfb45e0b76d1c0d714b7a556eecdab77562b7dda461b6b86350a11946548a42057f1453bd5934d0299f54923e335294b` v1.0.0

## Notable changes
### New Features
* Support access points on the same file system ([#185](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/185), [@2uasimojo](https://github.com/2uasimojo))
* Add encryptInTransit volume attribute ([#205](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/205), [@wongma7](https://github.com/wongma7))

### Bug fixes
* Adding amd64 as nodeSelector to avoid arm64 archtectures (#143) ([#144](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/144), [@hugoprudente](https://github.com/hugoprudente))
* Update efs-utils to 1.26-3.amzn2 and amazonlinux to 2.0.20200602.0 ([#216](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/216), [@wongma7](https://github.com/wongma7))

### Improvements
* Add example for Access Points ([#153](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/153), [@2uasimojo](https://github.com/2uasimojo))
* Pin dependency library versions ([#154](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/154), [@2uasimojo](https://github.com/2uasimojo))
* Bump livenessprobe and node-driver-registrar versions ([#155](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/155), [@wongma7](https://github.com/wongma7))
* Updated node.yaml to update deprecated node selectors ([#158](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/158), [@nithu0115](https://github.com/nithu0115))
* Only publish if access type is 'mount', not 'block' ([#164](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/164), [@wongma7](https://github.com/wongma7))
* Upgraded CSI spec to v1.2.0 ([#169](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/169), [@jqmichael](https://github.com/jqmichael))
* Bump k8s dependencies to 1.17.6 ([#174](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/174), [@wongma7](https://github.com/wongma7))
* added helm repo yaml in docs folder ([#194](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/194), [@kferrone](https://github.com/kferrone))
* Push image to 7 digit git sha tag instead of latest tag for master branch changes ([#202](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/202), [@wongma7](https://github.com/wongma7))
* Started treating the efs-utils config dir stateful and also handles the static files installed at image build time ([#212](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/212), [@jqmichael](https://github.com/jqmichael))
* Build and push every master commit to master tag ([#215](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/215), [@wongma7](https://github.com/wongma7))
