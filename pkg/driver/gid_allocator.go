package driver

import (
	"fmt"
	"sync"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type FilesystemID struct {
	gidMin int64
	gidMax int64
}

type GidAllocator struct {
	mu sync.Mutex
}

func NewGidAllocator() *GidAllocator {
	return &GidAllocator{}
}

// Retrieves the next available GID
func (g *GidAllocator) getNextGid(fsId string, accessPoints []*cloud.AccessPoint, gidMin, gidMax int64) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	klog.V(5).Infof("Received getNextGid for fsId: %v, min: %v, max: %v", fsId, gidMin, gidMax)

	usedGids, err := g.getUsedGids(fsId, accessPoints)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "Failed to discover used GIDs for filesystem: %v: %v ", fsId, err)
	}

	gid, err := getNextUnusedGid(usedGids, gidMin, gidMax)

	if err != nil {
		return 0, status.Errorf(codes.Internal, "Failed to locate a free GID for given file system: %v. "+
			"Please create a new storage class with a new file-system", fsId)
	}

	return gid, nil
}

func (g *GidAllocator) getUsedGids(fsId string, accessPoints []*cloud.AccessPoint) (gids []int64, err error) {
	gids = []int64{}
	if len(accessPoints) == 0 {
		return gids, nil
	}
	for _, ap := range accessPoints {
		// This should happen only in tests - skip nil pointers.
		if ap == nil {
			continue
		}
		if ap.PosixUser != nil {
			gids = append(gids, ap.PosixUser.Gid)
		}
	}
	klog.V(5).Infof("Discovered used GIDs: %+v for FS ID: %v", gids, fsId)
	return
}

func getNextUnusedGid(usedGids []int64, gidMin, gidMax int64) (nextGid int64, err error) {
	requestedRange := gidMax - gidMin

	if requestedRange > cloud.AccessPointPerFsLimit {
		overrideGidMax := gidMin + cloud.AccessPointPerFsLimit
		klog.Warningf("Requested GID range (%v:%v) exceeds EFS Access Point limit (%v) per Filesystem. Driver will use limited GID range (%v:%v)", gidMin, gidMax, cloud.AccessPointPerFsLimit, gidMin, overrideGidMax)
		gidMax = overrideGidMax
	}

	var lookup func(usedGids []int64)
	lookup = func(usedGids []int64) {
		for gid := gidMin; gid <= gidMax; gid++ {
			if !slices.Contains(usedGids, gid) {
				nextGid = gid
				return
			}
			klog.V(5).Infof("Allocator found GID which is already in use: %v, trying next one.", gid)
		}
		return
	}

	nextGid = -1
	lookup(usedGids)
	if nextGid == -1 {
		err = fmt.Errorf("allocator failed to find available GID")
		return
	}

	klog.V(5).Infof("Allocator found unused GID: %v", nextGid)
	return
}
