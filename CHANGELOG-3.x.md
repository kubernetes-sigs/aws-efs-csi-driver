# V3.0.0
* Add support for Amazon S3 Files ([#1828](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1828), [@DavidXU12345](https://github.com/DavidXU12345))
* Add debugLogs param to increase verbose level and enable debug logging in efs-utils
* Disallow crossaccount to be manually configured in mountOptions
* Remove updateStrategy configuration
* Deprecate path as volume attribute
* Honor stderrthreshold when logtostderr is enabled ([#1822](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1822), [@pierluigilenoci](https://github.com/pierluigilenoci))
* Add stricter filtering for filesystem accesspoint([#1796](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1796), [@DavidXU12345](https://github.com/DavidXU12345))
