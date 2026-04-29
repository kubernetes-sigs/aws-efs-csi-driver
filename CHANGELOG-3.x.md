# V3.0.0
* Add support for Amazon S3 Files ([#1828](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1828), [@DavidXU12345](https://github.com/DavidXU12345))
* Add debugLogs param to increase verbose level and enable debug logging in efs-utils
* Disallow crossaccount to be manually configured in mountOptions
* Remove updateStrategy configuration
* Deprecate path as volume attribute
* Honor stderrthreshold when logtostderr is enabled ([#1822](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1822), [@pierluigilenoci](https://github.com/pierluigilenoci))
* Add stricter filtering for filesystem accesspoint([#1796](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1796), [@DavidXU12345](https://github.com/DavidXU12345))

# V3.0.1
* Upgrade sidecar and go dependencies to fix critical CVEs ([#1847](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1847), [@DavidXU12345](https://github.com/DavidXU12345))
* Add helm updateStrategy to DaemonSet and strategy to Deployment ([#1846](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1846), [@camaeel](https://github.com/camaeel))
* Validate mountTargetIp as a valid IP address in NodePublishVolume ([#1844](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1844), [@DavidXU12345](https://github.com/DavidXU12345))

# V3.1.0
* Make hyperpod nodes retrieve metadata from kubernetes api instead of IMDS ([#1820](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1820), [@zmaguire](https://github.com/zmaguire))
* fix: Avoid random mounttargetip for cross-account EFS mounts ([#1861](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1861), [@vmishra22](https://github.com/vmishra22))
* Add FIPS validation: disallow users to set useFIPS when they are in non-US/CA regions ([#1862](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1862), [@DavidXU12345](https://github.com/DavidXU12345))
* Expose S3Files and EFS CloudWatch logs enabled config in CSI driver ([#1866](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1866), [@DavidXU12345](https://github.com/DavidXU12345))
