# v1.3.4

### Bug Fixes
* Only reap zombie stunnel processes ([#514](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/514), [@wongma7](https://github.com/wongma7))

# v1.3.3

### Misc.
* Fast-forward to latest ebs hack/e2e scripts with eksctl support, k8s 1.20, etc. ([#510](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/510), [@wongma7](https://github.com/wongma7))
* Add node/daemonset service account to helm chart ([#512](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/512), [@wongma7](https://github.com/wongma7))
* Fix (and format) log collector script ([#525](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/525), [@wongma7](https://github.com/wongma7))
* Fix node-serviceaccount.yaml missing from kustomize ([#527](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/527), [@wongma7](https://github.com/wongma7))

# v1.3.2

### Misc.
* Bump release version for multi-arch support. 

# v1.3.1

## Notable changes
- efs-csi-driver now supports arm and image is multi-arch

### Bug Fixes
* Fixed the error message ([#487](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/487), [@pierluigilenoci](https://github.com/pierluigilenoci))

### Misc.
* Clean up unnecessary resources after installation in docker file ([#483](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/483), [@kbasv](https://github.com/kbasv))
* Remove platform hardcode for golang in Dockerfile ([#485](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/485), [@kbasv](https://github.com/kbasv))
* Update cross account mount example with specs and add missing setup step ([#488](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/488), [@kbasv](https://github.com/kbasv))

# v1.3

## Notable changes
- efs-csi-driver now supports cross account and cross `az` mount
- Helm chart clean up

### New Features
* Add support for cross account mount ([#470](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/470), [@kbasv](https://github.com/kbasv))
* Helm chart: make more fields configurable for the deployment, daemonset and storage class ([#406](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/406), [@Misteur-Z](https://github.com/Misteur-Z))
* Add x-AZ mount support for efs-csi-driver ([#425](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/425), [@kbasv](https://github.com/kbasv))

### Bug Fixes
* Fix helm chart external-provisioner RBAC not being created if serviceaccount.controller.create false ([#386](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/386), [@wongma7](https://github.com/wongma7))
* Fix creation of multiple storage classes in Helm chart ([#388](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/388), [@NilsGriebner](https://github.com/NilsGriebner))
* Fix verify command ([#424](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/424), [@buptliuwei](https://github.com/buptliuwei))
* Helm chart: fix reclaimPolicy and volumeBindingMode ([#464](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/464), [@devinsmith911](https://github.com/devinsmith911))
* Put comments back in place inside the values file ([#475](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/475), [@pierluigilenoci](https://github.com/pierluigilenoci))

### Improvements
* feat: add helm config to enable deleteAccessPointRootDir ([#412](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/412), [@KarstenSiemer](https://github.com/KarstenSiemer))
* feat: add controller access point tags to helm chart ([#413](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/413), [@KarstenSiemer](https://github.com/KarstenSiemer))
* feat: helm add storageclass annotations ([#414](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/414), [@KarstenSiemer](https://github.com/KarstenSiemer))
* Add fargate support in the EFS CSI driver ([#418](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/418), [@anonymint](https://github.com/anonymint))
* Install efs-utils from github ([#442](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/442), [@kbasv](https://github.com/kbasv))
* Update access point root directory name to use PV name ([#448](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/448), [@kbasv](https://github.com/kbasv))

### Misc.
* Add documentation and examples for cross-account mount ([#477](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/477), [@kbasv](https://github.com/kbasv))
* Add `hostNetwork: true` on efs-csi-controller deployement. ([#380](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/380), [@jihed](https://github.com/jihed))
* Bump sidecar images to latest available in ecr ([#382](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/382), [@wongma7](https://github.com/wongma7))
* Add `iam` mount option while deleting Access Point root directory ([#422](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/422), [@kbasv](https://github.com/kbasv))
* Add empty StorageClasses from static example ([#423](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/423), [@Yu-HaoYu](https://github.com/Yu-HaoYu))
* Reduce default log level to 2 ([#426](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/426), [@wongma7](https://github.com/wongma7))
* Create a new AWS session with the region ([#435](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/435), [@bodgit](https://github.com/bodgit))
* Change controller port 9808->9909 to avoid node/ebs conflict ([#437](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/437), [@wongma7](https://github.com/wongma7))
* Add kbasv as approver ([#447](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/447), [@kbasv](https://github.com/kbasv))


# v1.2.1

### Bug fixes
* Revert efs-utils to 1.28.2 ([#385](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/385), [@wongma7](https://github.com/wongma7))

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
