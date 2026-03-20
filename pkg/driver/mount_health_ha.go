package driver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/klog/v2"
)

type MountParams struct {
	source       string
	target       string
	mountOptions []string
}

func checkMountHealth(ctx context.Context, target string) (bool, error) {
	osCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	mount_health := filepath.Join(target, ".efs_mount_health")
	writeBytes := []byte(fmt.Sprintf("%s: mount I/O health ping at %v", target, time.Now().UTC()))

	go func() {
		defer os.Remove(mount_health)
		err := os.WriteFile(mount_health, writeBytes, 0644)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	case <-osCtx.Done():
		return false, fmt.Errorf("mount point %s health check timed out", target)
	}
}

// High availability and self-heal attempt implementations for unhealthy mount points
func (d *Driver) AsyncMountHealthRecovery(volId, volPath string) {
	if !d.lockManager.lockMutex(volId, 1*time.Millisecond) {
		klog.V(4).Infof("Mount health recovery already in progress for path %s, skipping duplicate attempt", volPath)
		return
	}
	go func() {
		defer d.lockManager.unlockMutex(volId)
		params, ok := volumeMountOptions[volId]
		if !ok {
			klog.Errorf("Mount parameters not found for volume ID %s during health recovery attempt", volId)
			return
		}
		source := params.source
		target := params.target
		mountOptions := params.mountOptions

		klog.Infof("Attempting mount point health recovery for path: %s ", volPath)

		if err := d.mounter.UnmountWithForce(volPath, d.unmountTimeout); err != nil {
			klog.Errorf("Failed to unmount target %s during health recovery: %v", volPath, err)
			return
		}

		// Attempt to remount the target path
		if err := d.mounter.Mount(source, target, "efs", mountOptions); err != nil {
			klog.Errorf("Failed to remount target %s during health recovery: %v", volPath, err)
			return
		}
		health, err := checkMountHealth(context.Background(), volPath)
		if err != nil {
			klog.Errorf("Mount health check failed for path %s after recovery attempt: %v", volPath, err)
			return
		}
		if !health {
			klog.Errorf("Mount health recovery attempt did not restore healthy status for vol %s, path %s", volId, volPath)
			return
		}
		klog.Infof("Mount health recovery successful for vol %s", volId)
		updateVolMetricsWithHealth(volId, health)
		return
	}()
}
