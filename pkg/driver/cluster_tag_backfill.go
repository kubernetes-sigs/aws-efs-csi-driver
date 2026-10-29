/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	// clusterTagBackfillInterval is how often the backfill sweep runs after the
	// one at startup. New access points are tagged at create time, so later
	// sweeps only pick up resources an administrator tagged in the meantime
	// and retry failed calls. EFS throttles TagResource per account, so the
	// sweep runs rarely.
	clusterTagBackfillInterval = time.Hour

	// provisionedByAnnotation is set by the external-provisioner on every PV it
	// provisions.
	provisionedByAnnotation = "pv.kubernetes.io/provisioned-by"

	listPageSize = 500

	// maxListedResources caps the resource lists in the sweep summary line.
	maxListedResources = 20
)

// backfillResult counts one sweep. The counters mean:
//   - tagged: tag calls that succeeded;
//   - failed: tag calls that failed (not also counted in remaining);
//   - needsAdministrator: distinct resources reported for an administrator;
//   - remaining: resources, or whole listings, that could not be checked because a list or
//     describe call failed. A failed Kubernetes listing counts 1.
type backfillResult struct {
	tagged             int
	failed             int
	needsAdministrator int
	remaining          int

	failedResources             []string
	needsAdministratorResources []string
	firstError                  error
	reported                    map[string]struct{}
}

func (r *backfillResult) complete() bool {
	return r.failed == 0 && r.needsAdministrator == 0 && r.remaining == 0
}

func (r *backfillResult) setError(err error) {
	if r.firstError == nil {
		r.firstError = err
	}
}

func (r *backfillResult) tagFailed(resource string, err error) {
	r.failed++
	r.failedResources = append(r.failedResources, resource)
	r.setError(err)
}

func (r *backfillResult) notChecked(err error) {
	r.remaining++
	r.setError(err)
}

func (r *backfillResult) needsAdmin(resource, tagKey string) {
	if _, ok := r.reported[resource]; ok {
		return
	}
	if r.reported == nil {
		r.reported = map[string]struct{}{}
	}
	r.reported[resource] = struct{}{}
	r.needsAdministrator++
	r.needsAdministratorResources = append(r.needsAdministratorResources, resource)
	klog.V(4).InfoS("resource needs cluster tag from an administrator", "resource", resource, "tagKey", tagKey)
}

// summary returns the key/value pairs of the "cluster tag backfill sweep finished" line.
func (r *backfillResult) summary() []interface{} {
	kv := []interface{}{"tagged", r.tagged, "failed", r.failed, "needsAdministrator", r.needsAdministrator, "remaining", r.remaining}
	kv = appendResourceList(kv, "needsAdministrator", r.needsAdministratorResources)
	kv = appendResourceList(kv, "failed", r.failedResources)
	if (r.failed > 0 || r.remaining > 0) && r.firstError != nil {
		kv = append(kv, "firstError", r.firstError.Error())
	}
	return kv
}

// appendResourceList adds <name>Resources with at most maxListedResources IDs, and <name>Total
// when the list was cut.
func appendResourceList(kv []interface{}, name string, resources []string) []interface{} {
	if len(resources) == 0 {
		return kv
	}
	if len(resources) > maxListedResources {
		return append(kv, name+"Resources", resources[:maxListedResources], name+"Total", len(resources))
	}
	return append(kv, name+"Resources", resources)
}

// provisionerSecretNameKey is the StorageClass parameter that passes a secret to
// CreateVolume and DeleteVolume. In this driver that secret only carries
// cross-account settings (awsRoleArn, externalId, crossaccount).
const provisionerSecretNameKey = "csi.storage.k8s.io/provisioner-secret-name"

type sweepAccessPoint struct {
	id      string
	fsType  util.FileSystemType
	dynamic bool
	// mayBeShared is set when a PV's StorageClass has reuseAccessPoint=true or no
	// longer exists. Other clusters can reuse such an access point, so backfill
	// cannot tell whether this cluster owns it.
	mayBeShared bool
}

