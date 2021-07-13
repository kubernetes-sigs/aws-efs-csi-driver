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
	"io/ioutil"
	"os"
	"path/filepath"
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

# Define the port range that the TLS tunnel will choose from
port_range_lower_bound = 20049
port_range_upper_bound = 20449

# Optimize read_ahead_kb for Linux 5.4+
optimize_readahead = true

# By default, we enable the feature to fallback to mount with mount target ip address when dns name cannot be resolved
fall_back_to_mount_target_ip_address_enabled = true

# By default, we use IMDSv2 to get the instance metadata, set this to true if you want to disable IMDSv2 usage
disable_fetch_ec2_metadata_token = false


[mount.cn-north-1]
dns_name_suffix = amazonaws.com.cn


[mount.cn-northwest-1]
dns_name_suffix = amazonaws.com.cn


[mount.us-iso-east-1]
dns_name_suffix = c2s.ic.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount.us-isob-east-1]
dns_name_suffix = sc2s.sgov.gov
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

[mount-watchdog]
enabled = true
poll_interval_sec = 1
unmount_grace_period_sec = 30

# Set client auth/access point certificate renewal rate. Minimum value is 1 minute.
tls_cert_renewal_interval_min = 60

[client-info] 
source=k8s

[cloudwatch-log]
# enabled = true
log_group_name = /aws/efs/utils

# Possible values are : 1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1827, and 3653
# Comment this config to prevent log deletion
retention_in_days = 14
`
	configFileName = "efs-utils.conf"
)

func TestExecWatchdog(t *testing.T) {
	configDirName := createTempDir(t)
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	defer os.RemoveAll(staticFileDirName)

	w := newExecWatchdog(configDirName, staticFileDirName, "sleep", "300")
	if err := w.start(); err != nil {
		t.Fatalf("Failed to start %v", err)
	}
	time.Sleep(time.Second)
	w.stop()
}

func createTempDir(t *testing.T) string {
	name, err := ioutil.TempDir("", "")
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

	w := newExecWatchdog(configDirName, staticFileDirName, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	configFilePath := filepath.Join(configDirName, configFileName)
	if err := w.setup(efsClient); err != nil {
		t.Fatalf("Failed to update config file %v, %v", configFilePath, err)
	}

	verifyConfigFile(t, configFilePath)

	//verify file A and B are copied over to static file directory
	verifyFileContent(t, filepath.Join(configDirName, "A"), fileAContent)
	verifyFileContent(t, filepath.Join(configDirName, "B"), fileBContent)
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

	w := newExecWatchdog(configDirName, staticFileDirName, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	configFilePath := filepath.Join(configDirName, configFileName)
	if err := w.setup(efsClient); err != nil {
		t.Fatalf("Failed to update config file %v, %v", configFilePath, err)
	}

	verifyConfigFile(t, configFilePath)

	//verify file A is copied over from static files directory but file B in the config direcotry remains as is
	verifyFileContent(t, filepath.Join(configDirName, "A"), fileAContent)
	verifyFileContent(t, filepath.Join(configDirName, "B"), differentContent)
}

func TestSetupWithNonexistentConfigDirectory(t *testing.T) {
	configDirName := ""
	staticFileDirName := createTempDir(t)
	defer os.RemoveAll(staticFileDirName)
	w := newExecWatchdog(configDirName, staticFileDirName, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	if err := w.setup(efsClient); err == nil {
		t.Fatalf("Expected failure since static files directory doesn't exist.")
	}
}

func TestSetupWithNonexistentStaticFilesDirectory(t *testing.T) {
	configDirName := createTempDir(t)
	defer os.RemoveAll(configDirName)
	staticFileDirName := ""
	w := newExecWatchdog(configDirName, staticFileDirName, "sleep", "300").(*execWatchdog)
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

	_, err := ioutil.TempDir(staticFileDirName, "")
	checkError(t, err)

	w := newExecWatchdog(configDirName, staticFileDirName, "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	if err := w.setup(efsClient); err == nil {
		t.Fatalf("Expected failure since config directory contains another directory.")
	}
}

func verifyFileContent(t *testing.T, fileName string, expectedFileContent string) {
	fileContent, err := ioutil.ReadFile(fileName)
	if err != nil {
		t.Fatalf("Failed to read file %v, %v", fileName, err)
	}
	actualFileContent := string(fileContent)
	if actualFileContent != expectedFileContent {
		t.Fatalf("Unexpected A file config content: want %s\nactual:%s", expectedFileContent, actualFileContent)
	}
}

func verifyConfigFile(t *testing.T, configFilePath string) {
	configFileContent, err := ioutil.ReadFile(configFilePath)
	checkError(t, err)
	actualConfig := string(configFileContent)
	if actualConfig != expectedEfsUtilsConfig {
		t.Fatalf("Unexpected efs-utils config content: want %s\nactual:%s", expectedEfsUtilsConfig, actualConfig)
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
