package driver

import (
	"context"
	"fmt"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"sync"
)

var ACCESS_POINT_PER_FS_LIMIT int = 1000

type FilesystemID struct {
	gidMin int
	gidMax int
}

type GidAllocator struct {
	cloud      cloud.Cloud
	fsIdGidMap map[string]*FilesystemID
	mu         sync.Mutex
}

func NewGidAllocator(cloud cloud.Cloud) GidAllocator {
	return GidAllocator{
		cloud:      cloud,
		fsIdGidMap: make(map[string]*FilesystemID),
	}
}

// Retrieves the next available GID
func (g *GidAllocator) getNextGid(ctx context.Context, fsId string, gidMin, gidMax int) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	klog.V(5).Infof("Recieved getNextGid for fsId: %v, min: %v, max: %v", fsId, gidMin, gidMax)

	usedGids, err := g.getUsedGids(ctx, fsId)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "Failed to discover used GIDs for filesystem: %v: %v ", fsId, err)
	}

	gid, err := getNextUnusedGid(usedGids, gidMin, gidMax)

	if err != nil {
		return 0, status.Errorf(codes.Internal, "Failed to locate a free GID for given file system: %v. "+
			"Please create a new storage class with a new file-system", fsId)
	}

	return int64(gid), nil

}

func (g *GidAllocator) removeFsId(fsId string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fsIdGidMap, fsId)
}

func (g *GidAllocator) getUsedGids(ctx context.Context, fsId string) (gids []int64, err error) {
	gids = []int64{}
	accessPoints, err := g.cloud.ListAccessPoints(ctx, fsId)
	if err != nil {
		err = fmt.Errorf("failed to list access points: %v", err)
		return
	}
	if len(accessPoints) == 0 {
		return gids, nil
	}
	for _, ap := range accessPoints {
		// This should happen only in tests - skip nil pointers.
		if ap == nil {
			continue
		}
		if ap != nil && ap.PosixUser == nil {
			err = fmt.Errorf("failed to discover used GID because PosixUser is nil for AccessPoint: %s", ap.AccessPointId)
			return
		}
		gids = append(gids, ap.PosixUser.Gid)
	}
	klog.V(5).Infof("Discovered used GIDs: %+v for FS ID: %v", gids, fsId)
	return
}

func getNextUnusedGid(usedGids []int64, gidMin, gidMax int) (nextGid int, err error) {
	requestedRange := gidMax - gidMin

	if requestedRange > ACCESS_POINT_PER_FS_LIMIT {
		klog.Warningf("Requested GID range (%v:%v) exceeds EFS Access Point limit (%v) per Filesystem. Driver will not allocate GIDs outside of this limit.", gidMin, gidMax, ACCESS_POINT_PER_FS_LIMIT)
		gidMin = gidMax - ACCESS_POINT_PER_FS_LIMIT
	}

	var lookup func(usedGids []int64)
	lookup = func(usedGids []int64) {
		for gid := gidMax; gid > gidMin; gid-- {
			if !slices.Contains(usedGids, int64(gid)) {
				nextGid = gid
				return
			}
			klog.V(5).Infof("Allocator found GID which is already in use: %v - trying next one.", nextGid)
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