type sweepFileSystem struct {
	id     string
	fsType util.FileSystemType
}

// clusterTagBackfiller adds the cluster tag to existing access points that this
// cluster owns, and reports the resources it cannot tag.
type clusterTagBackfiller struct {
	cloud         cloud.Cloud
	clusterTag    clusterTagConfig
	newKubeClient cloud.KubernetesAPIClient
	completeOnce  bool
}

// run sweeps once at startup and then every interval until ctx is done.
func (b *clusterTagBackfiller) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		b.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sweep visits every access point and file system referenced by this cluster's
// EFS PersistentVolumes and StorageClasses.
//
// An access point is owned by this cluster when it backs a PV that this driver
// provisioned, it carries the driver's efs.csi.aws.com/cluster=true tag, and its
// StorageClass does not set reuseAccessPoint. Owned access points without the
// cluster tag get it in a TagResource request that contains only the cluster
// tag. Every other referenced resource without this cluster's tag value is
// reported for an administrator to tag: file systems, statically provisioned
// access points, access points without the driver's tag, access points that
// other clusters may reuse, resources tagged for another cluster, and resources
// the controller's own credentials cannot find. Cross-account volumes
// (StorageClasses with a provisioner secret) are left out: they live in another
// account, and that account's IAM governs them.
func (b *clusterTagBackfiller) sweep(ctx context.Context) backfillResult {
	var res backfillResult
	key := b.clusterTag.tagKey

	aps, fss, err := b.listReferencedResources(ctx)
	if err != nil {
		klog.V(4).InfoS("cluster tag backfill sweep could not list Kubernetes objects", "err", err)
		res.notChecked(err)
		b.logSummary(&res)
		return res
	}

	for _, ap := range aps {
		tags, err := b.cloud.GetAccessPointTags(ctx, ap.id, ap.fsType)
		if err != nil {
			if errors.Is(err, cloud.ErrNotFound) {
				res.needsAdmin(ap.id, key)
				continue
			}
			klog.V(4).InfoS("cluster tag backfill could not read tags of existing resource", "resource", ap.id, "err", err)
			res.notChecked(err)
			continue
		}
		if b.clusterTag.matches(tags) {
			continue
		}
		_, hasKey := tags[key]
		if hasKey || !ap.dynamic || ap.mayBeShared || tags[DefaultTagKey] != DefaultTagValue {
			res.needsAdmin(ap.id, key)
			continue
		}
		if err := b.cloud.TagAccessPoint(ctx, ap.id, b.clusterTag.tag(), ap.fsType); err != nil {
			if errors.Is(err, cloud.ErrNotFound) {
				res.needsAdmin(ap.id, key)
				continue
			}
			klog.V(4).InfoS("failed to add cluster tag to existing resource", "resource", ap.id, "tagKey", key, "err", err)
			res.tagFailed(ap.id, err)
			continue
		}
		klog.V(4).InfoS("added cluster tag to existing resource", "resource", ap.id, "tagKey", key)
		res.tagged++
	}

	for _, fs := range fss {
		tags, err := b.cloud.GetFileSystemTags(ctx, fs.id, fs.fsType)
		if err != nil {
			if errors.Is(err, cloud.ErrNotFound) {
				res.needsAdmin(fs.id, key)
				continue
			}
			klog.V(4).InfoS("cluster tag backfill could not read tags of existing resource", "resource", fs.id, "err", err)
			res.notChecked(err)
			continue
		}
		if b.clusterTag.matches(tags) {
			continue
		}
		res.needsAdmin(fs.id, key)
	}

	b.logSummary(&res)
	return res
}

// logSummary logs the one summary line of a sweep, and "complete" after the first clean sweep
// of this process.
func (b *clusterTagBackfiller) logSummary(res *backfillResult) {
	klog.InfoS("cluster tag backfill sweep finished", res.summary()...)
	if res.complete() && !b.completeOnce {
		b.completeOnce = true
		klog.InfoS("cluster tag backfill complete", "clusterName", b.clusterTag.clusterName, "tagKey", b.clusterTag.tagKey)
	}
}

