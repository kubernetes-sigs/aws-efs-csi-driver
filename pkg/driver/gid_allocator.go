package driver

import (
	"container/heap"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

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

type GidAllocator struct {
	fsIdGidMap map[string]*IntHeap
	mu         sync.Mutex
}

func NewGidAllocator() GidAllocator {
	return GidAllocator{
		fsIdGidMap: make(map[string]*IntHeap),
	}
}

//Retrieves the next available GID
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

//Creates an entry fsIdGidMap if fsId does not exist.
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

//Initializes a heap inclusive of min & max
func initHeap(min, max int) *IntHeap {
	h := make(IntHeap, max-min+1)
	val := min
	for i := range h {
		h[i] = val
		val += 1
	}
	return &h
}
