package e2e

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework/kubectl"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2epv "k8s.io/kubernetes/test/e2e/framework/pv"
	storageframework "k8s.io/kubernetes/test/e2e/storage/framework"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
	"k8s.io/kubernetes/test/e2e/storage/utils"
	admissionapi "k8s.io/pod-security-admission/api"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
)

var (
	// Parameters that are expected to be set by consumers of this package.
	ClusterName                 string
	Region                      string
	FileSystemId                string
	FileSystemName              string
	MountTargetSecurityGroupIds []string
	MountTargetSubnetIds        []string
	EfsDriverNamespace          string
	EfsDriverLabelSelectors     map[string]string

	// S3Files parameters
	S3FilesFileSystemId   string
	S3FilesFileSystemName string
	s3FilesResources      *S3FilesResources // Store complete S3Files resources for cleanup

	// Cross-account parameters — when set, StorageClasses include the provisioner
	// secret reference and iam mount option for cross-account EFS access.
	// CrossAccountSecretCrossaccountMode indicates the crossaccount value in the
	// pre-configured cross-account Secret. When set to "true", the test secret
	// has crossaccount=true (efs-utils resolves AZ-specific DNS — requires
	// Route 53 AZ-specific zones in the client VPC). When empty/unset, the
	// secret omits the crossaccount field (defaults to false), and the
	// controller injects mount target IP(s) into the PV volumeAttributes.
	// This flag also controls static PV test behavior:
	//   - "true": PV has crossaccount=true volumeAttribute (+ tls, iam mount opts)
	//   - else:   PV has mounttargetip volumeAttribute (+ tls, iam mount opts)
	// CrossAccountMountTargetIP is the mount target IP used for static PV tests
	// when CrossAccountSecretCrossaccountMode != "true".
	CrossAccountSecretName             string
	CrossAccountAZ                     string
	CrossAccountMountTargetIP          string
	CrossAccountSecretCrossaccountMode string

	// CreateFileSystem if set true will create a file system before tests.
	// Alternatively, provide an existing file system via FileSystemId. If this
	// is true, ClusterName and Region must be set. For CI it should be true
	// because there is no existing long-lived file system in the CI environment.
	CreateFileSystem bool
	deleteFileSystem bool

	// CreateS3FilesFileSystem if set true will create an S3Files file system before tests.
	// Alternatively, provide an existing S3Files file system via S3FilesFileSystemId.
	CreateS3FilesFileSystem bool
	deleteS3FilesFileSystem bool

	// DeployDriver if set true will deploy a stable version of the driver before
	// tests. For CI it should be false because something else ought to deploy an
	// unstable version of the driver to be tested.
	DeployDriver  bool
	destroyDriver bool

	// tlsFlagRe matches the --tls flag in efs-proxy command-line arguments.
	tlsFlagRe = regexp.MustCompile(`(^|\s)--tls(\s|$)`)
)

// FileSystemTestConfig holds test configuration for a specific filesystem type
type FileSystemTestConfig struct {
	FSType           util.FileSystemType
	ProvisioningMode string
}

// Parameterized test configurations
var fileSystemTestConfigs = []FileSystemTestConfig{
	{
		FSType:           util.FileSystemTypeEFS,
		ProvisioningMode: "efs-ap",
	},
	{
		FSType:           util.FileSystemTypeS3Files,
		ProvisioningMode: "s3files-ap",
	},
}

func (c *FileSystemTestConfig) GetFSID() string {
	switch c.FSType {
	case util.FileSystemTypeEFS:
		return FileSystemId
	case util.FileSystemTypeS3Files:
		return S3FilesFileSystemId
	default:
		panic(fmt.Sprintf("Unknown filesystem type: %s", c.FSType))
	}
}

type efsDriver struct {
	driverInfo storageframework.DriverInfo
	config     FileSystemTestConfig
}

var _ storageframework.TestDriver = &efsDriver{}

// TODO implement Inline (unless it's redundant)
// var _ testsuites.InlineVolumeTestDriver = &efsDriver{}
var _ storageframework.PreprovisionedPVTestDriver = &efsDriver{}
var _ storageframework.DynamicPVTestDriver = &efsDriver{}

func initCSIDriver(config FileSystemTestConfig) storageframework.TestDriver {
	return &efsDriver{
		driverInfo: storageframework.DriverInfo{
			Name:            "efs.csi.aws.com",
			SupportedFsType: sets.NewString(""),
			Capabilities: map[storageframework.Capability]bool{
				storageframework.CapPersistence: true,
				storageframework.CapExec:        true,
				storageframework.CapMultiPODs:   true,
				storageframework.CapRWX:         true,
			},
		},
		config: config,
	}
}

func (e *efsDriver) GetDriverInfo() *storageframework.DriverInfo {
	return &e.driverInfo
}

func (e *efsDriver) SkipUnsupportedTest(storageframework.TestPattern) {}

