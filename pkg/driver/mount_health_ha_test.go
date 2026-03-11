package driver

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
	"k8s.io/mount-utils"
)

type FakeMounter struct {
	mount.FakeMounter
}
type MockLockManager struct {
	locks map[string]bool
}

func NewMockLockManager() *MockLockManager {
	return &MockLockManager{
		locks: make(map[string]bool),
	}
}

var (
	volPath = "/var/lib/kubelet/pods/12345678-1234-1234-1234-123456789012/volumes/efs/test-vol"
	volId   = "fs-12345678"
)

func TestAsyncMountHealthRecoveryConcurrency(t *testing.T) {

	mockCtr := gomock.NewController(t)
	defer mockCtr.Finish()
	mockMounter := mocks.NewMockMounter(mockCtr)

	d := &Driver{
		lockManager: NewLockManagerMap(),
		mounter:     mockMounter,
	}

	mu.Lock()
	volumeMountOptions[volId] = MountParams{
		source: "fs-12345678:/",
		target: volPath,
	}
	mu.Unlock()

	blockThread := make(chan struct{})

	mockMounter.EXPECT().
		UnmountWithForce(volPath, gomock.Any()).Do(func(interface{}, interface{}) {
		<-blockThread
	}).Return(nil).Times(1)

	mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), "efs", gomock.Any()).Return(nil).Times(1)

	// Simulate concurrent health recovery attempts
	for i := 0; i < 3; i++ {
		go d.AsyncMountHealthRecovery(volId, volPath)
	}

	time.Sleep(50 * time.Millisecond)
	close(blockThread)
	time.Sleep(900 * time.Millisecond)
}

func TestAsyncMountHealthRecoveryHang(t *testing.T) {

	mockCtr := gomock.NewController(t)
	defer mockCtr.Finish()
	mockMounter := mocks.NewMockMounter(mockCtr)

	d := &Driver{
		lockManager: NewLockManagerMap(),
		mounter:     mockMounter,
	}

	mu.Lock()
	volumeMountOptions[volId] = MountParams{
		source: "fs-12345678:/",
		target: volPath,
	}
	mu.Unlock()

	blockThread := make(chan struct{})

	mockMounter.EXPECT().
		UnmountWithForce(volPath, gomock.Any()).Do(func(interface{}, interface{}) {
		<-blockThread
	}).Return(nil).Times(1)

	mockMounter.EXPECT().Mount(gomock.Any(), gomock.Any(), "efs", gomock.Any()).Return(nil).Times(1)

	go d.AsyncMountHealthRecovery(volId, volPath)

	time.Sleep(100 * time.Millisecond)

	if d.lockManager.lockMutex(volId, 10*time.Millisecond) {
		t.Errorf("Expected lock to be held during AsyncMountHealthRecovery long run or hang, but it was acquired")
	} else {
		t.Logf("Lock is correctly held during AsyncMountHealthRecovery long run or hang")
	}

	close(blockThread)
	time.Sleep(100 * time.Millisecond)
}

func TestAsyncMountHealthRecoveryRaceDeletion(t *testing.T) {

	mockCtr := gomock.NewController(t)
	defer mockCtr.Finish()
	mockMounter := mocks.NewMockMounter(mockCtr)

	d := &Driver{
		lockManager: NewLockManagerMap(),
		mounter:     mockMounter,
	}

	mu.Lock()
	volumeMountOptions[volId] = MountParams{
		source: "fs-12345678:/",
		target: volPath,
	}
	mu.Unlock()

	pauseUnmount := make(chan struct{})

	mockMounter.EXPECT().
		UnmountWithForce(volPath, gomock.Any()).Do(func(interface{}, interface{}) {
		mu.Lock()
		delete(volumeMountOptions, volId)
		mu.Unlock()
		close(pauseUnmount)
	}).Return(nil).Times(1)

	mockMounter.EXPECT().Mount("", "", "efs", gomock.Any()).Return(fmt.Errorf("invalid params")).AnyTimes()

	go d.AsyncMountHealthRecovery(volId, volPath)

	select {
	case <-pauseUnmount:
		t.Log("Successfully simulated deletion during recovery")
	case <-time.After(5 * time.Second):
		t.Error("Test timed out")
	}
}

func resetGlobals() {
	mu.Lock()
	defer mu.Unlock()
	volumeMountOptions = make(map[string]MountParams)
}
