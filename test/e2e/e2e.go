package e2e

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	"k8s.io/kubernetes/test/e2e/storage/testpatterns"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
	"k8s.io/kubernetes/test/e2e/storage/utils"
)

var (
	// Parameters that are expected to be set by consumers of this package.
	// If FileSystemId is not set, ClusterName and Region must be set so that a
	// file system can be created
	ClusterName  string
	Region       string
	FileSystemId string

	deleteFileSystem = false

	// DeployDriver if set true will deploy a stable version of the driver before
	// tests. For CI it should be false because something else ought to deploy an
	// unstable version of the driver to be tested.
	DeployDriver  = false
	destroyDriver = false
)

type efsDriver struct {
	driverInfo testsuites.DriverInfo
}

var _ testsuites.TestDriver = &efsDriver{}

// TODO implement Inline (unless it's redundant) and DynamicPV
// var _ testsuites.InlineVolumeTestDriver = &efsDriver{}
var _ testsuites.PreprovisionedPVTestDriver = &efsDriver{}

func InitEFSCSIDriver() testsuites.TestDriver {
	return &efsDriver{
		driverInfo: testsuites.DriverInfo{
			Name:                 "efs.csi.aws.com",
			SupportedFsType:      sets.NewString(""),
			SupportedMountOption: sets.NewString("tls", "ro"),
			Capabilities: map[testsuites.Capability]bool{
				testsuites.CapPersistence: true,
				testsuites.CapExec:        true,
				testsuites.CapMultiPODs:   true,
				testsuites.CapRWX:         true,
			},
		},
	}
}

func (e *efsDriver) GetDriverInfo() *testsuites.DriverInfo {
	return &e.driverInfo
}

func (e *efsDriver) SkipUnsupportedTest(testpatterns.TestPattern) {}

func (e *efsDriver) PrepareTest(f *framework.Framework) (*testsuites.PerTestConfig, func()) {
	cancelPodLogs := testsuites.StartPodLogs(f)

	return &testsuites.PerTestConfig{
			Driver:    e,
			Prefix:    "efs",
			Framework: f,
		}, func() {
			cancelPodLogs()
		}
}

func (e *efsDriver) CreateVolume(config *testsuites.PerTestConfig, volType testpatterns.TestVolType) testsuites.TestVolume {
	return nil
}

func (e *efsDriver) GetPersistentVolumeSource(readOnly bool, fsType string, volume testsuites.TestVolume) (*v1.PersistentVolumeSource, *v1.VolumeNodeAffinity) {
	pvSource := v1.PersistentVolumeSource{
		CSI: &v1.CSIPersistentVolumeSource{
			Driver:       e.driverInfo.Name,
			VolumeHandle: FileSystemId,
		},
	}
	return &pvSource, nil
}

// List of testSuites to be executed in below loop
var csiTestSuites = []func() testsuites.TestSuite{
	testsuites.InitVolumesTestSuite,
	testsuites.InitVolumeIOTestSuite,
	testsuites.InitVolumeModeTestSuite,
	testsuites.InitSubPathTestSuite,
	testsuites.InitProvisioningTestSuite,
	testsuites.InitMultiVolumeTestSuite,
}

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	// Validate parameters
	if FileSystemId == "" && (Region == "" || ClusterName == "") {
		ginkgo.Fail("FileSystemId is empty and can't create an EFS filesystem because both Region and ClusterName are empty")
	}

	if FileSystemId == "" {
		ginkgo.By(fmt.Sprintf("Creating EFS filesystem in region %q for cluster %q", Region, ClusterName))

		c := NewCloud(Region)
		id, err := c.CreateFileSystem(ClusterName)
		if err != nil {
			framework.ExpectNoError(err, "creating file system")
		}

		FileSystemId = id
		ginkgo.By(fmt.Sprintf("Created EFS filesystem %q in region %q for cluster %q", FileSystemId, Region, ClusterName))
		deleteFileSystem = true
	} else {
		ginkgo.By(fmt.Sprintf("Using already-created EFS file system %q", FileSystemId))
	}

	if DeployDriver {
		cs, err := framework.LoadClientset()
		framework.ExpectNoError(err, "loading kubernetes clientset")

		_, err = cs.StorageV1beta1().CSIDrivers().Get("efs.csi.aws.com", metav1.GetOptions{})
		if err == nil {
			// CSIDriver exists, assume driver has already been deployed
			ginkgo.By("Using already-deployed EFS CSI driver")
		} else if err != nil && !apierrors.IsNotFound(err) {
			// Non-NotFound errors are unexpected
			framework.ExpectNoError(err, "getting csidriver efs.csi.aws.com")
		} else {
			ginkgo.By("Deploying EFS CSI driver")
			framework.RunKubectlOrDie("apply", "-k", "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/k8s/v0.3.0/overlays/dockerhub/?ref=master")
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
		framework.RunKubectlOrDie("delete", "-k", "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/k8s/v0.3.0/overlays/dockerhub/?ref=master")
	}
})

