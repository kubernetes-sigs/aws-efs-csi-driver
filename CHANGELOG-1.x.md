# v1.0.0

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
