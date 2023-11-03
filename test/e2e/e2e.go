package e2e

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/util/rand"
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
	storageframework "k8s.io/kubernetes/test/e2e/storage/framework"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
	"k8s.io/kubernetes/test/e2e/storage/utils"
	admissionapi "k8s.io/pod-security-admission/api"
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

	// CreateFileSystem if set true will create a file system before tests.
	// Alternatively, provide an existing file system via FileSystemId. If this
	// is true, ClusterName and Region must be set. For CI it should be true
	// because there is no existing long-lived file system in the CI environment.
	CreateFileSystem bool
	deleteFileSystem bool

	// DeployDriver if set true will deploy a stable version of the driver before
	// tests. For CI it should be false because something else ought to deploy an
	// unstable version of the driver to be tested.
	DeployDriver  bool
	destroyDriver bool
)

type efsDriver struct {
	driverInfo storageframework.DriverInfo
}

var _ storageframework.TestDriver = &efsDriver{}

// TODO implement Inline (unless it's redundant)
// var _ testsuites.InlineVolumeTestDriver = &efsDriver{}
var _ storageframework.PreprovisionedPVTestDriver = &efsDriver{}
var _ storageframework.DynamicPVTestDriver = &efsDriver{}

func InitEFSCSIDriver() storageframework.TestDriver {
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
	}
}

func (e *efsDriver) GetDriverInfo() *storageframework.DriverInfo {
	return &e.driverInfo
}

func (e *efsDriver) SkipUnsupportedTest(storageframework.TestPattern) {}

func (e *efsDriver) PrepareTest(f *framework.Framework) *storageframework.PerTestConfig {
	cancelPodLogs := utils.StartPodLogs(f, f.Namespace)
	ginkgo.DeferCleanup(cancelPodLogs)
	return &storageframework.PerTestConfig{
		Driver:    e,
		Prefix:    "efs",
		Framework: f,
	}
}

func (e *efsDriver) CreateVolume(config *storageframework.PerTestConfig, volType storageframework.TestVolType) storageframework.TestVolume {
	return nil
}

func (e *efsDriver) GetPersistentVolumeSource(readOnly bool, fsType string, volume storageframework.TestVolume) (*v1.PersistentVolumeSource, *v1.VolumeNodeAffinity) {
	pvSource := v1.PersistentVolumeSource{
		CSI: &v1.CSIPersistentVolumeSource{
			Driver:       e.driverInfo.Name,
			VolumeHandle: FileSystemId,
		},
	}
	return &pvSource, nil
}

func (e *efsDriver) GetDynamicProvisionStorageClass(config *storageframework.PerTestConfig, fsType string) *storagev1.StorageClass {
	parameters := map[string]string{
		"provisioningMode": "efs-ap",
		"fileSystemId":     FileSystemId,
		"directoryPerms":   "777",
	}

	generateName := fmt.Sprintf("efs-csi-dynamic-sc-test1234-")

	defaultBindingMode := storagev1.VolumeBindingImmediate
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			//GenerateName: generateName,
			Name: generateName + generateRandomString(4),
		},
		Provisioner:       "efs.csi.aws.com",
		Parameters:        parameters,
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
	return []byte(FileSystemId)
}, func(data []byte) {
	// allNodesBody: each node needs to set its FileSystemId as returned by node 1
	FileSystemId = string(data)
})

var _ = ginkgo.SynchronizedAfterSuite(func() {
	// allNodesBody: do nothing because only node 1 needs to delete EFS
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

	if destroyDriver {
		ginkgo.By("Cleaning up EFS CSI driver")
		kubectl.RunKubectlOrDie("delete", "-k", "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=master")
	}
})

