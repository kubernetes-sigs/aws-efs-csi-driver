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
	"fmt"
	"sort"
	"strings"

	"k8s.io/klog/v2"
)

// ClusterTagIsolation selects how the controller uses the cluster tag
// (<--cluster-tag-key>=<--cluster-name>). It must match the IAM policy attached
// to the controller's role; the controller cannot verify that.
type ClusterTagIsolation string

const (
	// ClusterTagIsolationDisable keeps the default behavior. New access points still
	// get the cluster tag when --cluster-name and --cluster-tag-key are set.
	ClusterTagIsolationDisable ClusterTagIsolation = "disable"
	// ClusterTagIsolationBackfill adds the cluster tag to existing access points
	// this cluster owns, at startup and periodically. It does not filter discovery.
	ClusterTagIsolationBackfill ClusterTagIsolation = "backfill"
	// ClusterTagIsolationEnforce ignores discovered access points that do not
	// carry this cluster's tag value. It does not backfill.
	ClusterTagIsolationEnforce ClusterTagIsolation = "enforce"
)

// clusterTagConfig is the validated cluster identity of this controller.
type clusterTagConfig struct {
	clusterName string
	tagKey      string
	isolation   ClusterTagIsolation
}

// enabled reports whether new access points get the cluster tag.
func (c clusterTagConfig) enabled() bool {
	return c.clusterName != "" && c.tagKey != ""
}

// tag returns the cluster tag, or nil when it is not configured.
func (c clusterTagConfig) tag() map[string]string {
	if !c.enabled() {
		return nil
	}
	return map[string]string{c.tagKey: c.clusterName}
}

// matches reports whether tags carry this cluster's tag value.
func (c clusterTagConfig) matches(tags map[string]string) bool {
	v, ok := tags[c.tagKey]
	return ok && v == c.clusterName
}

func (c clusterTagConfig) enforce() bool {
	return c.enabled() && c.isolation == ClusterTagIsolationEnforce
}

func (c clusterTagConfig) backfill() bool {
	return c.enabled() && c.isolation == ClusterTagIsolationBackfill
}

// parseClusterTagIsolation accepts exactly "", disable, backfill and enforce. An empty value
// means disable. Values are not trimmed or lowercased.
func parseClusterTagIsolation(s string) (ClusterTagIsolation, error) {
	switch ClusterTagIsolation(s) {
	case "":
		return ClusterTagIsolationDisable, nil
	case ClusterTagIsolationDisable, ClusterTagIsolationBackfill, ClusterTagIsolationEnforce:
		return ClusterTagIsolation(s), nil
	}
	return "", fmt.Errorf(`invalid --cluster-tag-isolation %q: must be "disable", "backfill" or "enforce"`, s)
}

// logSettings logs the cluster tag isolation settings once at startup.
func (c clusterTagConfig) logSettings() {
	kv := []interface{}{"clusterName", c.clusterName, "tagKey", c.tagKey, "clusterTagIsolation", c.isolation}
	if c.backfill() {
		kv = append(kv, "backfillInterval", clusterTagBackfillInterval.String())
	}
	klog.InfoS("cluster tag isolation settings", kv...)
}

func newClusterTagConfig(clusterName, tagKey, isolation string) (clusterTagConfig, error) {
	clusterName = strings.TrimSpace(clusterName)
	tagKey = strings.TrimSpace(tagKey)
	iso, err := parseClusterTagIsolation(isolation)
	if err != nil {
		return clusterTagConfig{}, err
	}
	if iso != ClusterTagIsolationDisable && (clusterName == "" || tagKey == "") {
		return clusterTagConfig{}, fmt.Errorf("--cluster-tag-isolation=%s requires both --cluster-name and --cluster-tag-key", iso)
	}
	if (clusterName == "") != (tagKey == "") {
		klog.InfoS("only one of --cluster-name and --cluster-tag-key is set; no cluster tag is written", "clusterName", clusterName, "tagKey", tagKey)
	}
	if tagKey != "" {
		if strings.HasPrefix(strings.ToLower(tagKey), "aws:") {
			return clusterTagConfig{}, fmt.Errorf("--cluster-tag-key %q: the aws: prefix is reserved by AWS", tagKey)
		}
		if tagKey == DefaultTagKey {
			return clusterTagConfig{}, fmt.Errorf("--cluster-tag-key must not be %s, which the driver always sets to %q", DefaultTagKey, DefaultTagValue)
		}
	}
	return clusterTagConfig{clusterName: clusterName, tagKey: tagKey, isolation: iso}, nil
}

// parseExtraTags parses the --extra-tags format k1=v1,k2=v2.
func parseExtraTags(s string) (map[string]string, error) {
	tags := map[string]string{}
	s = strings.TrimSpace(s)
	if s == "" {
		return tags, nil
	}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --extra-tags entry %q: expected key=value", pair)
		}
		if _, dup := tags[k]; dup {
			return nil, fmt.Errorf("invalid --extra-tags: key %q is set more than once", k)
		}
		tags[k] = strings.TrimSpace(v)
	}
	return tags, nil
}

// mergeTags combines the deprecated --tags with --extra-tags and checks that
// neither sets a key the driver owns.
func mergeTags(legacy, extra map[string]string, clusterTag clusterTagConfig) (map[string]string, error) {
	merged := map[string]string{}
	for k, v := range legacy {
		merged[k] = v
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := extra[k]
		if strings.HasPrefix(strings.ToLower(k), "aws:") {
			return nil, fmt.Errorf("--extra-tags key %q: the aws: prefix is reserved by AWS", k)
		}
		if k == DefaultTagKey {
			return nil, fmt.Errorf("--extra-tags must not set %s, which the driver always sets", DefaultTagKey)
		}
		if old, ok := merged[k]; ok && old != v {
			return nil, fmt.Errorf("tag key %q has different values in --tags (%q) and --extra-tags (%q)", k, old, v)
		}
		merged[k] = v
	}
	if clusterTag.tagKey != "" {
		if _, ok := merged[clusterTag.tagKey]; ok {
			return nil, fmt.Errorf("--tags and --extra-tags must not set the cluster tag key %q; set --cluster-name instead", clusterTag.tagKey)
		}
	}
	return merged, nil
}
