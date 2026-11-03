/*
Copyright 2019 The Kubernetes Authors.
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
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/klog/v2"
)

const (
	efsConfigFileName     = "efs-utils.conf"
	s3filesConfigFileName = "s3files-utils.conf"
)

func TestExecWatchdog(t *testing.T) {
	configDirName := createTempDir(t)
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	defer os.RemoveAll(staticFileDirName)
	installConfFixtures(t, staticFileDirName)

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300")
	if err := w.start(); err != nil {
		t.Fatalf("Failed to start %v", err)
	}
	time.Sleep(time.Second)
	w.stop()
}

func createTempDir(t *testing.T) string {
	name, err := os.MkdirTemp("", "")
	checkError(t, err)
	return name
}

func checkError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
}

func createFileInDir(t *testing.T, dirName, fileName string) *os.File {
	f, err := os.Create(filepath.Join(dirName, fileName))
	checkError(t, err)
	return f
}

// clearConfEnv pins the environment the config render reads, so the expected
// output does not depend on what is exported on the machine running the tests.
func clearConfEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("FIPS_ENABLED", "")
	t.Setenv("PORT_RANGE_UPPER_BOUND", "")
}

// readConfFixture returns a conf file from testdata. These are snapshots of the
// conf files efs-utils ships (dist/efs-utils.conf and dist/s3files-utils.conf),
// standing in for what the efs-utils package installs in the image. They are
// test input only: the driver reads the installed file at runtime, so a
// snapshot that falls behind upstream cannot affect a node, and refreshing it
// is optional. They are real copies rather than hand-written ones so that
// TestDriverConfKeysExistInConf checks the driver's key names against a conf
// efs-utils actually shipped.
func readConfFixture(t *testing.T, fileName string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", fileName))
	if err != nil {
		t.Fatalf("failed to read the %s fixture: %v", fileName, err)
	}
	return string(data)
}

// installConfFixtures puts both conf fixtures in a static files directory,
// standing in for the efs-utils package install the Dockerfile moves there.
func installConfFixtures(t *testing.T, staticFilesDir string) {
	t.Helper()
	for _, fileName := range []string{efsConfigFileName, s3filesConfigFileName} {
		installConfFixture(t, staticFilesDir, fileName, readConfFixture(t, fileName))
	}
}

func installConfFixture(t *testing.T, staticFilesDir, fileName, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(staticFilesDir, fileName), []byte(content), 0644); err != nil {
		t.Fatalf("failed to install the %s fixture: %v", fileName, err)
	}
}

func TestSetupWithEmptyConfigDirectory(t *testing.T) {
	clearConfEnv(t)
	//create file A, B in static file directory and keep config directory empty
	configDirName := createTempDir(t)
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	defer os.RemoveAll(staticFileDirName)

	fileAName := "A"
	fileAContent := "dummyA"
	createFile(t, staticFileDirName, fileAName, fileAContent)

	fileBName := "B"
	fileBContent := "dummyB"
	createFile(t, staticFileDirName, fileBName, fileBContent)
	installConfFixtures(t, staticFileDirName)

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	efsConfigFilePath := filepath.Join(configDirName, efsConfigFileName)
	s3filesConfigFilePath := filepath.Join(configDirName, s3filesConfigFileName)
	if err := w.setup(efsClient); err != nil {
		t.Fatalf("Failed to update config file %v and %v, %v", efsConfigFilePath, s3filesConfigFilePath, err)
	}

	verifyEfsConfigFile(t, efsConfigFilePath)
	verifyS3FilesConfigFile(t, s3filesConfigFilePath)

	//verify file A and B are copied over to static file directory
	verifyFileContent(t, filepath.Join(configDirName, "A"), fileAContent)
	verifyFileContent(t, filepath.Join(configDirName, "B"), fileBContent)
}

func TestSetupWithCloudWatchMetricsDisabled(t *testing.T) {
	configDirName := createTempDir(t)
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	defer os.RemoveAll(staticFileDirName)

	installConfFixtures(t, staticFileDirName)

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, false, nil, nil, "sleep", "300").(*execWatchdog)
	if err := w.setup("k8s"); err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}

	s3filesConfigFilePath := filepath.Join(configDirName, s3filesConfigFileName)
	configFileContent, err := os.ReadFile(s3filesConfigFilePath)
	if err != nil {
		t.Fatalf("Failed to read s3files config: %v", err)
	}
	actualConfig := string(configFileContent)
	if !strings.Contains(actualConfig, "metrics_enabled = false") {
		t.Fatalf("Expected s3files config to contain 'metrics_enabled = false', got:\n%s", actualConfig)
	}
}

func TestSetupWithNonEmptyConfigDirectory(t *testing.T) {
	clearConfEnv(t)
	//create file A, B in static file directory
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(staticFileDirName)

	fileAName := "A"
	fileAContent := "dummyA"
	createFile(t, staticFileDirName, fileAName, fileAContent)

	fileBName := "B"
	fileBContent := "dummyB"
	createFile(t, staticFileDirName, fileBName, fileBContent)

	// Create a different B file in the config directory with the expectation that B won't be overwritten.
	configDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	differentContent := "differentDummy"
	createFile(t, configDirName, fileBName, differentContent)
	installConfFixtures(t, staticFileDirName)

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	efsConfigFilePath := filepath.Join(configDirName, efsConfigFileName)
	s3filesConfigFilePath := filepath.Join(configDirName, s3filesConfigFileName)
	if err := w.setup(efsClient); err != nil {
		t.Fatalf("Failed to update config file %v and %v, %v", efsConfigFilePath, s3filesConfigFilePath, err)
	}

	verifyEfsConfigFile(t, efsConfigFilePath)
	verifyS3FilesConfigFile(t, s3filesConfigFilePath)

	//verify file A is copied over from static files directory but file B in the config direcotry remains as is
	verifyFileContent(t, filepath.Join(configDirName, "A"), fileAContent)
	verifyFileContent(t, filepath.Join(configDirName, "B"), differentContent)
}

func TestSetupWithNonexistentConfigDirectory(t *testing.T) {
	configDirName := ""
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(staticFileDirName)
	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	if err := w.setup(efsClient); err == nil {
		t.Fatalf("Expected failure since static files directory doesn't exist.")
	}
}

func TestSetupWithNonexistentStaticFilesDirectory(t *testing.T) {
	configDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	staticFileDirName := ""
	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	if err := w.setup(efsClient); err == nil {
		t.Fatalf("Expected failure since config directory doesn't exist.")
	}
}

func TestSetupWithAdditionalDirectoryInStaticFilesDirectory(t *testing.T) {
	configDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)

	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(staticFileDirName)

	_, err := os.MkdirTemp(staticFileDirName, "")
	checkError(t, err)

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	if err := w.setup(efsClient); err == nil {
		t.Fatalf("Expected failure since config directory contains another directory.")
	}
}

func TestRemoveLibwrapOption(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-stunnel-configs")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testConfig := []byte(`
fips = no
[efs]
client = yes
accept = 127.0.0.1:1000
connect = fs-123.efs.us-west-2.amazonaws.com:2049
libwrap = no
`)
	testFilePath := filepath.Join(tempDir, "stunnel-config.test")
	if err := os.WriteFile(testFilePath, testConfig, 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	w := &execWatchdog{efsUtilsCfgPath: tempDir}

	if err := w.removeLibwrapOption(tempDir); err != nil {
		t.Fatalf("removeLibwrapOption failed: %v", err)
	}

	content, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	if strings.Contains(string(content), "libwrap = no") {
		t.Errorf("libwrap = no was not removed from the config file")
	}
}

func TestRemoveLibwrapOptionWithNonexistentStateDirectory(t *testing.T) {
	w := &execWatchdog{}

	// The state directory is only mounted into the node DaemonSet; the
	// controller Deployment has no stunnel state files to fix, so a
	// missing directory must be treated as a no-op rather than an error.
	stateDir := filepath.Join(t.TempDir(), "does-not-exist")
	if err := w.removeLibwrapOption(stateDir); err != nil {
		t.Fatalf("Expected no error for nonexistent state directory, got: %v", err)
	}
}

func TestRemoveLibwrapOptionWithUnreadableStateDirectory(t *testing.T) {
	w := &execWatchdog{}

	// A state directory that exists but cannot be read is still an error,
	// and the error must name the directory that was actually passed in.
	stateDir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(stateDir, []byte("x"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	err := w.removeLibwrapOption(stateDir)
	if err == nil {
		t.Fatalf("Expected error for state directory that is a regular file")
	}
	if !strings.Contains(err.Error(), stateDir) {
		t.Errorf("Expected error to mention %q, got: %v", stateDir, err)
	}
}

func verifyFileContent(t *testing.T, fileName string, expectedFileContent string) {
	fileContent, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("Failed to read file %v, %v", fileName, err)
	}
	actualFileContent := string(fileContent)
	if actualFileContent != expectedFileContent {
		t.Fatalf("Unexpected A file config content: want %s\nactual:%s", expectedFileContent, actualFileContent)
	}
}

// verifyEfsConfigFile checks the efs-utils.conf the driver wrote with default
// flags (debugLogs off, efs CloudWatch logs off) and no region in the
// environment, against the installed fixture.
func verifyEfsConfigFile(t *testing.T, efsConfigFilePath string) {
	verifyAgainstInstalled(t, efsConfigFilePath, efsConfigFileName, map[confKey]string{
		{"DEFAULT", "logging_level"}:        "INFO",
		{"mount", "stunnel_debug_enabled"}:  "false",
		{"mount", "fips_mode_enabled"}:      "false",
		{"mount", "port_range_upper_bound"}: "21049",
		{"cloudwatch-log", "enabled"}:       "false",
	})
}

// verifyS3FilesConfigFile is verifyEfsConfigFile for s3files-utils.conf, with
// s3files CloudWatch logs and metrics on.
func verifyS3FilesConfigFile(t *testing.T, s3filesConfigFilePath string) {
	verifyAgainstInstalled(t, s3filesConfigFilePath, s3filesConfigFileName, map[confKey]string{
		{"DEFAULT", "logging_level"}:        "INFO",
		{"mount", "fips_mode_enabled"}:      "false",
		{"mount", "port_range_upper_bound"}: "21049",
		{"proxy", "proxy_logging_level"}:    "INFO",
		{"proxy", "metrics_enabled"}:        "true",
		{"cloudwatch-log", "enabled"}:       "true",
	})
}

// clientInfoTail is what the driver appends to every conf file, since efs-utils
// does not ship a [client-info] section.
const clientInfoTail = "\n[client-info]\nsource = k8s\n"

// verifyAgainstInstalled checks a written conf file is the installed fixture
// with only the driver's changes: each key in driverKeys rewritten to
// "key = value" in place (uncommented if efs-utils ships it commented out), and
// clientInfoTail appended. Any other line that moved, changed, or appeared
// fails the check. Comparing against the fixture itself instead of a stored
// copy of the output means refreshing a fixture needs no second file updated.
func verifyAgainstInstalled(t *testing.T, configFilePath, fixtureName string, driverKeys map[confKey]string) {
	t.Helper()
	data, err := os.ReadFile(configFilePath)
	checkError(t, err)
	got := string(data)

	installed := strings.TrimRight(readConfFixture(t, fixtureName), "\n")
	body, ok := strings.CutSuffix(got, "\n"+clientInfoTail)
	if !ok {
		t.Fatalf("%s does not end with the [client-info] section, tail:\n%s", fixtureName, got[max(0, len(got)-120):])
	}

	wantLines, gotLines := strings.Split(installed, "\n"), strings.Split(body, "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("%s has %d lines before [client-info], want the installed %d: a line was added or removed",
			fixtureName, len(gotLines), len(wantLines))
	}

	seen := map[confKey]bool{}
	section := ""
	for i, want := range wantLines {
		trimmed := strings.TrimSpace(want)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = trimmed[1 : len(trimmed)-1]
		}
		if gotLines[i] == want {
			continue
		}
		// The only line allowed to change is one holding a driver key.
		k, _, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(trimmed, "#")), "=")
		key := confKey{section, strings.TrimSpace(k)}
		value, isDriverKey := driverKeys[key]
		switch {
		case !isDriverKey:
			t.Errorf("%s line %d changed but holds no driver key:\nwant: %s\ngot:  %s", fixtureName, i+1, want, gotLines[i])
		case seen[key]:
			t.Errorf("%s line %d: %s rewritten a second time", fixtureName, i+1, key)
		case gotLines[i] != key.key+" = "+value:
			t.Errorf("%s line %d = %q, want %q", fixtureName, i+1, gotLines[i], key.key+" = "+value)
		}
		seen[key] = true
	}

	// A driver key whose installed value already matches leaves its line as is,
	// so check every key's final value rather than requiring each line to change.
	for key, want := range driverKeys {
		if v, ok := confValue(got, key); !ok || v != want {
			t.Errorf("%s: %s = %q (set=%v), want %q", fixtureName, key, v, ok, want)
		}
	}
}

func createFile(t *testing.T, dirName, fileName, fileContent string) {
	f := createFileInDir(t, dirName, fileName)
	_, err := f.WriteString(fileContent)
	checkError(t, err)
}

func TestWrite(t *testing.T) {
	redirect := newInfoRedirect("info")
	if _, err := redirect.Write([]byte("abc")); err != nil {
		t.Errorf("Failed to Write in redirect: %v", err)
	}
}

func TestSetupWithOverrides(t *testing.T) {
	configDirName := createTempDir(t)
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	defer os.RemoveAll(staticFileDirName)

	overrides := []ConfOverride{
		{Section: "mount-watchdog", Key: "stunnel_health_check_interval_min", Value: "1"},
		{Section: "mount-watchdog", Key: "tls_cert_renewal_interval_min", Value: "30"},
	}
	installConfFixtures(t, staticFileDirName)

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, overrides, nil, "sleep", "300").(*execWatchdog)
	if err := w.setup("k8s"); err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}

	efsConfigFilePath := filepath.Join(configDirName, efsConfigFileName)
	data, err := os.ReadFile(efsConfigFilePath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}
	result := string(data)
	if !strings.Contains(result, "stunnel_health_check_interval_min = 1") {
		t.Fatalf("Expected override applied to efs-utils.conf, got: %s", result)
	}
	if !strings.Contains(result, "tls_cert_renewal_interval_min = 30") {
		t.Fatalf("Expected override applied to efs-utils.conf, got: %s", result)
	}
}

func TestSetupWithS3FilesOverrides(t *testing.T) {
	configDirName := createTempDir(t)
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	defer os.RemoveAll(staticFileDirName)

	s3overrides := []ConfOverride{
		{Section: "proxy", Key: "read_bypass_denylist_size", Value: "20000"},
	}
	installConfFixtures(t, staticFileDirName)

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, s3overrides, "sleep", "300").(*execWatchdog)
	if err := w.setup("k8s"); err != nil {
		t.Fatalf("Failed to setup: %v", err)
	}

	s3filesConfigFilePath := filepath.Join(configDirName, s3filesConfigFileName)
	data, err := os.ReadFile(s3filesConfigFilePath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}
	result := string(data)
	if !strings.Contains(result, "read_bypass_denylist_size = 20000") {
		t.Fatalf("Expected override applied to s3files-utils.conf, got: %s", result)
	}
}

// confKey identifies one config entry, e.g. {"mount", "region"}.
type confKey struct {
	section string
	key     string
}

func (c confKey) String() string { return "[" + c.section + "] " + c.key }

// confValue returns the value set for section/key in an INI-style config, and
// whether it is set at all. A commented-out line does not count as set, which
// is how the upstream conf files ship the keys efs-utils reads from instance
// metadata instead.
func confValue(config string, want confKey) (string, bool) {
	inSection := false
	for _, line := range strings.Split(config, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			// Section headers may carry trailing text, which ConfigParser ignores.
			inSection = strings.HasPrefix(trimmed, "["+want.section+"]")
			continue
		}
		if !inSection || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eqPos := strings.Index(trimmed, "=")
		if eqPos < 0 {
			continue
		}
		if strings.TrimSpace(trimmed[:eqPos]) == want.key {
			return strings.TrimSpace(trimmed[eqPos+1:]), true
		}
	}
	return "", false
}

// TestSetupSetsDriverConfKeys checks that every config key the driver owns is
// written from its flag or environment variable, for both conf files. That the
// rest of each file is left as installed is checked by verifyAgainstInstalled.
func TestSetupSetsDriverConfKeys(t *testing.T) {
	testCases := []struct {
		name                            string
		region                          string
		fipsEnabled                     string
		portRangeUpperBound             string
		debugLogs                       bool
		efsCloudWatchLogEnabled         bool
		s3filesCloudWatchLogEnabled     bool
		s3filesCloudWatchMetricsEnabled bool
		wantEfs                         map[confKey]string
		wantS3Files                     map[confKey]string
		wantEfsUnset                    []confKey
		wantS3FilesUnset                []confKey
	}{
		{
			name: "defaults",
			wantEfs: map[confKey]string{
				{"DEFAULT", "logging_level"}:        "INFO",
				{"mount", "stunnel_debug_enabled"}:  "false",
				{"mount", "fips_mode_enabled"}:      "false",
				{"mount", "port_range_upper_bound"}: "21049",
				{"cloudwatch-log", "enabled"}:       "false",
				{"client-info", "source"}:           "k8s",
			},
			wantS3Files: map[confKey]string{
				{"DEFAULT", "logging_level"}:        "INFO",
				{"mount", "fips_mode_enabled"}:      "false",
				{"mount", "port_range_upper_bound"}: "21049",
				{"proxy", "proxy_logging_level"}:    "INFO",
				{"proxy", "metrics_enabled"}:        "false",
				{"cloudwatch-log", "enabled"}:       "false",
				{"client-info", "source"}:           "k8s",
			},
			// With no AWS_DEFAULT_REGION the region key must stay commented out
			// so efs-utils reads the region from instance metadata.
			wantEfsUnset:     []confKey{{"mount", "region"}},
			wantS3FilesUnset: []confKey{{"mount", "region"}},
		},
		{
			name:                            "all flags set",
			region:                          "us-west-2",
			fipsEnabled:                     "true",
			portRangeUpperBound:             "22049",
			debugLogs:                       true,
			efsCloudWatchLogEnabled:         true,
			s3filesCloudWatchLogEnabled:     true,
			s3filesCloudWatchMetricsEnabled: true,
			wantEfs: map[confKey]string{
				{"DEFAULT", "logging_level"}:        "DEBUG",
				{"mount", "region"}:                 "us-west-2",
				{"mount", "stunnel_debug_enabled"}:  "true",
				{"mount", "fips_mode_enabled"}:      "true",
				{"mount", "port_range_upper_bound"}: "22049",
				{"cloudwatch-log", "enabled"}:       "true",
				{"client-info", "source"}:           "k8s",
			},
			wantS3Files: map[confKey]string{
				{"DEFAULT", "logging_level"}:        "DEBUG",
				{"mount", "region"}:                 "us-west-2",
				{"mount", "fips_mode_enabled"}:      "true",
				{"mount", "port_range_upper_bound"}: "22049",
				{"proxy", "proxy_logging_level"}:    "DEBUG",
				{"proxy", "metrics_enabled"}:        "true",
				{"cloudwatch-log", "enabled"}:       "true",
				{"client-info", "source"}:           "k8s",
			},
		},
		{
			name:                        "cloudwatch logs on, proxy metrics off",
			efsCloudWatchLogEnabled:     true,
			s3filesCloudWatchLogEnabled: true,
			wantEfs: map[confKey]string{
				{"cloudwatch-log", "enabled"}: "true",
			},
			wantS3Files: map[confKey]string{
				{"cloudwatch-log", "enabled"}: "true",
				{"proxy", "metrics_enabled"}:  "false",
			},
		},
		{
			name: "port range upper bound below the floor falls back to the default",
			// efs-utils needs at least one port above port_range_lower_bound,
			// so a smaller value is replaced rather than written through.
			portRangeUpperBound: "100",
			wantEfs: map[confKey]string{
				{"mount", "port_range_upper_bound"}: "21049",
			},
			wantS3Files: map[confKey]string{
				{"mount", "port_range_upper_bound"}: "21049",
			},
		},
		{
			name:                "non-numeric port range upper bound falls back to the default",
			portRangeUpperBound: "not-a-number",
			wantEfs: map[confKey]string{
				{"mount", "port_range_upper_bound"}: "21049",
			},
			wantS3Files: map[confKey]string{
				{"mount", "port_range_upper_bound"}: "21049",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AWS_DEFAULT_REGION", tc.region)
			t.Setenv("FIPS_ENABLED", tc.fipsEnabled)
			t.Setenv("PORT_RANGE_UPPER_BOUND", tc.portRangeUpperBound)

			configDirName := t.TempDir()
			staticFileDirName := t.TempDir()
			installConfFixtures(t, staticFileDirName)
			w := newExecWatchdog(configDirName, staticFileDirName, tc.debugLogs, tc.efsCloudWatchLogEnabled,
				tc.s3filesCloudWatchLogEnabled, tc.s3filesCloudWatchMetricsEnabled, nil, nil, "sleep", "300").(*execWatchdog)
			if err := w.setup("k8s"); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			for _, f := range []struct {
				fileName string
				want     map[confKey]string
				unset    []confKey
			}{
				{efsConfigFileName, tc.wantEfs, tc.wantEfsUnset},
				{s3filesConfigFileName, tc.wantS3Files, tc.wantS3FilesUnset},
			} {
				data, err := os.ReadFile(filepath.Join(configDirName, f.fileName))
				if err != nil {
					t.Fatalf("failed to read %s: %v", f.fileName, err)
				}
				config := string(data)
				for key, wantValue := range f.want {
					got, ok := confValue(config, key)
					if !ok {
						t.Errorf("%s: %s is not set, want %q", f.fileName, key, wantValue)
						continue
					}
					if got != wantValue {
						t.Errorf("%s: %s = %q, want %q", f.fileName, key, got, wantValue)
					}
				}
				for _, key := range f.unset {
					if got, ok := confValue(config, key); ok {
						t.Errorf("%s: %s is set to %q, want it left commented out", f.fileName, key, got)
					}
				}
			}
		})
	}
}

// TestDriverConfKeysExistInConf catches a typo or a stale name in the driver's
// key list by applying each key strictly to a real efs-utils conf. At runtime a
// missing key is only a warning (the driver adds it), so without this test a
// typo would write a line efs-utils never reads. It cannot see an upstream
// rename, since the driver reads the installed file at runtime and the fixture
// here is a copy.
func TestDriverConfKeysExistInConf(t *testing.T) {
	cfg := utilsConfig{EfsClientSource: "k8s", Region: "us-west-2", FipsEnabled: "false", PortRangeUpperBound: "21049"}

	testCases := []struct {
		fileName string
		keys     []ConfOverride
	}{
		{efsConfigFileName, efsUtilsDriverConf(cfg)},
		{s3filesConfigFileName, s3filesUtilsDriverConf(cfg)},
	}

	for _, tc := range testCases {
		t.Run(tc.fileName, func(t *testing.T) {
			conf := readConfFixture(t, tc.fileName)
			if len(tc.keys) == 0 {
				t.Fatalf("no driver keys for %s", tc.fileName)
			}
			for _, key := range tc.keys {
				if _, err := applyConfOverrides(conf, []ConfOverride{key}); err != nil {
					t.Errorf("driver sets [%s] %s but %s does not have it: %v",
						key.Section, key.Key, tc.fileName, err)
				}
			}
		})
	}
}

// TestSetupReadsInstalledConf checks the written config comes from the conf
// efs-utils installed and not from anything compiled into the driver. The
// sentinel is a key the driver never sets, so it can only reach the output by
// being read off disk.
func TestSetupReadsInstalledConf(t *testing.T) {
	clearConfEnv(t)
	configDirName := t.TempDir()
	staticFileDirName := t.TempDir()

	const sentinel = "logging_max_bytes = 4242"
	for _, fileName := range []string{efsConfigFileName, s3filesConfigFileName} {
		conf := readConfFixture(t, fileName)
		if !strings.Contains(conf, "logging_max_bytes = 1048576") {
			t.Fatalf("the %s fixture no longer has the line this test rewrites", fileName)
		}
		installConfFixture(t, staticFileDirName, fileName,
			strings.Replace(conf, "logging_max_bytes = 1048576", sentinel, 1))
	}

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300").(*execWatchdog)
	if err := w.setup("k8s"); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for _, fileName := range []string{efsConfigFileName, s3filesConfigFileName} {
		data, err := os.ReadFile(filepath.Join(configDirName, fileName))
		if err != nil {
			t.Fatalf("failed to read %s: %v", fileName, err)
		}
		if !strings.Contains(string(data), sentinel) {
			t.Errorf("%s does not carry %q from the installed conf", fileName, sentinel)
		}
	}
}

// TestSetupFailsOnMissingInstalledConf checks that a conf file missing from the
// static files directory stops setup. The image's efs-utils always ships both,
// so a missing one means a broken image rather than an older efs-utils.
func TestSetupFailsOnMissingInstalledConf(t *testing.T) {
	for _, missing := range []string{efsConfigFileName, s3filesConfigFileName} {
		t.Run(missing, func(t *testing.T) {
			clearConfEnv(t)
			configDirName := t.TempDir()
			staticFileDirName := t.TempDir()
			for _, fileName := range []string{efsConfigFileName, s3filesConfigFileName} {
				if fileName != missing {
					installConfFixture(t, staticFileDirName, fileName, readConfFixture(t, fileName))
				}
			}

			w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300").(*execWatchdog)
			err := w.setup("k8s")
			if err == nil {
				t.Fatalf("expected setup to fail with no %s installed", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("expected the error to name %s, got: %v", missing, err)
			}
		})
	}
}

// TestUpdateConfigIgnoresStaleConfInConfigDirectory pins which copy counts as
// the installed one. The config directory is a host path that outlives the pod,
// so it can still hold a conf file an earlier driver wrote there. Only what the
// efs-utils package installed may be used as the base, otherwise a value from an
// older efs-utils or an earlier override would carry over across upgrades.
//
// It calls updateConfig rather than setup: setup first runs restoreStaticFiles,
// which copies the installed conf over the config directory, so the stale copy
// would be gone before the read and the test would pass whichever copy is read.
func TestUpdateConfigIgnoresStaleConfInConfigDirectory(t *testing.T) {
	clearConfEnv(t)
	configDirName := t.TempDir()
	staticFileDirName := t.TempDir()
	installConfFixtures(t, staticFileDirName)

	// An earlier driver left both conf files behind with a value for a key the
	// driver does not set, so only the base file decides it.
	const installed, stale = "logging_max_bytes = 1048576", "logging_max_bytes = 4242"
	for _, fileName := range []string{efsConfigFileName, s3filesConfigFileName} {
		conf := readConfFixture(t, fileName)
		if !strings.Contains(conf, installed) {
			t.Fatalf("the %s fixture no longer has the line this test rewrites", fileName)
		}
		if err := os.WriteFile(filepath.Join(configDirName, fileName),
			[]byte(strings.Replace(conf, installed, stale, 1)), 0644); err != nil {
			t.Fatalf("failed to write the stale %s: %v", fileName, err)
		}
	}

	w := newExecWatchdog(configDirName, staticFileDirName, false, false, true, true, nil, nil, "sleep", "300").(*execWatchdog)
	if err := w.updateConfig("k8s"); err != nil {
		t.Fatalf("updateConfig failed: %v", err)
	}

	for _, fileName := range []string{efsConfigFileName, s3filesConfigFileName} {
		data, err := os.ReadFile(filepath.Join(configDirName, fileName))
		if err != nil {
			t.Fatalf("failed to read %s: %v", fileName, err)
		}
		if strings.Contains(string(data), stale) || !strings.Contains(string(data), installed) {
			t.Errorf("%s was built from the stale copy in the config directory, not the installed one", fileName)
		}
	}
}

// TestWriteConfigFileAddsMissingDriverKey checks a driver key the installed conf
// does not have is still written, as the driver did when it rendered its own
// template, instead of failing startup. That covers a key missing from an
// existing section and a section missing altogether.
func TestWriteConfigFileAddsMissingDriverKey(t *testing.T) {
	dir := t.TempDir()
	w := &execWatchdog{efsUtilsCfgPath: dir}
	conf := "[DEFAULT]\nlogging_level = INFO\n\n[mount]\ndns_name_suffix = amazonaws.com\n\n[mount-watchdog]\nenabled = true\n"
	driverConf := []ConfOverride{
		{Section: "mount", Key: "port_range_upper_bound", Value: "21049"},
		{Section: "cloudwatch-log", Key: "enabled", Value: "false"},
	}

	if err := w.writeConfigFile(efsConfigFileName, conf, utilsConfig{EfsClientSource: "k8s"}, driverConf, nil); err != nil {
		t.Fatalf("writeConfigFile failed on a missing driver key: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, efsConfigFileName))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "[mount]\ndns_name_suffix = amazonaws.com\nport_range_upper_bound = 21049\n\n[mount-watchdog]") {
		t.Errorf("expected port_range_upper_bound at the end of [mount], got:\n%s", got)
	}
	if v, ok := confValue(got, confKey{"cloudwatch-log", "enabled"}); !ok || v != "false" {
		t.Errorf("expected an added [cloudwatch-log] enabled = false, got %q (set=%v) in:\n%s", v, ok, got)
	}
}

// driverAddedKeyLogMarker is the part of the missing-driver-key warning that the
// e2e test greps driver logs for (utilsConfDriverAddedKeyLog in
// test/e2e/utils_conf_test.go). Keep the two in step: if the warning stops
// containing it, the e2e drift check silently passes.
const driverAddedKeyLogMarker = "so the driver added it"

// captureKlog sends klog output to a buffer for the rest of the test.
func captureKlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	for name, value := range map[string]string{"logtostderr": "false", "alsologtostderr": "false", "stderrthreshold": "FATAL"} {
		if err := fs.Set(name, value); err != nil {
			t.Fatalf("setting klog flag %s: %v", name, err)
		}
	}
	var buf bytes.Buffer
	klog.SetOutput(&buf)
	t.Cleanup(func() {
		klog.Flush()
		_ = fs.Set("logtostderr", "true")
		klog.SetOutput(os.Stderr)
	})
	return &buf
}

// TestWriteConfigFileWarnsOnMissingDriverKey checks the warning the e2e drift
// check depends on: logged for a driver key the installed conf lacks, naming the
// file, section and key, and not logged when the key is there.
func TestWriteConfigFileWarnsOnMissingDriverKey(t *testing.T) {
	driverConf := []ConfOverride{{Section: "mount", Key: "port_range_upper_bound", Value: "21049"}}

	t.Run("missing key", func(t *testing.T) {
		logs := captureKlog(t)
		w := &execWatchdog{efsUtilsCfgPath: t.TempDir()}
		conf := "[mount]\ndns_name_suffix = amazonaws.com\n"
		if err := w.writeConfigFile(efsConfigFileName, conf, utilsConfig{EfsClientSource: "k8s"}, driverConf, nil); err != nil {
			t.Fatalf("writeConfigFile: %v", err)
		}
		klog.Flush()
		var warning string
		for _, line := range strings.Split(logs.String(), "\n") {
			if strings.HasPrefix(line, "W") && strings.Contains(line, driverAddedKeyLogMarker) {
				warning = line
			}
		}
		if warning == "" {
			t.Fatalf("expected a warning containing %q, got logs:\n%s", driverAddedKeyLogMarker, logs.String())
		}
		for _, want := range []string{efsConfigFileName, "[mount]", "port_range_upper_bound"} {
			if !strings.Contains(warning, want) {
				t.Errorf("warning does not name %q: %s", want, warning)
			}
		}
	})

	t.Run("key present", func(t *testing.T) {
		logs := captureKlog(t)
		w := &execWatchdog{efsUtilsCfgPath: t.TempDir()}
		conf := "[mount]\nport_range_upper_bound = 21049\n"
		if err := w.writeConfigFile(efsConfigFileName, conf, utilsConfig{EfsClientSource: "k8s"}, driverConf, nil); err != nil {
			t.Fatalf("writeConfigFile: %v", err)
		}
		klog.Flush()
		if strings.Contains(logs.String(), driverAddedKeyLogMarker) {
			t.Errorf("unexpected missing-key warning:\n%s", logs.String())
		}
	})
}

// TestOperatorOverrideStillRejectsMissingKey checks the leniency is for driver
// keys only: an operator override naming a key the conf does not have is a
// startup error, as docs/parameters.md documents.
func TestOperatorOverrideStillRejectsMissingKey(t *testing.T) {
	w := &execWatchdog{efsUtilsCfgPath: t.TempDir()}
	conf := "[DEFAULT]\nlogging_level = INFO\n\n[mount]\ndns_name_suffix = amazonaws.com\n"

	err := w.writeConfigFile(efsConfigFileName, conf, utilsConfig{EfsClientSource: "k8s"}, nil,
		[]ConfOverride{{Section: "mount", Key: "no_such_key", Value: "1"}})
	if err == nil || !strings.Contains(err.Error(), "no_such_key") {
		t.Fatalf("expected an error naming no_such_key, got: %v", err)
	}
}

// TestOperatorOverrideWinsOverDriverKey pins the precedence the
// --efs-utils-conf-overrides flag documents.
func TestOperatorOverrideWinsOverDriverKey(t *testing.T) {
	configDirName := t.TempDir()
	overrides := []ConfOverride{{Section: "cloudwatch-log", Key: "enabled", Value: "false"}}

	staticFileDirName := t.TempDir()
	installConfFixtures(t, staticFileDirName)

	// efsCloudWatchLogEnabled is true, so the driver key and the operator
	// override disagree and only the override may survive.
	w := newExecWatchdog(configDirName, staticFileDirName, false, true, true, true, overrides, nil, "sleep", "300").(*execWatchdog)
	if err := w.setup("k8s"); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(configDirName, efsConfigFileName))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if got, _ := confValue(string(data), confKey{"cloudwatch-log", "enabled"}); got != "false" {
		t.Errorf("[cloudwatch-log] enabled = %q, want the operator override %q", got, "false")
	}
}
