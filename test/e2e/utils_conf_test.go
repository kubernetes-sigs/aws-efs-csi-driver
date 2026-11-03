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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ginkgo "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/kubectl"
	admissionapi "k8s.io/pod-security-admission/api"
)

// utilsConfDriverKeys are the keys the driver sets in each conf file, mirroring
// efsUtilsDriverConf and s3filesUtilsDriverConf in pkg/driver/efs_watch_dog.go.
// They are the only installed keys the driver may change.
var utilsConfDriverKeys = map[string][][2]string{
	"efs-utils.conf": {
		{"DEFAULT", "logging_level"},
		{"mount", "region"},
		{"mount", "stunnel_debug_enabled"},
		{"mount", "fips_mode_enabled"},
		{"mount", "port_range_upper_bound"},
		{"cloudwatch-log", "enabled"},
	},
	"s3files-utils.conf": {
		{"DEFAULT", "logging_level"},
		{"mount", "region"},
		{"mount", "fips_mode_enabled"},
		{"mount", "port_range_upper_bound"},
		{"proxy", "proxy_logging_level"},
		{"proxy", "metrics_enabled"},
		{"cloudwatch-log", "enabled"},
	},
}

// utilsConfOverrideFlags maps each conf file to the driver flag that overrides it.
var utilsConfOverrideFlags = map[string]string{
	"efs-utils.conf":     "--efs-utils-conf-overrides=",
	"s3files-utils.conf": "--s3files-utils-conf-overrides=",
}

// utilsConfDriverAddedKeyLog is part of the warning the driver logs when the
// installed conf lacks one of its keys, which usually means efs-utils renamed
// it. TestWriteConfigFileWarnsOnMissingDriverKey in pkg/driver pins the warning
// to this text (driverAddedKeyLogMarker); keep the two in step.
const utilsConfDriverAddedKeyLog = "so the driver added it"

// utilsConfCheckScript runs in the efs-plugin container. It parses the conf
// efs-utils installed and the one the driver wrote with configparser, which is
// what efs-utils reads them with, and prints "OK" or one failure per line:
//   - either file fails to parse (e.g. a duplicate key)
//   - an installed section is missing, or an installed key changed, other than
//     a driver key or an operator override
//   - a key appeared that is not installed, a driver key, or an override
//   - a driver key (other than the optional region) or [client-info] source is unset
//
// argv[1] is the driver keys and argv[2] the overridden keys, per file, as JSON.
const utilsConfCheckScript = `
import configparser, json, sys
driver_keys, override_keys = json.loads(sys.argv[1]), json.loads(sys.argv[2])
fails = []

def load(path):
    p = configparser.RawConfigParser()
    with open(path) as f:
        p.read_file(f)
    d = {"DEFAULT": dict(p.defaults())}
    for s in p.sections():
        d[s] = dict(p._sections[s])
    return d

for name, keys in sorted(driver_keys.items()):
    try:
        installed = load("/etc/amazon/efs-static-files/" + name)
        written = load("/etc/amazon/efs/" + name)
    except Exception as e:
        fails.append("%s: %s: %s" % (name, type(e).__name__, e))
        continue
    owned = {tuple(k) for k in keys} | {tuple(k) for k in override_keys.get(name, [])}
    for s, kv in installed.items():
        if s not in written:
            fails.append("%s: installed section [%s] is missing" % (name, s))
            continue
        for k, v in kv.items():
            if (s, k) not in owned and written[s].get(k) != v:
                fails.append("%s: [%s] %s = %r, installed %r" % (name, s, k, written[s].get(k), v))
    known = {(s, k) for s, kv in installed.items() for k in kv} | owned | {("client-info", "source")}
    for s, k in sorted((s, k) for s, kv in written.items() for k in kv):
        if (s, k) not in known:
            fails.append("%s: unexpected key [%s] %s" % (name, s, k))
    for s, k in keys:
        if (s, k) != ("mount", "region") and k not in written.get(s, {}):
            fails.append("%s: driver key [%s] %s is not set" % (name, s, k))
    if not written.get("client-info", {}).get("source"):
        fails.append("%s: [client-info] source is not set" % name)

print("\n".join(fails) if fails else "OK")
`

var _ = ginkgo.Describe("[efs-csi] efs-utils conf", func() {
	f := framework.NewDefaultFramework("efs-utils-conf")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelBaseline

	ginkgo.It("should write efs-utils conf files that efs-utils can parse and that keep the installed settings", func() {
		driverPods, err := f.ClientSet.CoreV1().Pods(EfsDriverNamespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: buildLabelSelector(EfsDriverLabelSelectors),
		})
		framework.ExpectNoError(err, "listing efs-csi-node pods")
		if len(driverPods.Items) == 0 {
			ginkgo.Fail("No efs-csi-node pods found")
		}

		driverKeys, err := json.Marshal(utilsConfDriverKeys)
		framework.ExpectNoError(err, "encoding driver keys")

		for _, pod := range driverPods.Items {
			ginkgo.By(fmt.Sprintf("Checking the conf files on %s (node %s)", pod.Name, pod.Spec.NodeName))

			// Keys the operator overrides may differ from the installed value.
			overrideKeys := map[string][][2]string{}
			for _, c := range pod.Spec.Containers {
				if c.Name != "efs-plugin" {
					continue
				}
				for _, arg := range c.Args {
					for fileName, flagPrefix := range utilsConfOverrideFlags {
						if raw, ok := strings.CutPrefix(arg, flagPrefix); ok {
							overrideKeys[fileName] = append(overrideKeys[fileName], parseConfOverrideKeys(raw)...)
						}
					}
				}
			}
			overrides, err := json.Marshal(overrideKeys)
			framework.ExpectNoError(err, "encoding override keys")

			output := strings.TrimSpace(kubectl.RunKubectlOrDie(EfsDriverNamespace, "exec", pod.Name, "-c", "efs-plugin", "--",
				"python3", "-c", utilsConfCheckScript, string(driverKeys), string(overrides)))
			if output != "OK" {
				ginkgo.Fail(fmt.Sprintf("conf files on %s are not what the driver should write:\n%s", pod.Name, output))
			}

			ginkgo.By(fmt.Sprintf("Checking %s did not have to add a key missing from the installed conf", pod.Name))
			logs := kubectl.RunKubectlOrDie(EfsDriverNamespace, "logs", pod.Name, "-c", "efs-plugin")
			for _, line := range strings.Split(logs, "\n") {
				if strings.Contains(line, utilsConfDriverAddedKeyLog) {
					ginkgo.Fail(fmt.Sprintf("the installed efs-utils lacks a key the driver sets, so efs-utils may have renamed it "+
						"and the driver's setting no longer takes effect. Update efsUtilsDriverConf/s3filesUtilsDriverConf:\n%s", line))
				}
			}
		}
	})
})

// parseConfOverrideKeys returns the section and key of each "section:key=value"
// entry in a --*-conf-overrides flag value.
func parseConfOverrideKeys(raw string) [][2]string {
	var keys [][2]string
	for _, entry := range strings.Split(raw, ",") {
		section, kv, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok {
			continue
		}
		key, _, _ := strings.Cut(kv, "=")
		keys = append(keys, [2]string{strings.TrimSpace(section), strings.TrimSpace(key)})
	}
	return keys
}
