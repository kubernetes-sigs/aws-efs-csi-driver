# v0.3.0
[Documentation](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/v0.3.0/docs/README.md)

filename  | sha512 hash
--------- | ------------
[v0.3.0.zip](https://github.com/kubernetes-sigs/aws-efs-csi-driver/archive/v0.3.0.zip) | `08c10d855261d7973e43ca442726e703800f06e23f4b2906f7a0d3433cac4cd12e0c4a3bf809862eabc74082d35ca72fcf7ca9c6c28423e1dd51d0c745607dc3`
[v0.3.0.tar.gz](https://github.com/kubernetes-sigs/aws-efs-csi-driver/archive/v0.3.0.tar.gz) | `cf4765a1b8930d8cf46175e742977a32d5afac03f818dcc1b6909309fd55f331dd84ca1eb546027d1ffefab1d2ac3e6ca4f207cf74749e61b4d74b5921031491`

## Action required
If you are mounting subpath as persistent volume, please update the volume path and set it as part of `volumeHandle` instead of `volumeAttributes`. See [volume path example](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/master/examples/kubernetes/volume_path/specs/example.yaml#L23) for this use case.

## Notable changes
### New features
* Add helm support ([#139](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/139), [@leakingtapan](https://github.com/leakingtapan))
* Switch to use kustomize for manifest ([#88](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/88), [@leakingtapan](https://github.com/leakingtapan))
* Update to read subpath from volume handle ([#102](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/102), [@leakingtapan](https://github.com/leakingtapan))

### Bug fixes
* Migrate to use new test framework ([#96](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/96), [@leakingtapan](https://github.com/leakingtapan))
* Fix bug when unpublishing already unmounted file system ([#106](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/106), [@leakingtapan](https://github.com/leakingtapan))
* Fix bug in e2e test script for sed ([#114](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/114), [@leakingtapan](https://github.com/leakingtapan))
* Preserve efs state file across efs driver recycle ([#135](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/135), [@leakingtapan](https://github.com/leakingtapan))

### Improvements
* Update volume path example for accessing multiple volumes within the same EFS filesystem ([#107](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/107), [@leakingtapan](https://github.com/leakingtapan))
* Add watch dog for efs mount with stunnel ([#113](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/113), [@leakingtapan](https://github.com/leakingtapan))
* Update daemonset tolerations to run on all nodes ([#133](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/133), [@mikegirard](https://github.com/mikegirard))
* Fix of url to Kubernetes CSI Developer Documentation ([#137](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/137), [@DmitriyStoyanov](https://github.com/DmitriyStoyanov))

# v0.2.0
[Documentation](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/v0.2.0/docs/README.md)

filename  | sha512 hash
--------- | ------------
[v0.2.0.zip](https://github.com/kubernetes-sigs/aws-efs-csi-driver/archive/v0.2.0.zip) | `5be59ba16a2ec379059863f94124d6334602200bc57967e3894b9edc65ffc7f634c6bc6915b649ebaa6879ed410c1a80e724a8c731682ae345765941dad7b339`
[v0.2.0.tar.gz](https://github.com/kubernetes-sigs/aws-efs-csi-driver/archive/v0.2.0.tar.gz) | `7048c818d7df82101cecc73d3f28c087c5c758a329eabdc847105ce1d4ccd1c0c43d401054b2074e4678bed4969ebdc6fe169f61021d4cfdfa4b10e577fe058c`

## Changelog

### Notable changes
* Combine manifest files ([#35](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/35), [@leakingtapan](https://github.com/leakingtapan))
* Add example stateful sets ([#43](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/43), [@leakingtapan](https://github.com/leakingtapan))
* Added flag for version information output ([#44](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/44), [@djcass44](https://github.com/djcass44))
* fix namespace in csi-node clusterrole ([#47](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/47), [@d-nishi](https://github.com/d-nishi))
* Update to CSI v1.1.0 ([#48](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/48), [@wongma7](https://github.com/wongma7))
* Add support for 'path' field in volumeContext ([#52](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/52), [@wongma7](https://github.com/wongma7))
* Replace deprecated Recycle policy with Retain ([#53](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/53), [@wongma7](https://github.com/wongma7))
* Add sanity test ([#54](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/54), [@wongma7](https://github.com/wongma7))
* Run upstream e2e tests  ([#55](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/55), [@wongma7](https://github.com/wongma7))
* Add linux nodeSelector to manifest.yaml ([#61](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/61), [@wongma7](https://github.com/wongma7))
* Add liveness probe ([#62](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/62), [@wongma7](https://github.com/wongma7))
* Add example for volume path ([#65](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/65), [@leakingtapan](https://github.com/leakingtapan))
* Upgrade to golang 1.12 ([#70](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/70), [@wongma7](https://github.com/wongma7))

# v0.1.0
[Documentation](https://github.com/kubernetes-sigs/aws-efs-csi-driver/blob/v0.1.0/docs/README.md)

filename  | sha512 hash
--------- | ------------
[v0.1.0.zip](https://github.com/kubernetes-sigs/aws-efs-csi-driver/archive/v0.1.0.zip) | `b2ac6ccedfbd40f7a47ed1c14fb9bc16742592f03c3f51e26ef5f72ed2f97718cae32dca998304f5773c3b0d3df100942817d55bbb09cbd2226a51000cfc1505`
[v0.1.0.tar.gz](https://github.com/kubernetes-sigs/aws-efs-csi-driver/archive/v0.1.0.tar.gz) | `1db081d96906ae07a868cbcf3e3902fe49c44f219966c1f5ba5a8beabd9311e42cae57ff1884edf63b936afce128b113ed94d85afc2e2955dedb81ece99f72dc`

## Changelog

### Notable changes
* Multiple README updates and example updates
* Switch to use klog for logging ([#20](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/20), [@leakingtapan](https://github.com/leakingtapan/))
* Update README and add more examples ([#18](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/18), [@leakingtapan](https://github.com/leakingtapan/))
* Update manifest files ([#12](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/12), [@leakingtapan](https://github.com/leakingtapan/))
* Add sample manifest for multiple pod RWX scenario ([#9](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/9), [@leakingtapan](https://github.com/leakingtapan/))
* Update travis with code verification ([#8](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/8), [@leakingtapan](https://github.com/leakingtapan/))
* Implement mount options support ([#5](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/5), [@leakingtapan](https://github.com/leakingtapan/))
* Update logging format of the driver ([#4](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/4), [@leakingtapan](https://github.com/leakingtapan/))
* Implement node service for EFS driver  ([bca5d36](https://github.com/kubernetes-sigs/aws-efs-csi-driver/commit/bca5d36), [@leakingtapan](https://github.com/leakingtapan/))
