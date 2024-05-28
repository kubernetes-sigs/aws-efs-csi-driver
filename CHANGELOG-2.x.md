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