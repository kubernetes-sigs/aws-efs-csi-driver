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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	expectedEfsUtilsConfig = `
#
# Copyright 2017-2018 Amazon.com, Inc. and its affiliates. All Rights Reserved.
#
# Licensed under the MIT License. See the LICENSE accompanying this file
# for the specific language governing permissions and limitations under
# the License.
#

[DEFAULT]
logging_level = INFO
logging_max_bytes = 1048576
logging_file_count = 10
# mode for /var/run/efs and subdirectories in octal
state_file_dir_mode = 750

[mount]
dns_name_format = {az}.{fs_id}.efs.{region}.{dns_name_suffix}
dns_name_suffix = amazonaws.com
#The region of the file system when mounting from on-premises or cross region.
#region = us-east-1
stunnel_debug_enabled = false
#Uncomment the below option to save all stunnel logs for a file system to the same file
#stunnel_logs_file = /var/log/amazon/efs/{fs_id}.stunnel.log
stunnel_cafile = /etc/amazon/efs/efs-utils.crt

# Validate the certificate hostname on mount. This option is not supported by certain stunnel versions.
stunnel_check_cert_hostname = true

# Use OCSP to check certificate validity. This option is not supported by certain stunnel versions.
stunnel_check_cert_validity = false

# Enable FIPS mode. stunnel complains if FIPS is available and enabled system-wide, but not set here.
fips_mode_enabled = false

# Define the port range that the TLS tunnel will choose from
port_range_lower_bound = 20049
port_range_upper_bound = 21049

# Optimize read_ahead_kb for Linux 5.4+
optimize_readahead = true

# By default, we enable the feature to fallback to mount with mount target ip address when dns name cannot be resolved
fall_back_to_mount_target_ip_address_enabled = true

# By default, we use IMDSv2 to get the instance metadata, set this to true if you want to disable IMDSv2 usage
disable_fetch_ec2_metadata_token = false

# By default, we enable efs-utils to retry failed mount.nfs command that due to (1) connection reset by peer (2) the
# mount.nfs is not finished within 'retry_nfs_mount_command_timeout_sec'. If the retry count is set as N, initial N - 1
# mount attempts will timeout if the command does not finish within 'retry_nfs_mount_command_timeout_sec' sec.
# The last mount attempt will keep the existing behavior of mount.nfs.
#
retry_nfs_mount_command = true
retry_nfs_mount_command_count = 3
retry_nfs_mount_command_timeout_sec = 15

[mount.cn-north-1]
dns_name_suffix = amazonaws.com.cn

[mount.cn-northwest-1]
dns_name_suffix = amazonaws.com.cn

[mount.eu-isoe-west-1]
dns_name_suffix = cloud.adc-e.uk
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.eusc-de-east-1]
dns_name_suffix = amazonaws.eu
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-iso-east-1]
dns_name_suffix = c2s.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-iso-west-1]
dns_name_suffix = c2s.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isob-east-1]
dns_name_suffix = sc2s.sgov.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isob-west-1]
dns_name_suffix = sc2s.sgov.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isof-east-1]
dns_name_suffix = csp.hci.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isof-south-1]
dns_name_suffix = csp.hci.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount-watchdog]
enabled = true
poll_interval_sec = 1
unmount_count_for_consistency = 5
unmount_grace_period_sec = 30

# Set client auth/access point certificate renewal rate. Minimum value is 1 minute.
tls_cert_renewal_interval_min = 60

# Periodically check the health of stunnel to make sure the connection is fully established
stunnel_health_check_enabled = true
stunnel_health_check_interval_min = 5
stunnel_health_check_command_timeout_sec = 30

[client-info] 
source=k8s

[cloudwatch-log]
enabled = false
log_group_name = /aws/efs/utils

# Possible values are : 1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1827, and 3653
# Comment this config to prevent log deletion
retention_in_days = 14
`
	efsConfigFileName     = "efs-utils.conf"
	s3filesConfigFileName = "s3files-utils.conf"

	expectedS3FilesConfig = `
#
# Copyright 2017-2018 Amazon.com, Inc. and its affiliates. All Rights Reserved.
#
# Licensed under the MIT License. See the LICENSE accompanying this file
# for the specific language governing permissions and limitations under
# the License.
#

[DEFAULT]
logging_level = INFO
logging_max_bytes = 1048576
logging_file_count = 10
# mode for /var/run/efs and subdirectories in octal
state_file_dir_mode = 750

[mount]
dns_name_format = {az_id}.{fs_id}.s3files.{region}.{dns_name_suffix}
dns_name_suffix = on.aws
#The region of the file system when mounting from on-premises or cross region.
#region = us-east-1
#Uncomment the below option to save all stunnel logs for a file system to the same file
#stunnel_logs_file = /var/log/amazon/efs/{fs_id}.stunnel.log
stunnel_cafile = /etc/amazon/efs/efs-utils.crt

# Validate the certificate hostname on mount. This option is not supported by certain stunnel versions.
stunnel_check_cert_hostname = true

# Use OCSP to check certificate validity. This option is not supported by certain stunnel versions.
stunnel_check_cert_validity = false

# Set to true to use FIPS-mode for stunnel. Enabling this will change the AWS SDK client to use FIPS as well.
fips_mode_enabled = false

# Define the port range that the TLS tunnel will choose from
port_range_lower_bound = 20049
port_range_upper_bound = 21049

# Optimize read_ahead_kb for Linux 5.4+
optimize_readahead = true

# By default, we enable the feature to fallback to mount with mount target ip address when dns name cannot be resolved
fall_back_to_mount_target_ip_address_enabled = true

# By default, we use IMDSv2 to get the instance metadata, set this to true if you want to disable IMDSv2 usage
disable_fetch_ec2_metadata_token = false

# By default, we enable efs-utils to retry failed mount.nfs command that due to (1) connection reset by peer (2) the
# mount.nfs is not finished within 'retry_nfs_mount_command_timeout_sec'. If the retry count is set as N, initial N - 1
# mount attempts will timeout if the command does not finish within 'retry_nfs_mount_command_timeout_sec' sec.
# The last mount attempt will keep the existing behavior of mount.nfs.
#
retry_nfs_mount_command = true
retry_nfs_mount_command_count = 3
retry_nfs_mount_command_timeout_sec = 15

[mount.cn-north-1]
dns_name_suffix = amazonaws.com.cn

[mount.cn-northwest-1]
dns_name_suffix = amazonaws.com.cn

[mount.eu-isoe-west-1]
dns_name_suffix = cloud.adc-e.uk
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.eusc-de-east-1]
dns_name_suffix = amazonaws.eu
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-iso-east-1]
dns_name_suffix = c2s.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-iso-west-1]
dns_name_suffix = c2s.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isob-east-1]
dns_name_suffix = sc2s.sgov.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isob-west-1]
dns_name_suffix = sc2s.sgov.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isof-east-1]
dns_name_suffix = csp.hci.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isof-south-1]
dns_name_suffix = csp.hci.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

# Note: mount-watchdog settings are configured in /etc/amazon/efs/efs-utils.conf
# The watchdog process monitors all mount types and reads its configuration from that file.

[proxy]
proxy_logging_level = INFO
proxy_logging_max_bytes = 1048576
proxy_logging_file_count = 10

# CloudWatch metric emission from proxy, set to false to disable / opt-out
metrics_enabled = true

# Readbypass parameters, Uncomment the below options if you want override default value
# read_bypass_denylist_size = 10000
# read_bypass_denylist_ttl_seconds = 300
# readahead_cache_enabled = true
# s3_read_chunk_size_bytes = 1048576
# readahead_cache_init_memory_size_mb = 10
# readahead_cache_max_memory_size_mb = 1024
# readahead_init_window_size_bytes = 8388608
# readahead_max_window_size_bytes = 8388608
# readahead_cache_eviction_interval_ms = 500
# readahead_cache_target_utilization_percent = 80
# read_bypass_max_in_flight_s3_bytes = 268435456

[client-info] 
source=k8s

[cloudwatch-log]
enabled = true
log_group_name = /aws/efs/utils

# Possible values are : 1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1827, and 3653
# Comment this config to prevent log deletion
retention_in_days = 14
`
)

func TestExecWatchdog(t *testing.T) {
	configDirName := createTempDir(t)
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	defer os.RemoveAll(staticFileDirName)

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

func TestSetupWithEmptyConfigDirectory(t *testing.T) {
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

func verifyEfsConfigFile(t *testing.T, efsConfigFilePath string) {
	configFileContent, err := os.ReadFile(efsConfigFilePath)
	checkError(t, err)
	actualConfig := string(configFileContent)
	if actualConfig != expectedEfsUtilsConfig {
		t.Fatalf("Unexpected efs-utils config content: want %s\nactual:%s", expectedEfsUtilsConfig, actualConfig)
	}
}

func verifyS3FilesConfigFile(t *testing.T, efsConfigFilePath string) {
	configFileContent, err := os.ReadFile(efsConfigFilePath)
	checkError(t, err)
	actualConfig := string(configFileContent)
	if actualConfig != expectedS3FilesConfig {
		t.Fatalf("Unexpected s3files-utils config content: want %s\nactual:%s", expectedS3FilesConfig, actualConfig)
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
