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
	"testing"
	"time"
)

const expectedEfsUtilsConfig = `
[DEFAULT]
logging_level = INFO
logging_max_bytes = 1048576
logging_file_count = 10
# mode for /var/run/efs and subdirectories in octal
state_file_dir_mode = 750

[mount]
dns_name_format = {fs_id}.efs.{region}.{dns_name_suffix}
dns_name_suffix = amazonaws.com
#The region of the file system when mounting from on-premises or cross region.
#region = us-east-1
stunnel_debug_enabled = true
#Uncomment the below option to save all stunnel logs for a file system to the same file
stunnel_logs_file = /var/log/amazon/efs/{fs_id}.stunnel.log
# RootCA on AmazonLinux2/CentOS. https://golang.org/src/crypto/x509/root_linux.go
stunnel_cafile = /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem

# Validate the certificate hostname on mount. This option is not supported by certain stunnel versions.
stunnel_check_cert_hostname = true

# Use OCSP to check certificate validity. This option is not supported by certain stunnel versions.
stunnel_check_cert_validity = false

# Define the port range that the TLS tunnel will choose from
port_range_lower_bound = 20049
port_range_upper_bound = 20449

[mount.cn-north-1]
dns_name_suffix = amazonaws.com.cn

[mount.cn-northwest-1]
dns_name_suffix = amazonaws.com.cn

[mount.us-iso-east-1]
dns_name_suffix = c2s.ic.gov

[mount.us-isob-east-1]
dns_name_suffix = sc2s.sgov.gov

[mount-watchdog]
enabled = true
poll_interval_sec = 1
unmount_grace_period_sec = 30

# Set client auth/access point certificate renewal rate. Minimum value is 1 minute.
tls_cert_renewal_interval_min = 60

[client-info] 
source=k8s
`

func TestExecWatchdog(t *testing.T) {
	f := createEmptyConfigFile(t)
	defer os.Remove(f.Name())

	w := newExecWatchdog(f.Name(), "sleep", "300")
	if err := w.start(); err != nil {
		t.Fatalf("Failed to start %v", err)
	}
	time.Sleep(time.Second)
	w.stop()
}

func createEmptyConfigFile(t *testing.T) *os.File {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("couldn't create temp file %v, %v", f, err)
	}
	return f
}

func TestUpdateConfig(t *testing.T) {
	f := createEmptyConfigFile(t)
	defer os.Remove(f.Name())

	w := newExecWatchdog(f.Name(), "sleep", "300").(*execWatchdog)
	efsClient := "k8s"
	if err := w.updateConfig(efsClient); err != nil {
		t.Fatalf("Failed to update config file %v, %v", f, err)
	}
	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read config file %v, %v", f, err)
	}
	actualConfig := string(bytes)
	if actualConfig != expectedEfsUtilsConfig {
		t.Errorf("Unexpected efs-utils config content: want %s\nactual:%s", expectedEfsUtilsConfig, actualConfig)
	}
}

func TestWrite(t *testing.T) {
	redirect := newInfoRedirect("info")
	if _, err := redirect.Write([]byte("abc")); err != nil {
		t.Errorf("Failed to Write in redirect: %v", err)
	}
}
