package driver

import (
	"container/heap"
	"strconv"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type FileSystemIdentityManager struct {
	gidAllocator GidAllocator
}

func NewFileSystemIdentityManager() FileSystemIdentityManager {
	return FileSystemIdentityManager{NewGidAllocator()}
}

func (f *FileSystemIdentityManager) GetUidAndGid(rawUid string, rawGid string, rawGidMin string, rawGidMax string,
	fsId string) (int, int, error) {

	var (
		uid int
		gid int
		err error
	)

	if rawGid != "" {
		gid, err = f.extractId(rawGid)
		if err != nil {
			return -1, -1, err
		}
	} else {
		gidMin, gidMax, err := f.parseGidMinAndMax(rawGidMin, rawGidMax)
		if err != nil {
			return -1, -1, err
		}
		allocatedGid, err := f.gidAllocator.getNextGid(fsId, int(gidMin), int(gidMax))
		if err != nil {
			return -1, -1, err
		}
		gid = allocatedGid
	}

	if rawUid == "" {
		return gid, gid, nil
	} else {
		uid, err = f.extractId(rawUid)
		if err != nil {
			f.ReleaseGid(fsId, gid)
			return -1, -1, err
		}
	}

	return uid, gid, nil
}

func (f *FileSystemIdentityManager) ReleaseGid(fsId string, gid int) {
	if gid != 0 {
		f.gidAllocator.releaseGid(fsId, gid)
	}
}

func (f *FileSystemIdentityManager) parseGidMinAndMax(rawGidMin string, rawGidMax string) (int, int, error) {
	if rawGidMin == "0" {
		return -1, -1, status.Errorf(codes.InvalidArgument, "GidMin should be a > 0")
	}

	if rawGidMin == "" && rawGidMax == "" {
		return DefaultGidMin, DefaultGidMax, nil
	}

	gidMin, err := f.extractId(rawGidMin)
	if err != nil {
		return -1, -1, err
	}
	gidMax, err := f.extractId(rawGidMax)
	if err != nil {
		return -1, -1, err
	}

	if gidMin >= gidMax {
		return -1, -1, status.Errorf(codes.InvalidArgument, "GidMin cannot be greater than GidMax")
	} else if gidMin > 0 && gidMax == 0 {
		return -1, -1, status.Errorf(codes.InvalidArgument, "Both GidMin and GidMax must be provided")
	}
	return gidMin, gidMax, nil
}

func (f *FileSystemIdentityManager) extractId(rawId string) (int, error) {
	id, err := strconv.ParseInt(rawId, 10, 32)
	if err != nil {
		return -1, err
	}
	if id < 0 {
		return -1, status.Errorf(codes.InvalidArgument, "UID should be a positive integer but was %d", id)
	}
	return int(id), nil
}

type GidAllocator struct {
	fsIdGidMap map[string]*IntHeap
	mu         sync.Mutex
}

func NewGidAllocator() GidAllocator {
	return GidAllocator{
		fsIdGidMap: make(map[string]*IntHeap),
	}
}

// Retrieves the next available GID
func (g *GidAllocator) getNextGid(fsId string, gidMin, gidMax int) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	klog.V(5).Infof("Recieved getNextGid for fsId: %v, min: %v, max: %v", fsId, gidMin, gidMax)

	if _, ok := g.fsIdGidMap[fsId]; !ok {
		klog.V(5).Infof("FS Id doesn't exist, initializing...")
		g.initFsId(fsId, gidMin, gidMax)
	}

	gidHeap := g.fsIdGidMap[fsId]

	if gidHeap.Len() > 0 {
		return heap.Pop(gidHeap).(int), nil
	} else {
		return 0, status.Errorf(codes.Internal, "Failed to locate a free GID for given the file system: %v. "+
			"Please create a new storage class with a new file-system", fsId)
	}
}

func (g *GidAllocator) releaseGid(fsId string, gid int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	gidHeap := g.fsIdGidMap[fsId]
	gidHeap.Push(gid)
}

// Creates an entry fsIdGidMap if fsId does not exist.
func (g *GidAllocator) initFsId(fsId string, gidMin, gidMax int) {
	h := initHeap(gidMin, gidMax)
	heap.Init(h)
	g.fsIdGidMap[fsId] = h
}

func (g *GidAllocator) removeFsId(fsId string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fsIdGidMap, fsId)
}

type IntHeap []int

func (h IntHeap) Len() int {
	return len(h)
}
func (h IntHeap) Less(i, j int) bool {
	return h[i] < h[j]
}
func (h IntHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *IntHeap) Push(x interface{}) {
	*h = append(*h, x.(int))
}

func (h *IntHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Initializes a heap inclusive of min & max
func initHeap(min, max int) *IntHeap {
	h := make(IntHeap, max-min+1)
	val := min
	for i := range h {
		h[i] = val
		val += 1
	}
	return &h
}