var _ = ginkgo.Describe("[efs-csi] EFS CSI", func() {
	ginkgo.BeforeEach(func() {
		if FileSystemId == "" {
			ginkgo.Fail("FileSystemId is empty. Set it to an existing file system. Or set CreateFileSystem, Region and ClusterName so that the test can create a new file system.")
		}
	})

	driver := InitEFSCSIDriver()
	ginkgo.Context(storageframework.GetDriverNameWithFeatureTags(driver), func() {
		storageframework.DefineTestSuites(driver, csiTestSuites)
	})

	f := framework.NewDefaultFramework("efs")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	ginkgo.Context(storageframework.GetDriverNameWithFeatureTags(driver), func() {
		ginkgo.It("should mount different paths on same volume on same node", func() {
			ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with no subpath"))
			pvcRoot, pvRoot, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-root", "/", map[string]string{})
			framework.ExpectNoError(err, "creating efs pvc & pv with no subpath")
			defer func() {
				_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pvRoot.Name, metav1.DeleteOptions{})
			}()

			ginkgo.By(fmt.Sprintf("Creating pod to make subpaths /a and /b"))
			pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcRoot}, false, "mkdir -p /mnt/volume1/a && mkdir -p /mnt/volume1/b")
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err, "creating pod")
			framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod success")

			ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with subpath /a"))
			pvcA, pvA, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-a", "/a", map[string]string{})
			framework.ExpectNoError(err, "creating efs pvc & pv with subpath /a")
			defer func() {
				_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pvA.Name, metav1.DeleteOptions{})
			}()

			ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with subpath /b"))
			pvcB, pvB, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-b", "/b", map[string]string{})
			framework.ExpectNoError(err, "creating efs pvc & pv with subpath /b")
			defer func() {
				_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pvB.Name, metav1.DeleteOptions{})
			}()

			ginkgo.By("Creating pod to mount subpaths /a and /b")
			pod = e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcA, pvcB}, false, "")
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err, "creating pod")
			framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod running")
		})

		ginkgo.It("should continue reading/writing without interruption after the driver pod is restarted", func() {
			const FilePath = "/mnt/testfile.txt"
			const TestDuration = 30 * time.Second

			ginkgo.By("Creating EFS PVC and associated PV")
			pvc, pv, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name, "", map[string]string{})
			framework.ExpectNoError(err)
			defer f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})

			ginkgo.By("Deploying a pod to write data")
			writeCommand := fmt.Sprintf("while true; do date +%%s >> %s; sleep 1; done", FilePath)
			pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, false, writeCommand)
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(f.ClientSet, pod.Name, f.Namespace.Name))
			defer f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})

			ginkgo.By("Triggering a restart for the EFS CSI Node DaemonSet")
			_, err = kubectl.RunKubectl("kube-system", "rollout", "restart", "daemonset", "efs-csi-node")
			framework.ExpectNoError(err)

			time.Sleep(TestDuration)

			ginkgo.By("Validating no interruption")
			readCommand := fmt.Sprintf("cat %s", FilePath)
			content, err := kubectl.RunKubectl(f.Namespace.Name, "exec", pod.Name, "--", "/bin/sh", "-c", readCommand)
			framework.ExpectNoError(err)

			timestamps := strings.Split(strings.TrimSpace(content), "\n")
			checkInterruption(timestamps)
		})

		testEncryptInTransit := func(f *framework.Framework, encryptInTransit *bool) {
			ginkgo.By("Creating efs pvc & pv")
			volumeAttributes := map[string]string{}
			if encryptInTransit != nil {
				if *encryptInTransit {
					volumeAttributes["encryptInTransit"] = "true"
				} else {
					volumeAttributes["encryptInTransit"] = "false"
				}
			}
			pvc, pv, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name, "/", volumeAttributes)
			framework.ExpectNoError(err, "creating efs pvc & pv with no subpath")
			defer func() {
				_ = f.ClientSet.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
			}()

			// If mount.efs is passed option tls, the mount table entry should be...
			// 127.0.0.1:/ on /mnt/volume1 type nfs4 (rw,relatime,vers=4.1,rsize=1048576,wsize=1048576,namlen=255,hard,noresvport,proto=tcp,port=20052,timeo=600,retrans=2,sec=sys,clientaddr=127.0.0.1,local_lock=none,addr=127.0.0.1)
			// Note the tls option is not actually there. The proof that tls is passed
			// to mount.efs is the server is 127.0.0.1
			// (stunnel proxy running on localhost)
			// instead of the EFS DNS name
			// (file-system-id.efs.aws-region.amazonaws.com).
			// Call `mount` alone first to print it for debugging.
			command := "mount && mount | grep /mnt/volume1 | grep 127.0.0.1"
			if encryptInTransit != nil {
				if !*encryptInTransit {
					command = fmt.Sprintf("mount && mount | grep /mnt/volume1 | grep %v", FileSystemId)
				}
			}
			ginkgo.By(fmt.Sprintf("Creating pod to mount pvc %q and run %q", pvc.Name, command))
			pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, false, command)
			pod.Spec.RestartPolicy = v1.RestartPolicyNever
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err, "creating pod")

			err = e2epod.WaitForPodSuccessInNamespace(f.ClientSet, pod.Name, f.Namespace.Name)
			logs, _ := e2epod.GetPodLogs(f.ClientSet, f.Namespace.Name, pod.Name, "write-pod")
			framework.Logf("pod %q logs:\n %v", pod.Name, logs)
			framework.ExpectNoError(err, "waiting for pod success")
		}

		ginkgo.It("should mount with option tls when encryptInTransit unset", func() {
			testEncryptInTransit(f, nil)
		})

		ginkgo.It("should mount with option tls when encryptInTransit set true", func() {
			encryptInTransit := true
			testEncryptInTransit(f, &encryptInTransit)
		})

		ginkgo.It("should mount without option tls when encryptInTransit set false", func() {
			encryptInTransit := false
			testEncryptInTransit(f, &encryptInTransit)
		})

		ginkgo.It("should successfully perform dynamic provisioning", func() {

			ginkgo.By("Creating EFS Storage Class, PVC and associated PV")
			params := map[string]string{
				"provisioningMode":      "efs-ap",
				"fileSystemId":          FileSystemId,
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
			pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, false, writeCommand)
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err, "creating pod")
			framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod success")
			_ = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})

			ginkgo.By("Deploying a second pod that reads the data")
			pod = e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, false, "while true; do echo $(date -u); sleep 5; done")
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err, "creating pod")
			framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod running")

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

	})
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
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
			StorageClassName: &storageClassName,
		},
	}
}