func (e *efsDriver) PrepareTest(ctx context.Context, f *framework.Framework) *storageframework.PerTestConfig {
	cancelPodLogs := utils.StartPodLogs(ctx, f, f.Namespace)
	ginkgo.DeferCleanup(cancelPodLogs)
	return &storageframework.PerTestConfig{
		Driver:    e,
		Prefix:    e.config.FSType.String(),
		Framework: f,
	}
}

func (e *efsDriver) CreateVolume(ctx context.Context, config *storageframework.PerTestConfig, volType storageframework.TestVolType) storageframework.TestVolume {
	return nil
}

func (e *efsDriver) GetPersistentVolumeSource(readOnly bool, fsType string, volume storageframework.TestVolume) (*v1.PersistentVolumeSource, *v1.VolumeNodeAffinity) {
	pvSource := v1.PersistentVolumeSource{
		CSI: &v1.CSIPersistentVolumeSource{
			Driver:       e.driverInfo.Name,
			VolumeHandle: e.config.FSType.String() + ":" + e.config.GetFSID(),
		},
	}
	return &pvSource, nil
}

func (e *efsDriver) GetDynamicProvisionStorageClass(ctx context.Context, config *storageframework.PerTestConfig, fsType string) *storagev1.StorageClass {
	parameters := map[string]string{
		"provisioningMode": e.config.ProvisioningMode,
		"fileSystemId":     e.config.GetFSID(),
		"directoryPerms":   "777",
	}

	mountOptions := applyCrossAccountConfig(parameters)

	generateName := fmt.Sprintf("efs-csi-dynamic-sc-test1234-")

	defaultBindingMode := storagev1.VolumeBindingImmediate
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateName + generateRandomString(4),
		},
		Provisioner:       "efs.csi.aws.com",
		Parameters:        parameters,
		MountOptions:      mountOptions,
		VolumeBindingMode: &defaultBindingMode,
	}
}

func generateRandomString(len int) string {
	rand.Seed(time.Now().UnixNano())
	return rand.String(len)
}

// List of testSuites to be executed in below loop
var csiTestSuites = []func() storageframework.TestSuite{
	testsuites.InitVolumesTestSuite,
	testsuites.InitVolumeIOTestSuite,
	testsuites.InitVolumeModeTestSuite,
	testsuites.InitSubPathTestSuite,
	testsuites.InitProvisioningTestSuite,
	testsuites.InitMultiVolumeTestSuite,
}

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	if CreateFileSystem {
		// Validate parameters
		if Region == "" || ClusterName == "" {
			ginkgo.By("CreateFileSystem is true. Set both Region and ClusterName so that the test can create a new file system. Or set CreateFileSystem false and set FileSystemId to an existing file system.")
			return []byte{}
		}

		if FileSystemId == "" {
			ginkgo.By(fmt.Sprintf("Creating EFS filesystem in region %q for cluster %q", Region, ClusterName))

			c := NewCloud(Region)

			opts := CreateOptions{
				Name:             FileSystemName,
				ClusterName:      ClusterName,
				SecurityGroupIds: MountTargetSecurityGroupIds,
				SubnetIds:        MountTargetSubnetIds,
			}
			id, err := c.CreateFileSystem(opts)
			if err != nil {
				framework.ExpectNoError(err, "creating file system")
			}

			FileSystemId = id
			ginkgo.By(fmt.Sprintf("Created EFS filesystem %q in region %q for cluster %q", FileSystemId, Region, ClusterName))
			deleteFileSystem = true
		} else {
			ginkgo.By(fmt.Sprintf("Using already-created EFS file system %q", FileSystemId))
		}
	}

	if CreateS3FilesFileSystem {
		// Validate parameters
		if Region == "" || ClusterName == "" {
			ginkgo.By("CreateS3FilesFileSystem is true. Set both Region and ClusterName so that the test can create a new S3Files file system. Or set CreateS3FilesFileSystem false and set S3FilesFileSystemId to an existing S3Files file system.")
			return []byte{}
		}

		if S3FilesFileSystemId == "" {
			ginkgo.By(fmt.Sprintf("Creating S3Files filesystem in region %q for cluster %q", Region, ClusterName))

			c := NewCloud(Region)

			opts := CreateOptions{
				Name:        S3FilesFileSystemName,
				ClusterName: ClusterName,
			}
			resources, err := c.CreateS3FilesFileSystem(opts)
			if err != nil {
				framework.ExpectNoError(err, "creating S3Files file system")
			}

			s3FilesResources = resources
			S3FilesFileSystemId = resources.FileSystemId
			ginkgo.By(fmt.Sprintf("Created S3Files filesystem %q in region %q for cluster %q", S3FilesFileSystemId, Region, ClusterName))
			deleteS3FilesFileSystem = true
		} else {
			ginkgo.By(fmt.Sprintf("Using already-created S3Files file system %q", S3FilesFileSystemId))
		}
	}

	if DeployDriver {
		cs, err := framework.LoadClientset()
		framework.ExpectNoError(err, "loading kubernetes clientset")

		_, err = cs.StorageV1beta1().CSIDrivers().Get(context.TODO(), "efs.csi.aws.com", metav1.GetOptions{})
		if err == nil {
			// CSIDriver exists, assume driver has already been deployed
			ginkgo.By("Using already-deployed EFS CSI driver")
		} else if err != nil && !apierrors.IsNotFound(err) {
			// Non-NotFound errors are unexpected
			framework.ExpectNoError(err, "getting csidriver efs.csi.aws.com")
		} else {
			ginkgo.By("Deploying EFS CSI driver")
			kubectl.RunKubectlOrDie("kube-system", "apply", "-k", "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=master")
			ginkgo.By("Deployed EFS CSI driver")
			destroyDriver = true
		}
	}
	return []byte(FileSystemId + "|" + S3FilesFileSystemId)
}, func(data []byte) {
	// allNodesBody: each node needs to set its FileSystemId and S3FilesFileSystemId as returned by node 1
	parts := strings.Split(string(data), "|")
	FileSystemId = parts[0]
	S3FilesFileSystemId = parts[1]
})

