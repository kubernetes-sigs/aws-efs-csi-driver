# V2.0.6
* Updated the docker file to install the latest version of Rust. ([#1414](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1414),[@mskanth972](https://github.com/mskanth972))
* Increase the default Port Range from 400 to 1000. ([#1402](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1402),[@mskanth972](https://github.com/mskanth972))
* Update statefulset example ([#1400](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1400) [@seanzatzdev-amazon](https://github.com/seanzatzdev-amazon))
* Add additionalLabels to node-daemonset ([#1394](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1394) [@omerap12](https://github.com/omerap12))
* Set fips_mode_enabled in efs-utils.conf ([#1344](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1344) [@mpatlasov](https://github.com/mpatlasov))
* make sure the startup taint will eventually being removed after efs driver ready ([#1287](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1287) [@abbshr](https://github.com/abbshr))
* Refactor re-use Access Point ([#1233](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1233) [@otorreno](https://github.com/otorreno))
# V2.0.5
* Add a note to not proceed to the next step until pv STATUS is Bound ([#1075](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1075),[@wafuwafu13](https://github.com/wafuwafu13))
* Add Pod Identity Support ([#1254](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/12541),[@askulkarni2](https://github.com/askulkarni2))
* Add Pod Identity Documentation ([#1381](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1381),[@arnavgup1](https://github.com/arnavgup1))
* Bump Side-cars and add Patch verbs ([#1387](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1387),[@mskanth972](https://github.com/mskanth972))
* Update k8s dependencies ([#1384](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1384),[@mskanth972](https://github.com/mskanth972))
# V2.0.4
* Reap efs-proxy zombie processes. ([#1364](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1364),[@mskanth972](https://github.com/mskanth972))
* Sanitize CSI RPC request logs. ([#1363](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1363),[@mskanth972](https://github.com/mskanth972))
* Edit file paths in provisioning.go to fix failing e2e test. ([#1366](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1366) [@seanzatzdev-amazon](https://github.com/seanzatzdev-amazon))
# V2.0.3
* Expose env, volume, and volume mounts in helm chart for the efs controller and deamonset. ([#1165](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1165), [@cnmcavoy](https://github.com/cnmcavoy))
* Update golang.org dependency. ([#1355](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1355),[@mskanth972](https://github.com/mskanth972))
* efs-utils v2.0.2: Check for efs-proxy PIDs when cleaning tunnel state files. ([#219](https://github.com/aws/efs-utils/pull/219), [@anthotse](https://github.com/anthotse))
# V2.0.2
* Update the ChangeLog to point to latest. ([#1334](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1334), [@mskanth972](https://github.com/mskanth972))
* Fix ARM support for EFS CSI Driver.
# V2.0.1
* Efs-utils v2.0.1. Disable Nagle's algorithm for efs-proxy TLS mounts to improve latencies. ([#210](https://github.com/aws/efs-utils/pull/210), [@RyanStan](https://github.com/RyanStan))
* Updated the default image to be used from Public AWS ECR Repo instead of DockerHub. ([#1323](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1323), [@mskanth972](https://github.com/mskanth972))
# V2.0.0
* Efs-utils v2.0.0 replaces stunnel, which provides TLS encryptions for mounts, with efs-proxy, a component built in-house at AWS. ([#203](https://github.com/aws/efs-utils/pull/203), [@RyanStan](https://github.com/RyanStan))
* Install Rust and Cargo for building efs-utils v2.0.0 ([#1306](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1306), [@RyanStan](https://github.com/RyanStan))
* Update go-restful dependency. ([#1308](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1308), [@RyanStan](https://github.com/RyanStan))
* Adds script + instructions for an in-place upgrade test. ([#1304](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1304), [@seanzatzdev-amazon](https://github.com/seanzatzdev-amazon))
* Update test file manifest paths for e2e tests. ([#1303](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1303), [@seanzatzdev-amazon](https://github.com/seanzatzdev-amazon))
* Bump SIDECARS to the latest. ([#1302](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1302), [@mskanth972](https://github.com/mskanth972))