func GetStorageClass(params map[string]string) *storagev1.StorageClass {
	parameters := params

	generateName := fmt.Sprintf("efs-csi-dynamic-sc-test1234-")

	defaultBindingMode := storagev1.VolumeBindingImmediate
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: generateName + generateRandomString(4),
		},
		Provisioner:       "efs.csi.aws.com",
		Parameters:        parameters,
		VolumeBindingMode: &defaultBindingMode,
	}
}

func createEFSPVCPV(c clientset.Interface, namespace, name, path string, volumeAttributes map[string]string) (*v1.PersistentVolumeClaim, *v1.PersistentVolume, error) {
	pvc, pv := makeEFSPVCPV(namespace, name, path, volumeAttributes)
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

func makeEFSPVCPV(namespace, name, path string, volumeAttributes map[string]string) (*v1.PersistentVolumeClaim, *v1.PersistentVolume) {
	pvc := makeEFSPVC(namespace, name)
	pv := makeEFSPV(name, path, volumeAttributes)
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
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
			StorageClassName: &storageClassName,
		},
	}
}

func makeEFSPV(name, path string, volumeAttributes map[string]string) *v1.PersistentVolume {
	volumeHandle := FileSystemId
	if path != "" {
		volumeHandle += ":" + path
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
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
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

// checkInterruption takes a slice of strings, where each string is expected to
// be an integer representing a timestamp. It checks that the difference between each successive
// pair of integers is not greater than 1.
//
// This function is used to check that reading/writing to a file was not
// interrupted for more than 1 second at a time, even when the driver pod is
// restarted.
func checkInterruption(timestamps []string) {
	var curr int64
	var err error

	for i, t := range timestamps {
		if i == 0 {
			curr, err = strconv.ParseInt(t, 10, 64)
			framework.ExpectNoError(err)
			continue
		}

		next, err := strconv.ParseInt(t, 10, 64)
		framework.ExpectNoError(err)
		if next-curr > 1 {
			framework.Failf("Detected an interruption. Time gap: %d seconds.", next-curr)
		}

		curr = next
	}
}