var _ = ginkgo.SynchronizedAfterSuite(func() {
	// allNodesBody: do nothing because only node 1 needs to delete EFS and S3Files
}, func() {
	if deleteFileSystem {
		ginkgo.By(fmt.Sprintf("Deleting EFS filesystem %q", FileSystemId))

		c := NewCloud(Region)
		err := c.DeleteFileSystem(FileSystemId)
		if err != nil {
			framework.ExpectNoError(err, "deleting file system")
		}

		ginkgo.By(fmt.Sprintf("Deleted EFS filesystem %q", FileSystemId))
	}

	if deleteS3FilesFileSystem && s3FilesResources != nil {
		ginkgo.By(fmt.Sprintf("Deleting S3Files filesystem %q", S3FilesFileSystemId))

		c := NewCloud(Region)
		err := c.DeleteS3FilesFileSystem(s3FilesResources)
		if err != nil {
			framework.ExpectNoError(err, "deleting S3Files file system")
		}
	}

	if destroyDriver {
		ginkgo.By("Cleaning up EFS CSI driver")
		kubectl.RunKubectlOrDie("delete", "-k", "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=master")
	}
})

var _ = ginkgo.Describe("[efs-csi]", func() {
	f := framework.NewDefaultFramework("efs")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	for _, config := range fileSystemTestConfigs {
		config := config // Local copy
		ginkgo.Context(fmt.Sprintf("[%s]", config.FSType), func() {
			ginkgo.BeforeEach(func() {
				fsId := config.GetFSID()
				if fsId == "" {
					ginkgo.Fail(fmt.Sprintf("%s FileSystemId is empty. Set it to an existing file system.", config.FSType))
				}
			})

			driver := initCSIDriver(config)
			ginkgo.Context(storageframework.GetDriverNameWithFeatureTags(driver)[0].(string), func() {
				storageframework.DefineTestSuites(driver, csiTestSuites)
			})

			ginkgo.Context(storageframework.GetDriverNameWithFeatureTags(driver)[0].(string), func() {
				ginkgo.It("should mount different paths on same volume on same node", func() {
					ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with no subpath"))
					pvcRoot, pvRoot, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-root", "/", map[string]string{}, config)
					framework.ExpectNoError(err, "creating efs pvc & pv with no subpath")
					defer func() {
						_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pvRoot.Name, metav1.DeleteOptions{})
					}()

					ginkgo.By(fmt.Sprintf("Creating pod to make subpaths /a and /b"))
					pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcRoot}, admissionapi.LevelBaseline, "mkdir -p /mnt/volume1/a && mkdir -p /mnt/volume1/b")
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating pod")
					framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod success")

					ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with subpath /a"))
					pvcA, pvA, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-a", "/a", map[string]string{}, config)
					framework.ExpectNoError(err, "creating efs pvc & pv with subpath /a")
					defer func() {
						_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pvA.Name, metav1.DeleteOptions{})
					}()

					ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with subpath /b"))
					pvcB, pvB, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-b", "/b", map[string]string{}, config)
					framework.ExpectNoError(err, "creating efs pvc & pv with subpath /b")
					defer func() {
						_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pvB.Name, metav1.DeleteOptions{})
					}()

					ginkgo.By("Creating pod to mount subpaths /a and /b")
					pod = e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcA, pvcB}, admissionapi.LevelBaseline, "")
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating pod")
					framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod running")
				})

				testEncryptInTransit := func(f *framework.Framework, encryptInTransit *bool) {
					// With efs-utils v2+, both TLS and non-TLS mounts go through efs-proxy on 127.0.0.1.
					// To distinguish them, we check the efs-proxy process args on the driver pod:
					// TLS mounts have the '--tls' flag, non-TLS mounts do not.

					expectTLS := encryptInTransit == nil || *encryptInTransit // default (nil) and true both expect TLS

					ginkgo.By("Creating efs pvc & pv")
					volumeAttributes := map[string]string{}
					if encryptInTransit != nil {
						if *encryptInTransit {
							volumeAttributes["encryptInTransit"] = "true"
						} else {
							volumeAttributes["encryptInTransit"] = "false"
						}
					}
					pvc, pv, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name, "/", volumeAttributes, config)
					framework.ExpectNoError(err, "creating efs pvc & pv with no subpath")
					defer func() {
						_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
					}()
					defer func() {
						_ = f.ClientSet.CoreV1().PersistentVolumeClaims(f.Namespace.Name).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
					}()

					ginkgo.By("Creating a long-running pod to mount the volume")
					pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, "while true; do echo $(date -u); sleep 5; done")
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating pod")
					framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod running")
					defer func() {
						_ = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
					}()

					ginkgo.By("Verifying mount goes through efs-proxy on 127.0.0.1")
					mountOutput := kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", pod.Name, "--", "/bin/sh", "-c", "mount | grep /mnt/volume1")
					framework.Logf("mount output: %s", mountOutput)
					if !strings.Contains(mountOutput, "127.0.0.1") {
						ginkgo.Fail(fmt.Sprintf("Expected mount through efs-proxy (127.0.0.1), got: %s", mountOutput))
					}

					ginkgo.By("Finding the efs-csi-node driver pod on the same node")
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Get(context.TODO(), pod.Name, metav1.GetOptions{})
					framework.ExpectNoError(err, "getting pod to determine node")
					nodeName := pod.Spec.NodeName

					labelSelector := buildLabelSelector(EfsDriverLabelSelectors)
					driverPods, err := f.ClientSet.CoreV1().Pods(EfsDriverNamespace).List(context.TODO(), metav1.ListOptions{
						LabelSelector: labelSelector,
						FieldSelector: "spec.nodeName=" + nodeName,
					})
					framework.ExpectNoError(err, "listing efs-csi-node pods on node")
					if len(driverPods.Items) == 0 {
						ginkgo.Fail(fmt.Sprintf("No efs-csi-node pod found on node %q", nodeName))
					}
					driverPod := driverPods.Items[0]
					framework.Logf("Found efs-csi-node pod %q on node %q", driverPod.Name, nodeName)

					ginkgo.By("Checking efs-proxy process args for --tls flag")
					fsID := config.GetFSID()
					// Match on PVC name to identify the exact efs-proxy process for this test's mount,
					// avoiding false matches from other tests sharing the same filesystem.
					pvcName := pvc.Name

					// Retry: efs-proxy may still be starting when the pod first reaches Running.
					var matchingLine, psOutput string
					waitCtx, waitCancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer waitCancel()
					waitErr := wait.PollUntilContextTimeout(waitCtx, 5*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
						psOutput = strings.TrimSpace(kubectl.RunKubectlOrDie(EfsDriverNamespace, "exec", driverPod.Name, "-c", "efs-plugin", "--", "python3", "-c", "import glob\nfor f in sorted(glob.glob('/proc/[0-9]*/cmdline')):\n try:\n  c=open(f).read().replace('\\x00',' ').strip()\n  if 'efs-proxy' in c: print(c)\n except: pass"))
						framework.Logf("efs-proxy processes:\n%s", psOutput)
						for _, line := range strings.Split(psOutput, "\n") {
							if strings.Contains(line, fsID) && strings.Contains(line, pvcName) {
								matchingLine = line
								return true, nil
							}
						}
						return false, nil
					})
					if waitErr != nil {
						ginkgo.Fail(fmt.Sprintf("Timed out waiting for efs-proxy process for filesystem %q with PVC %q; last ps output:\n%s", fsID, pvcName, psOutput))
					}
					framework.Logf("Matched efs-proxy line: %s", matchingLine)

					hasTLS := tlsFlagRe.MatchString(matchingLine)
					if expectTLS && !hasTLS {
						ginkgo.Fail(fmt.Sprintf("Expected efs-proxy --tls flag for filesystem %q but not found: %s", fsID, matchingLine))
					}
					if !expectTLS && hasTLS {
						ginkgo.Fail(fmt.Sprintf("Expected efs-proxy to NOT have --tls flag for filesystem %q but found: %s", fsID, matchingLine))
					}
					framework.Logf("TLS validation passed: expectTLS=%v, hasTLS=%v, fs=%s", expectTLS, hasTLS, fsID)
				}

				ginkgo.It("should mount with option tls when encryptInTransit unset", func() {
					testEncryptInTransit(f, nil)
				})

				ginkgo.It("should mount with option tls when encryptInTransit set true", func() {
					encryptInTransit := true
					testEncryptInTransit(f, &encryptInTransit)
				})

				ginkgo.It("should mount without option tls when encryptInTransit set false", func() {
					if config.FSType == util.FileSystemTypeS3Files {
						ginkgo.Skip(fmt.Sprintf("encryptInTransit is not supported for %s", config.FSType))
					}
					encryptInTransit := false
					testEncryptInTransit(f, &encryptInTransit)
				})

				ginkgo.It("should successfully perform dynamic provisioning", func() {

					ginkgo.By("Creating EFS Storage Class, PVC and associated PV")
					params := map[string]string{
						"provisioningMode":      config.ProvisioningMode,
						"fileSystemId":          config.GetFSID(),
						"subPathPattern":        "${.PVC.name}",
						"directoryPerms":        "700",
						"gidRangeStart":         "1000",
						"gidRangeEnd":           "2000",
						"basePath":              "/dynamic_provisioning",
						"ensureUniqueDirectory": "true",
					}

					sc := GetStorageClass(params)
					sc, err := f.ClientSet.StorageV1().StorageClasses().Create(context.TODO(), sc, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating storage class")
					pvc, err := createEFSPVCPVDynamicProvisioning(f.ClientSet, f.Namespace.Name, f.Namespace.Name, sc.Name)
					framework.ExpectNoError(err, "creating pvc")

					ginkgo.By("Deploying a pod that applies the PVC and writes data")
					testData := "DP TEST"
					writePath := "/mnt/volume1/out"
					writeCommand := fmt.Sprintf("echo \"%s\" >> %s", testData, writePath)
					pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, writeCommand)
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating pod")
					framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod success")
					_ = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})

					ginkgo.By("Deploying a second pod that reads the data")
					pod = e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, "while true; do echo $(date -u); sleep 5; done")
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating pod")
					framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod running")

					readCommand := fmt.Sprintf("cat %s", writePath)
					output := kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", pod.Name, "--", "/bin/sh", "-c", readCommand)
					output = strings.TrimSuffix(output, "\n")
					framework.Logf("The output is: %s", output)

					defer func() {
						_ = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
						_ = f.ClientSet.CoreV1().PersistentVolumeClaims(f.Namespace.Name).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
						_ = f.ClientSet.StorageV1().StorageClasses().Delete(context.TODO(), sc.Name, metav1.DeleteOptions{})
					}()

					if output == "" {
						ginkgo.Fail("Read data is empty.")
					}
					if output != testData {
						ginkgo.Fail("Read data does not match write data.")
					}
				})

				ginkgo.It("should reuse access point when reuseAccessPoint is enabled", func() {
					if config.FSType != util.FileSystemTypeEFS {
						ginkgo.Skip("reuseAccessPoint is only supported for EFS filesystem type")
					}
					pvcName := "reuse-ap-test-" + generateRandomString(4)
					testData := "REUSE AP TEST"
					writePath := "/mnt/volume1/out"

					ginkgo.By("Creating StorageClass with reuseAccessPoint enabled and Retain reclaim policy")
					params := map[string]string{
						"provisioningMode": config.ProvisioningMode,
						"fileSystemId":     config.GetFSID(),
						"directoryPerms":   "700",
						"basePath":         "/reuse_ap_test",
						"reuseAccessPoint": "true",
					}
					sc := GetStorageClass(params)
					retainPolicy := v1.PersistentVolumeReclaimRetain
					sc.ReclaimPolicy = &retainPolicy
					sc, err := f.ClientSet.StorageV1().StorageClasses().Create(context.TODO(), sc, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating storage class")
					defer f.ClientSet.StorageV1().StorageClasses().Delete(context.TODO(), sc.Name, metav1.DeleteOptions{})

					ginkgo.By("Creating first PVC and writing data")
					pvc1, err := createEFSPVCPVDynamicProvisioning(f.ClientSet, f.Namespace.Name, pvcName, sc.Name)
					framework.ExpectNoError(err, "creating first pvc")

					writeCommand := fmt.Sprintf("echo \"%s\" > %s", testData, writePath)
					pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc1}, admissionapi.LevelBaseline, writeCommand)
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating write pod")
					framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name), "waiting for write pod success")
					_ = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})

					ginkgo.By("Recording the PV name and VolumeHandle from first PVC")
					pvc1, err = f.ClientSet.CoreV1().PersistentVolumeClaims(f.Namespace.Name).Get(context.TODO(), pvc1.Name, metav1.GetOptions{})
					framework.ExpectNoError(err, "getting first pvc")
					firstPVName := pvc1.Spec.VolumeName
					firstPV, err := f.ClientSet.CoreV1().PersistentVolumes().Get(context.TODO(), firstPVName, metav1.GetOptions{})
					framework.ExpectNoError(err, "getting first pv")
					firstVolumeHandle := firstPV.Spec.CSI.VolumeHandle
					framework.Logf("First PV name: %s, VolumeHandle: %s", firstPVName, firstVolumeHandle)

					ginkgo.By("Deleting first PVC and waiting for it to be fully removed")
					_ = f.ClientSet.CoreV1().PersistentVolumeClaims(f.Namespace.Name).Delete(context.TODO(), pvc1.Name, metav1.DeleteOptions{})
					err = wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
						_, err := f.ClientSet.CoreV1().PersistentVolumeClaims(f.Namespace.Name).Get(ctx, pvc1.Name, metav1.GetOptions{})
						if apierrors.IsNotFound(err) {
							return true, nil
						}
						return false, err
					})
					framework.ExpectNoError(err, "waiting for first pvc deletion")

					ginkgo.By("Deleting first PV and waiting for it to be fully removed")
					_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), firstPVName, metav1.DeleteOptions{})
					framework.ExpectNoError(e2epv.WaitForPersistentVolumeDeleted(context.TODO(), f.ClientSet, firstPVName, 5*time.Second, 2*time.Minute), "waiting for first pv deletion")

					ginkgo.By("Creating second PVC with the same name to trigger access point reuse")
					pvc2, err := createEFSPVCPVDynamicProvisioning(f.ClientSet, f.Namespace.Name, pvcName, sc.Name)
					framework.ExpectNoError(err, "creating second pvc")
					var secondPV *v1.PersistentVolume
					defer func() {
						// Delete PVC first so the Delete reclaim policy triggers CSI DeleteVolume for access point cleanup
						_ = f.ClientSet.CoreV1().PersistentVolumeClaims(f.Namespace.Name).Delete(context.TODO(), pvc2.Name, metav1.DeleteOptions{})
						if secondPV != nil {
							_ = e2epv.WaitForPersistentVolumeDeleted(context.TODO(), f.ClientSet, secondPV.Name, 5*time.Second, 2*time.Minute)
						}
					}()

					ginkgo.By("Verifying the same access point was reused")
					pvs, err := e2epv.WaitForPVClaimBoundPhase(context.TODO(), f.ClientSet, []*v1.PersistentVolumeClaim{pvc2}, 5*time.Minute)
					framework.ExpectNoError(err, "waiting for second pvc to be bound")
					secondPV = pvs[0]
					secondVolumeHandle := secondPV.Spec.CSI.VolumeHandle
					framework.Logf("Second PV name: %s, VolumeHandle: %s", secondPV.Name, secondVolumeHandle)

					// Switch reclaim policy to Delete so the reclaim controller cleans up the access point when PVC is deleted
					secondPV.Spec.PersistentVolumeReclaimPolicy = v1.PersistentVolumeReclaimDelete
					_, err = f.ClientSet.CoreV1().PersistentVolumes().Update(context.TODO(), secondPV, metav1.UpdateOptions{})
					framework.ExpectNoError(err, "updating second pv reclaim policy to Delete")
					if firstVolumeHandle != secondVolumeHandle {
						ginkgo.Fail(fmt.Sprintf("Access point was not reused: first VolumeHandle %q != second VolumeHandle %q", firstVolumeHandle, secondVolumeHandle))
					}

					ginkgo.By("Reading data from second PVC to verify access point was reused")
					readCommand := fmt.Sprintf("cat %s", writePath)
					pod = e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc2}, admissionapi.LevelBaseline, "while true; do echo $(date -u); sleep 5; done")
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating read pod")
					defer f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
					framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name), "waiting for read pod running")

					output := kubectl.RunKubectlOrDie(f.Namespace.Name, "exec", pod.Name, "--", "/bin/sh", "-c", readCommand)
					output = strings.TrimSuffix(output, "\n")
					framework.Logf("Read output: %s", output)

					if output == "" {
						ginkgo.Fail("Read data is empty -- access point was not reused.")
					}
					if output != testData {
						ginkgo.Fail(fmt.Sprintf("Read data %q does not match written data %q -- access point may not have been reused.", output, testData))
					}
				})

				ginkgo.It("should continue reading/writing after the driver pod is restarted", ginkgo.Serial, func() {
					const FilePath = "/mnt/testfile.txt"
					const TestDuration = 30 * time.Second

					ginkgo.By("Creating EFS PVC and associated PV")
					pvc, pv, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name, "", map[string]string{}, config)
					framework.ExpectNoError(err)
					defer f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})

					ginkgo.By("Deploying a pod to write data")
					writeCommand := fmt.Sprintf("while true; do date +%%s >> %s; sleep 1; done", FilePath)
					pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, writeCommand)
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err)
					framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name))
					defer f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})

					ginkgo.By("Recording timestamp before restart")
					readCommand := fmt.Sprintf("cat %s", FilePath)
					content, err := kubectl.RunKubectl(f.Namespace.Name, "exec", pod.Name, "--", "/bin/sh", "-c", readCommand)
					framework.ExpectNoError(err)
					lines := strings.Split(strings.TrimSpace(content), "\n")
					beforeVal, err := strconv.ParseInt(lines[len(lines)-1], 10, 64)
					framework.ExpectNoError(err)

					ginkgo.By("Triggering a restart for the EFS CSI Node DaemonSet")
					for retries := 0; retries < 3; retries++ {
						_, err = kubectl.RunKubectl("kube-system", "rollout", "restart", "daemonset", "efs-csi-node")
						if err == nil {
							break
						}
						if strings.Contains(err.Error(), "restart has already been triggered") {
							framework.Logf("Rollout restart conflict, retrying in 2s (attempt %d/3)", retries+1)
							time.Sleep(2 * time.Second)
							continue
						}
						break
					}
					framework.ExpectNoError(err)

					time.Sleep(TestDuration)

					ginkgo.By("Validating writes resumed after restart")
					content, err = kubectl.RunKubectl(f.Namespace.Name, "exec", pod.Name, "--", "/bin/sh", "-c", readCommand)
					framework.ExpectNoError(err)
					lines = strings.Split(strings.TrimSpace(content), "\n")
					lastTimestamp, err := strconv.ParseInt(lines[len(lines)-1], 10, 64)
					framework.ExpectNoError(err)

					expectedMin := beforeVal + int64(TestDuration.Seconds())
					framework.Logf("beforeVal=%d, lastTimestamp=%d, expectedMin=%d, TestDuration=%v", beforeVal, lastTimestamp, expectedMin, TestDuration)
					if lastTimestamp < expectedMin {
						ginkgo.Fail(fmt.Sprintf("Writes did not resume after CSI driver restart. Last write was at %d, but expected a write after %d.", lastTimestamp, expectedMin))
					}
				})

				ginkgo.It("should have efs-proxy logs containing ReadBypass enabled", func() {
					if config.FSType != util.FileSystemTypeS3Files {
						ginkgo.Skip("ReadBypass is only applicable to S3Files filesystem type")
					}

					labelSelector := buildLabelSelector(EfsDriverLabelSelectors)
					allDriverPods, err := f.ClientSet.CoreV1().Pods(EfsDriverNamespace).List(context.TODO(), metav1.ListOptions{
						LabelSelector: labelSelector,
					})
					framework.ExpectNoError(err, "listing all efs-csi-node pods")

					ginkgo.By("Clearing efs-proxy logs on all efs-csi-node pods")
					for _, dp := range allDriverPods.Items {
						_, err := kubectl.RunKubectl(EfsDriverNamespace, "exec", dp.Name, "-c", "efs-plugin", "--", "find", "/var/log/amazon/efs", "-name", "fs-*.log*", "-delete")
						if err != nil {
							framework.Logf("Warning: failed to clear efs-proxy logs on pod %q: %v", dp.Name, err)
							continue
						}
						framework.Logf("Cleared efs-proxy logs on pod %q", dp.Name)
					}

					ginkgo.By("Creating EFS PVC and PV")
					pvc, pv, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name, "/", map[string]string{}, config)
					framework.ExpectNoError(err, "creating efs pvc & pv")
					defer func() {
						_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
					}()

					ginkgo.By("Creating a pod to mount the volume")
					pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, admissionapi.LevelBaseline, "while true; do echo $(date -u); sleep 5; done")
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
					framework.ExpectNoError(err, "creating pod")
					framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(context.TODO(), f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod running")
					defer func() {
						_ = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
					}()

					ginkgo.By("Getting the node the pod is running on")
					pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Get(context.TODO(), pod.Name, metav1.GetOptions{})
					framework.ExpectNoError(err, "getting pod")
					nodeName := pod.Spec.NodeName
					framework.Logf("Pod %q is running on node %q", pod.Name, nodeName)

					ginkgo.By("Waiting for efs-proxy to initialize and collect logs. Sleeping for 120 seconds.")
					time.Sleep(120 * time.Second)

					ginkgo.By("Finding the efs-csi-node pod on the same node")
					driverPods, err := f.ClientSet.CoreV1().Pods(EfsDriverNamespace).List(context.TODO(), metav1.ListOptions{
						LabelSelector: labelSelector,
						FieldSelector: "spec.nodeName=" + nodeName,
					})
					framework.ExpectNoError(err, "listing efs-csi-node pods")
					if len(driverPods.Items) == 0 {
						ginkgo.Fail(fmt.Sprintf("No efs-csi-node pod found on node %q with label selector %q", nodeName, labelSelector))
					}
					driverPod := driverPods.Items[0]
					framework.Logf("Found efs-csi-node pod %q on node %q", driverPod.Name, nodeName)

					ginkgo.By("Checking efs-proxy logs for ReadBypass enabled message")
					expectedLog := "S3 bucket accessible. ReadBypass enabled."
					grepCommand := fmt.Sprintf("cat /var/log/amazon/efs/fs-*.log* 2>/dev/null | grep -c '%s' || true", expectedLog)
					output := strings.TrimSpace(kubectl.RunKubectlOrDie(EfsDriverNamespace, "exec", driverPod.Name, "-c", "efs-plugin", "--", "/bin/sh", "-c", grepCommand))
					framework.Logf("Grep count for ReadBypass log: %s", output)

					count, err := strconv.Atoi(output)
					framework.ExpectNoError(err, "parsing grep count")
					if count == 0 {
						// Dump logs for debugging
						allLogs := kubectl.RunKubectlOrDie(EfsDriverNamespace, "exec", driverPod.Name, "-c", "efs-plugin", "--", "/bin/sh", "-c", "cat /var/log/amazon/efs/fs-*.log* 2>/dev/null || echo 'No log files found'")
						framework.Logf("efs-proxy logs:\n%s", allLogs)
						ginkgo.Fail(fmt.Sprintf("Expected efs-proxy logs to contain %q but it was not found", expectedLog))
					}
				})
			})
		})
	}
})