// listReferencedResources returns the access points and file systems referenced
// by this driver's PersistentVolumes, plus the file systems named in this
// driver's StorageClasses, sorted by ID.
func (b *clusterTagBackfiller) listReferencedResources(ctx context.Context) ([]sweepAccessPoint, []sweepFileSystem, error) {
	client, err := b.newKubeClient()
	if err != nil {
		return nil, nil, err
	}
	aps := map[string]sweepAccessPoint{}
	fss := map[string]sweepFileSystem{}

	scs, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	// reuse maps each of this driver's StorageClasses to its reuseAccessPoint
	// value, and crossAccount marks those whose volumes live in another account.
	reuse := map[string]bool{}
	crossAccount := map[string]bool{}
	for _, sc := range scs.Items {
		if sc.Provisioner != driverName {
			continue
		}
		if sc.Parameters[provisionerSecretNameKey] != "" {
			crossAccount[sc.Name] = true
			continue
		}
		fsType := util.FileSystemTypeEFS
		if sc.Parameters[ProvisioningMode] == S3FilesAccessPointMode {
			fsType = util.FileSystemTypeS3Files
		}
		addFileSystem(fss, sc.Parameters[FsId], fsType)
		v, err := strconv.ParseBool(sc.Parameters[ReuseAccessPointKey])
		reuse[sc.Name] = err == nil && v
	}

	if err := listPersistentVolumes(ctx, client, func(volumeHandle, storageClass string, dynamic bool) {
		if crossAccount[storageClass] {
			return
		}
		fsId, _, apId, fsType, err := parseVolumeId(volumeHandle)
		if err != nil {
			return
		}
		addFileSystem(fss, fsId, fsType)
		if apId == "" {
			return
		}
		ap := aps[apId]
		ap.id, ap.fsType = apId, fsType
		ap.dynamic = ap.dynamic || dynamic
		if dynamic {
			shared, known := reuse[storageClass]
			ap.mayBeShared = ap.mayBeShared || shared || !known
		}
		aps[apId] = ap
	}); err != nil {
		return nil, nil, err
	}

	apList := make([]sweepAccessPoint, 0, len(aps))
	for _, ap := range aps {
		apList = append(apList, ap)
	}
	sort.Slice(apList, func(i, j int) bool { return apList[i].id < apList[j].id })
	fsList := make([]sweepFileSystem, 0, len(fss))
	for _, fs := range fss {
		fsList = append(fsList, fs)
	}
	sort.Slice(fsList, func(i, j int) bool { return fsList[i].id < fsList[j].id })
	return apList, fsList, nil
}

func listPersistentVolumes(ctx context.Context, client kubernetes.Interface, visit func(volumeHandle, storageClass string, dynamic bool)) error {
	opts := metav1.ListOptions{Limit: listPageSize}
	for {
		pvs, err := client.CoreV1().PersistentVolumes().List(ctx, opts)
		if err != nil {
			return err
		}
		for _, pv := range pvs.Items {
			if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != driverName {
				continue
			}
			visit(pv.Spec.CSI.VolumeHandle, pv.Spec.StorageClassName, pv.Annotations[provisionedByAnnotation] == driverName)
		}
		if pvs.Continue == "" {
			return nil
		}
		opts.Continue = pvs.Continue
	}
}

// addFileSystem records a file system by ID. A mount target DNS name
// (fs-12345678.efs.<region>.amazonaws.com) is reduced to the file system ID.
func addFileSystem(fss map[string]sweepFileSystem, id string, fsType util.FileSystemType) {
	id = strings.TrimSpace(id)
	if i := strings.Index(id, "."); i > 0 {
		id = id[:i]
	}
	if !strings.HasPrefix(id, "fs-") {
		return
	}
	if _, ok := fss[id]; !ok {
		fss[id] = sweepFileSystem{id: id, fsType: fsType}
	}
}
