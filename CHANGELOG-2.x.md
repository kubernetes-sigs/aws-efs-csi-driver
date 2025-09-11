# V2.1.12
* Update golang MV in DockerFile ([#1697](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1697), [@mondaljs](https://github.com/mondaljs))
* Update Sidecar Versions to Resolve CVEs ([#1694](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1694), [@samuhale](https://github.com/samuhale))
* Allow : in Tags with Escape Character ([#1693](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1693), [@samuhale](https://github.com/samuhale))
# V2.1.11
* Bump k8 version to default 1.33 for e2e tests([#1681](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1681), [@dankova22](https://github.com/dankova22))
* Update openssl installation change in Dockerfile ([#1678](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1678), [@anthotse](https:///github.com/anthotse))
* Fix spelling errors and modernize deprecated io/ioutil usage ([#1674](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1674), [@oyiz](https://github.com/oyuz))
# V2.1.10
* Update dependencies and fix go.sum ([#1663](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1663), [@anthotse](https://anthotse))
* Implement ControllerModifyVolume function ([#1663](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1663), [@anthotse](https://anthotse))
* Update golang in Dockerfile ([#1663](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1663), [@anthotse](https://anthotse))
# V2.1.9
* Fixing CVEs and depricated pod images ([#1649](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1649)), [@thakurmi](https://github.com/thakurmi)
* unpinned openssl to fix failing build ([#1611](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1638)), [@thakurmi](https://github.com/thakurmi)
* CVE-2025-22869: bump golang.org/x/crypto to v0.35.0 ([#1611](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1611)), [@kunalmemane](https://github.com/kunalmemane)
# V2.1.8
* Remove unused workflow that publishes images to dockerhub ([#1621](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1621), [@jrakas-dev](https://github.com/jrakas-dev))
*  Return existing access point if one already exists during create workflow ([#1620](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1620), [@jrakas-dev](https://github.com/jrakas-dev))
* Clean install openssl and standardize eks distro ([#1619](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1619), [@dankova22](https://github.com/dankova22))
* Fix centos image in pod config examples ([#1611](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1611), [@thakurmi](https://github.com/thakurmi))
# V2.1.7
* Adding additional checks and multi threaded testing to ensure concurrent createVolume and deleteVolume calls are handled correctly ([#1592] (https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1592), [@dluthcke](https://github.com/dluthcke))
* Clarifying Note when uninstalling CSI driver ([#1599] (https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1599), [@dluthcke](https://github.com/dluthcke))
* Update README.md to include uninstall instructions ([#1597] (https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1596), [@dluthcke](https://github.com/dluthcke))
* Updating sidecar versions ([#1596](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1596), [@dluthcke](https://github.com/dluthcke))
* Update K8s version to mitigate CVE-2025-0426 ([#1594](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1594), [@mskanth97](https://github.com/mskanth972))
# V2.1.6
Remove libwrap=no from stunnel config on startup for newer stunnel compatibility ([#1586](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1586/commits/5151feef34da86595a9ccc7e3c960aea537a61dc), [@dankova22](https://github.com/dankova22))
# V2.1.5
* Upgrade golang.net (v0.25.0 -> v0.33.0) ([#1562]https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1562)
* Updated Python distribution to latest version and add symlink to stunnel5 to ensure compatibility ([#1569]https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1569/files)
# V2.1.4
* Upgrade stunnel to 5 ([#1561](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1561), [@dankova22](https://github.com/dankova22))
# V2.1.3
* Fix default value for unhealthyPodEvictionPolicy ([#1524](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1524), [@z0rc](https://github.com/z0rc))
* Switch to adaptive retry mode to reduce throttling errors ([#1520](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1520), [@dankova22](https://github.com/dankova22))
# V2.1.2
* Modify delete access point root directory logic to only remove temporary directory if empty ([#1532](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1532), [@jrakas-dev](https://github.com/jrakas-dev))
* Bump golang.org/x/crypto to v0.31.0 ([#1531](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1531), [@germanparente](https://github.com/germanparente))
* Update k8s dependencies ([#1514](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1514), [@andrewjamesbrown](https://github.com/andrewjamesbrown))
# V2.1.1
* Fix volume delete failure for static provisioning when accessPointId is empty ([#1507](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1507), [@dankova22](https://github.com/dankova22))
* Update Go and dependencies to address CVEs ([#1513](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1513), [@andrewjamesbrown](https://github.com/andrewjamesbrown))
* Add metadata.namespace to chart templates ([#1376](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1376), [@Kellen275](https://github.com/Kellen275))
* Adding new argument for csi provisioner ([#1512](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1512), [@mskanth972](https://github.com/mskanth972))
* Add permissions to all GitHub actions ([#1508](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1508), [@mskanth972](https://github.com/mskanth972))
* Add additional arguments for Side cars ([#1506](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1506), [@mskanth972](https://github.com/mskanth972))
* Fix controller podLabels typo in values.yaml ([#1445](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1445), [@pvickery-ParamountCommerce](https://github.com/pvickery-ParamountCommerce))
* Add anti-affinity for incompatible compute types ([#1496](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1496), [@abhinavmpandey08](https://github.com/abhinavmpandey08))
* Update python base images to newer versions ([#1480](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1480), [@andrewjamesbrown](https://github.com/andrewjamesbrown))
# V2.1.0
* Update CodeQL workflow to v2. ([#1485](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1485),[@mskanth972](https://github.com/mskanth972))
* Bump side-cars to the latest. ([#1484](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1484),[@mskanth972](https://github.com/mskanth972))
* Update kubernetes to version 1.27.16 to patch CVE-2024-5321. ([#1475](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1475),[@mselim00](https://github.com/mselim00))
# V2.0.9
* Upgrade AL2 version and address CVEs (CVE-2024-34156, CVE-2024-34158)
* Fix controller template to support replicaCount, resources, topologySpreadConstraints
* Migrate to aws-sdk-go-v2. ([#1458](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1458), [@avanish23](https://github.com/avanish23))
# V2.0.8
* Update K8s dependencies. ([#1440](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1440), [@retornam](https://github.com/retornam))
* Add flag that enables CSI driver to be added without using helm hooks ([#1074](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1074), [@woehrl01](https://github.com/woehrl01))
* Add new region DNS suffixes to watchdog ([#1455](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1455), [@jdwtf](https://github.com/jdwtf))
* Use protobuf content type instead of JSON for K8s client ([#1451](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1451), [@bhavi-koduru](https://github.com/bhavi-koduru))
# V2.0.7
* Update GO version from 1.20 to 1.22.5 to mitigate CVEs. ([#1427](https://github.com/kubernetes-sigs/aws-efs-csi-driver/pull/1427),[@mskanth972](https://github.com/mskanth972))
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