func createEFSPVCPVDynamicProvisioning(c clientset.Interface, namespace, name, storageClassName string) (*v1.PersistentVolumeClaim, error) {
	pvc := makeEFSPVCDynamicProvisioning(namespace, name, storageClassName)
	pvc, err := c.CoreV1().PersistentVolumeClaims(namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

func makeEFSPVCDynamicProvisioning(namespace, name string, storageClassName string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
			StorageClassName: &storageClassName,
		},
	}
}

// applyCrossAccountConfig adds cross-account secret reference, mount options,
// and az parameter to StorageClass parameters when cross-account testing is enabled.
func applyCrossAccountConfig(parameters map[string]string) []string {
	if CrossAccountSecretName == "" {
		return nil
	}
	parameters["csi.storage.k8s.io/provisioner-secret-name"] = CrossAccountSecretName
	parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = "kube-system"
	if CrossAccountAZ != "" {
		parameters["az"] = CrossAccountAZ
	}
	return []string{"tls", "iam"}
}

func GetStorageClass(params map[string]string) *storagev1.StorageClass {
	parameters := params
	mountOptions := applyCrossAccountConfig(parameters)

	generateName := fmt.Sprintf("efs-csi-dynamic-sc-test1234-")

	defaultBindingMode := storagev1.VolumeBindingImmediate
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateName + generateRandomString(4),
		},
		Provisioner:       "efs.csi.aws.com",
		Parameters:        parameters,
		MountOptions:      mountOptions,
		VolumeBindingMode: &defaultBindingMode,
	}
}