var _ = ginkgo.Describe("[efs-csi] EFS CSI", func() {
	driver := InitEFSCSIDriver()
	ginkgo.Context(testsuites.GetDriverNameWithFeatureTags(driver), func() {
		testsuites.DefineTestSuite(driver, csiTestSuites)
	})

	f := framework.NewDefaultFramework("efs")

	ginkgo.Context(testsuites.GetDriverNameWithFeatureTags(driver), func() {
		ginkgo.It("should mount different paths on same volume on same node", func() {
			ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with no subpath"))
			pvcRoot, pvRoot, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-root", "/")
			framework.ExpectNoError(err, "creating efs pvc & pv with no subpath")
			defer func() { _ = f.ClientSet.CoreV1().PersistentVolumes().Delete(pvRoot.Name, &metav1.DeleteOptions{}) }()

			ginkgo.By(fmt.Sprintf("Creating pod to make subpaths /a and /b"))
			pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcRoot}, false, "mkdir /mnt/volume1/a && mkdir /mnt/volume1/b")
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(pod)
			framework.ExpectNoError(err, "creating pod")
			framework.ExpectNoError(e2epod.WaitForPodSuccessInNamespace(f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod success")

			ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with subpath /a"))
			pvcA, pvA, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-a", "/a")
			framework.ExpectNoError(err, "creating efs pvc & pv with subpath /a")
			defer func() { _ = f.ClientSet.CoreV1().PersistentVolumes().Delete(pvA.Name, &metav1.DeleteOptions{}) }()

			ginkgo.By(fmt.Sprintf("Creating efs pvc & pv with subpath /b"))
			pvcB, pvB, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name+"-b", "/b")
			framework.ExpectNoError(err, "creating efs pvc & pv with subpath /b")
			defer func() { _ = f.ClientSet.CoreV1().PersistentVolumes().Delete(pvB.Name, &metav1.DeleteOptions{}) }()

			ginkgo.By("Creating pod to mount subpaths /a and /b")
			pod = e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvcA, pvcB}, false, "")
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(pod)
			framework.ExpectNoError(err, "creating pod")
			framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod running")
		})

		ginkgo.It("should continue reading/writing without hanging after the driver pod is restarted", func() {
			ginkgo.By(fmt.Sprintf("Creating efs pvc & pv"))
			pvc, pv, err := createEFSPVCPV(f.ClientSet, f.Namespace.Name, f.Namespace.Name, "")
			framework.ExpectNoError(err, "creating efs pvc & pv")
			defer func() { _ = f.ClientSet.CoreV1().PersistentVolumes().Delete(pv.Name, &metav1.DeleteOptions{}) }()

			node, err := e2enode.GetRandomReadySchedulableNode(f.ClientSet)
			framework.ExpectNoError(err, "getting random ready schedulable node")
			command := fmt.Sprintf("touch /mnt/volume1/%s-%s && trap exit TERM; while true; do sleep 1; done", f.Namespace.Name, time.Now().Format(time.RFC3339))

			ginkgo.By(fmt.Sprintf("Creating pod on node %q to mount pvc %q and run %q", node.Name, pvc.Name, command))
			pod := e2epod.MakePod(f.Namespace.Name, nil, []*v1.PersistentVolumeClaim{pvc}, false, command)
			pod, err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(pod)
			framework.ExpectNoError(err, "creating pod")
			framework.ExpectNoError(e2epod.WaitForPodNameRunningInNamespace(f.ClientSet, pod.Name, f.Namespace.Name), "waiting for pod running")

			ginkgo.By(fmt.Sprintf("Getting driver pod on node %q", node.Name))
			labelSelector := labels.SelectorFromSet(labels.Set{"app": "efs-csi-node"}).String()
			fieldSelector := fields.SelectorFromSet(fields.Set{"spec.nodeName": node.Name}).String()
			podList, err := f.ClientSet.CoreV1().Pods("kube-system").List(
				metav1.ListOptions{
					LabelSelector: labelSelector,
					FieldSelector: fieldSelector,
				})
			framework.ExpectNoError(err, "getting driver pod")
			framework.ExpectEqual(len(podList.Items), 1, "expected 1 efs csi node pod but got %d", len(podList.Items))
			driverPod := podList.Items[0]

			ginkgo.By(fmt.Sprintf("Deleting driver pod %q on node %q", driverPod.Name, node.Name))
			err = e2epod.DeletePodWithWaitByName(f.ClientSet, driverPod.Name, "kube-system")
			framework.ExpectNoError(err, "deleting driver pod")

			ginkgo.By(fmt.Sprintf("Execing a write via the pod on node %q", node.Name))
			command = fmt.Sprintf("touch /mnt/volume1/%s-%s", f.Namespace.Name, time.Now().Format(time.RFC3339))
			done := make(chan bool)
			go func() {
				defer ginkgo.GinkgoRecover()
				utils.VerifyExecInPodSucceed(f, pod, command)
				done <- true
			}()
			select {
			case <-done:
				framework.Logf("verified exec in pod succeeded")
			case <-time.After(30 * time.Second):
				framework.Failf("timed out verifying exec in pod succeeded")
			}
		})
	})
})

func createEFSPVCPV(c clientset.Interface, namespace, name, path string) (*v1.PersistentVolumeClaim, *v1.PersistentVolume, error) {
	pvc, pv := makeEFSPVCPV(namespace, name, path)
	pvc, err := c.CoreV1().PersistentVolumeClaims(namespace).Create(pvc)
	if err != nil {
		return nil, nil, err
	}
	_, err = c.CoreV1().PersistentVolumes().Create(pv)
	if err != nil {
		return nil, nil, err
	}
	return pvc, pv, nil
}

func makeEFSPVCPV(namespace, name, path string) (*v1.PersistentVolumeClaim, *v1.PersistentVolume) {
	pvc := makeEFSPVC(namespace, name)
	pv := makeEFSPV(name, path)
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

func makeEFSPV(name, path string) *v1.PersistentVolume {
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
					Driver:       "efs.csi.aws.com",
					VolumeHandle: volumeHandle,
				},
			},
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
		},
	}
}
