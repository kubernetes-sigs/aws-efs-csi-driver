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
	"fmt"
	"os"
	"os/exec"
	"sync"
	"text/template"

	"k8s.io/klog"
)

// https://github.com/aws/efs-utils/blob/master/dist/efs-utils.conf
const efsUtilsConfigTemplate = `
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
source={{.EfsClientSource}}
`

// Watchdog defines the interface for process monitoring and supervising
type Watchdog interface {
	// start starts the watch dog along with the process
	start() error

	// stop stops the watch dog along with the process
	stop()
}

// execWatchdog is a watch dog that monitors a process and restart it
// if it has crashed accidentally
type execWatchdog struct {
	// the command to be exec and monitored
	execCmd string
	// the command arguments
	execArg []string
	// the cmd that is running
	cmd *exec.Cmd
	// efs-utils config file path
	efsUtilsCfgPath string
	// stopCh indicates if it should be stopped
	stopCh chan struct{}

	mu sync.Mutex
}

type efsUtilsConfig struct {
	EfsClientSource string
}

func newExecWatchdog(efsUtilsCfgPath, cmd string, arg ...string) Watchdog {
	return &execWatchdog{
		efsUtilsCfgPath: efsUtilsCfgPath,
		execCmd:         cmd,
		execArg:         arg,
		stopCh:          make(chan struct{}),
	}
}

func (w *execWatchdog) start() error {
	if err := w.updateConfig(GetVersion().EfsClientSource); err != nil {
		return err
	}

	go w.runLoop(w.stopCh)

	return nil
}

func (w *execWatchdog) updateConfig(efsClientSource string) error {
	efsCfgTemplate := template.Must(template.New("efs-utils-config").Parse(efsUtilsConfigTemplate))
	f, err := os.Create(w.efsUtilsCfgPath)
	if err != nil {
		return fmt.Errorf("cannot create config file %s for efs-utils. Error: %v", w.efsUtilsCfgPath, err)
	}
	defer f.Close()
	efsCfg := efsUtilsConfig{EfsClientSource: efsClientSource}
	if err = efsCfgTemplate.Execute(f, efsCfg); err != nil {
		return fmt.Errorf("cannot update config %s for efs-utils. Error: %v", w.efsUtilsCfgPath, err)
	}
	return nil
}

// stop kills the underlying process and stops the watchdog
func (w *execWatchdog) stop() {
	close(w.stopCh)

	w.mu.Lock()
	if w.cmd.Process != nil {
		p := w.cmd.Process
		err := p.Kill()
		if err != nil {
			klog.Errorf("Failed to kill process: %s", err)
		}
	}
	w.mu.Unlock()
}

// runLoop starts the monitoring loop
func (w *execWatchdog) runLoop(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			klog.Info("stopping...")
			break
		default:
			err := w.exec()
			if err != nil {
				klog.Errorf("Process %s exits %s", w.execCmd, err)
			}
		}
	}
}

func (w *execWatchdog) exec() error {
	cmd := exec.Command(w.execCmd, w.execArg...)
	cmd.Stdout = newInfoRedirect(w.execCmd)
	cmd.Stderr = newErrRedirect(w.execCmd)

	w.cmd = cmd

	w.mu.Lock()
	err := cmd.Start()
	if err != nil {
		return err
	}
	w.mu.Unlock()

	return cmd.Wait()
}

type logRedirect struct {
	processName string
	level       string
	logFunc     func(string, ...interface{})
}

func newInfoRedirect(name string) *logRedirect {
	return &logRedirect{
		processName: name,
		level:       "Info",
		logFunc:     klog.V(4).Infof,
	}
}

func newErrRedirect(name string) *logRedirect {
	return &logRedirect{
		processName: name,
		level:       "Error",
		logFunc:     klog.Errorf,
	}
}
func (l *logRedirect) Write(p []byte) (n int, err error) {
	msg := fmt.Sprintf("%s[%s]: %s", l.processName, l.level, string(p))
	l.logFunc("%s", msg)
	return len(msg), nil
}