func createEFSPVCPV(c clientset.Interface, namespace, name, path string, volumeAttributes map[string]string, config FileSystemTestConfig) (*v1.PersistentVolumeClaim, *v1.PersistentVolume, error) {
	pvc, pv := makeEFSPVCPV(namespace, name, path, volumeAttributes, config)
	pvc, err := c.CoreV1().PersistentVolumeClaims(namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}
	_, err = c.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}
	return pvc, pv, nil
}

func makeEFSPVCPV(namespace, name, path string, volumeAttributes map[string]string, config FileSystemTestConfig) (*v1.PersistentVolumeClaim, *v1.PersistentVolume) {
	pvc := makeEFSPVC(namespace, name)
	pv := makeEFSPV(name, path, volumeAttributes, config)
	pvc.Spec.VolumeName = pv.Name
	pv.Spec.ClaimRef = &v1.ObjectReference{
		Namespace: pvc.Namespace,
		Name:      pvc.Name,
	}
	return pvc, pv
}

func makeEFSPVC(namespace, name string) *v1.PersistentVolumeClaim {
	storageClassName := ""
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
			StorageClassName: &storageClassName,
		},
	}
}

func makeEFSPV(name, path string, volumeAttributes map[string]string, config FileSystemTestConfig) *v1.PersistentVolume {
	volumeHandle := config.FSType.String() + ":" + config.GetFSID()
	if path != "" {
		volumeHandle += ":" + path
	}

	// Cross-account static PV tests: static provisioning bypasses the CSI
	// controller, so we must either (a) use the crossaccount mount option
	// which relies on efs-utils resolving AZ-specific DNS (requires Route 53
	// private hosted zones in the client VPC), or
	// (b) inject the mount target IP directly via mounttargetip
	var mountOptions []string
	if CrossAccountSecretName != "" {
		mountOptions = []string{"tls", "iam"}
		if CrossAccountSecretCrossaccountMode == "true" {
			volumeAttributes["crossaccount"] = "true"
		} else if CrossAccountMountTargetIP != "" {
			volumeAttributes["mounttargetip"] = CrossAccountMountTargetIP
		}
	}

	return &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
			Capacity: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse("1Gi"),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:           "efs.csi.aws.com",
					VolumeHandle:     volumeHandle,
					VolumeAttributes: volumeAttributes,
				},
			},
			MountOptions: mountOptions,
			AccessModes:  []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
		},
	}
}

func makeDir(path string) error {
	err := os.MkdirAll(path, os.FileMode(0777))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func buildLabelSelector(labels map[string]string) string {
	selector := ""
	for k, v := range labels {
		if selector != "" {
			selector += ","
		}
		selector += k + "=" + v
	}
	return selector
}
