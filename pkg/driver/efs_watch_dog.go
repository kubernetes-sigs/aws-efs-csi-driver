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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

const (
	efsUtilsConfigFileName     = "efs-utils.conf"
	s3filesUtilsConfigFileName = "s3files-utils.conf"
	efsStateDir                = "/var/run/efs"
)

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
	// efs-utils static files path
	efsUtilsStaticFilesPath string
	// enable debug logging in efs-utils.conf and s3files-utils.conf
	debugLogs bool
	// enable CloudWatch logging for EFS
	efsCloudWatchLogEnabled bool
	// enable CloudWatch logging for S3Files
	s3filesCloudWatchLogEnabled bool
	// enable CloudWatch metrics for S3Files proxy
	s3filesCloudWatchMetricsEnabled bool
	// conf overrides for efs-utils.conf
	efsUtilsConfOverrides []ConfOverride
	// conf overrides for s3files-utils.conf
	s3filesUtilsConfOverrides []ConfOverride
	// stopCh indicates if it should be stopped
	stopCh chan struct{}

	mu sync.Mutex
}

type utilsConfig struct {
	EfsClientSource          string
	Region                   string
	FipsEnabled              string
	PortRangeUpperBound      string
	DebugLogs                bool
	CloudWatchLogEnabled     bool
	CloudWatchMetricsEnabled bool
}

func newExecWatchdog(efsUtilsCfgPath, efsUtilsStaticFilesPath string, debugLogs, efsCloudWatchLogEnabled, s3filesCloudWatchLogEnabled, s3filesCloudWatchMetricsEnabled bool, efsUtilsConfOverrides, s3filesUtilsConfOverrides []ConfOverride, cmd string, arg ...string) Watchdog {
	return &execWatchdog{
		efsUtilsCfgPath:                 efsUtilsCfgPath,
		efsUtilsStaticFilesPath:         efsUtilsStaticFilesPath,
		debugLogs:                       debugLogs,
		efsCloudWatchLogEnabled:         efsCloudWatchLogEnabled,
		s3filesCloudWatchLogEnabled:     s3filesCloudWatchLogEnabled,
		s3filesCloudWatchMetricsEnabled: s3filesCloudWatchMetricsEnabled,
		efsUtilsConfOverrides:           efsUtilsConfOverrides,
		s3filesUtilsConfOverrides:       s3filesUtilsConfOverrides,
		execCmd:                         cmd,
		execArg:                         arg,
		stopCh:                          make(chan struct{}),
	}
}

func (w *execWatchdog) start() error {
	if err := w.setup(GetVersion().EfsClientSource); err != nil {
		return err
	}

	if err := w.removeLibwrapOption(efsStateDir); err != nil {
		klog.Warningf("Error removing libwrap option from state files: %v", err)
	}

	go w.runLoop(w.stopCh)

	return nil
}

func (w *execWatchdog) setup(efsClientSource string) error {
	if err := w.restoreStaticFiles(); err != nil {
		return err
	}

	if err := w.updateConfig(efsClientSource); err != nil {
		return err
	}
	return nil
}

/*
At image build time, static files installed by efs-utils in the config directory, i.e. CAs file, need
to be saved in another place so that the other stateful files created at runtime, i.e. private key for
client certificate, in the same config directory can be persisted to host with a host path volume.
Otherwise creating a host path volume for that directory will clean up everything inside at the first time.
Those static files need to be copied back to the config directory when the driver starts up.
*/
func (w *execWatchdog) restoreStaticFiles() error {
	return copyWithoutOverwriting(w.efsUtilsStaticFilesPath, w.efsUtilsCfgPath)
}

func copyWithoutOverwriting(srcDir, dstDir string) error {
	if _, err := os.Stat(dstDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())

		if _, err := os.Stat(dst); os.IsNotExist(err) {
			klog.Infof("Copying %s since it doesn't exist", dst)
			if err := copyFile(src, dst); err != nil {
				return err
			}
		} else if filepath.Base(src) == "efs-utils.crt" || filepath.Base(src) == efsUtilsConfigFileName || filepath.Base(src) == s3filesUtilsConfigFileName {
			klog.Infof("Copying %s ", dst)
			if err := copyFile(src, dst); err != nil {
				return err
			}
		} else {
			klog.Infof("Skip copying %s since it exists already", dst)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func (w *execWatchdog) updateConfig(efsClientSource string) error {
	// used on Fargate, IMDS queries suffice otherwise
	region := os.Getenv("AWS_DEFAULT_REGION")
	fipsEnabled := os.Getenv("FIPS_ENABLED")
	if fipsEnabled == "" {
		fipsEnabled = "false"
	}
	portRangeUpperBound := os.Getenv("PORT_RANGE_UPPER_BOUND")
	val, err := strconv.Atoi(portRangeUpperBound)
	if err != nil || val < 21049 {
		portRangeUpperBound = "21049"
	}
	cfg := utilsConfig{
		EfsClientSource:      efsClientSource,
		Region:               region,
		FipsEnabled:          fipsEnabled,
		PortRangeUpperBound:  portRangeUpperBound,
		DebugLogs:            w.debugLogs,
		CloudWatchLogEnabled: false,
	}

	efsCfg := cfg
	efsCfg.CloudWatchLogEnabled = w.efsCloudWatchLogEnabled
	if err := w.updateConfigFile(efsUtilsConfigFileName, efsCfg, efsUtilsDriverConf(efsCfg), w.efsUtilsConfOverrides); err != nil {
		return err
	}

	s3filesCfg := cfg
	s3filesCfg.CloudWatchLogEnabled = w.s3filesCloudWatchLogEnabled
	s3filesCfg.CloudWatchMetricsEnabled = w.s3filesCloudWatchMetricsEnabled
	if err := w.updateConfigFile(s3filesUtilsConfigFileName, s3filesCfg, s3filesUtilsDriverConf(s3filesCfg), w.s3filesUtilsConfOverrides); err != nil {
		return err
	}
	return nil
}

// updateConfigFile sets the driver's keys on the conf file efs-utils installed
// and writes the result into the config directory. It reads from the static
// files directory rather than the config directory because that copy is the one
// the image's efs-utils package put there, untouched by any earlier run.
//
// Both conf files must be installed: the image's efs-utils ships efs-utils.conf
// and s3files-utils.conf, so a missing one means a broken image.
func (w *execWatchdog) updateConfigFile(fileName string, cfg utilsConfig, driverConf, overrides []ConfOverride) error {
	installedPath := filepath.Join(w.efsUtilsStaticFilesPath, fileName)
	installed, err := os.ReadFile(installedPath)
	if err != nil {
		return fmt.Errorf("cannot read installed config file %s. Error: %v", installedPath, err)
	}
	return w.writeConfigFile(fileName, string(installed), cfg, driverConf, overrides)
}

// loggingLevel maps the --debug-logs flag onto the efs-utils log level.
func loggingLevel(debugLogs bool) string {
	if debugLogs {
		return "DEBUG"
	}
	return "INFO"
}

// efsUtilsDriverConf returns the efs-utils.conf keys whose value comes from a
// driver flag or the node environment. Every other key keeps the value the
// installed efs-utils shipped, so a new efs-utils config option needs no change
// here.
func efsUtilsDriverConf(cfg utilsConfig) []ConfOverride {
	conf := []ConfOverride{
		{Section: "DEFAULT", Key: "logging_level", Value: loggingLevel(cfg.DebugLogs)},
		{Section: "mount", Key: "stunnel_debug_enabled", Value: strconv.FormatBool(cfg.DebugLogs)},
		{Section: "mount", Key: "fips_mode_enabled", Value: cfg.FipsEnabled},
		{Section: "mount", Key: "port_range_upper_bound", Value: cfg.PortRangeUpperBound},
		{Section: "cloudwatch-log", Key: "enabled", Value: strconv.FormatBool(cfg.CloudWatchLogEnabled)},
	}
	return append(conf, regionConf(cfg.Region)...)
}

// s3filesUtilsDriverConf is efsUtilsDriverConf for s3files-utils.conf. The two
// key sets differ: s3files has no stunnel_debug_enabled, and its proxy log
// level and CloudWatch metrics switch live in [proxy].
func s3filesUtilsDriverConf(cfg utilsConfig) []ConfOverride {
	conf := []ConfOverride{
		{Section: "DEFAULT", Key: "logging_level", Value: loggingLevel(cfg.DebugLogs)},
		{Section: "mount", Key: "fips_mode_enabled", Value: cfg.FipsEnabled},
		{Section: "mount", Key: "port_range_upper_bound", Value: cfg.PortRangeUpperBound},
		{Section: "proxy", Key: "proxy_logging_level", Value: loggingLevel(cfg.DebugLogs)},
		{Section: "proxy", Key: "metrics_enabled", Value: strconv.FormatBool(cfg.CloudWatchMetricsEnabled)},
		{Section: "cloudwatch-log", Key: "enabled", Value: strconv.FormatBool(cfg.CloudWatchLogEnabled)},
	}
	return append(conf, regionConf(cfg.Region)...)
}

// regionConf sets [mount] region only when the driver has a region to set. With
// no region the key stays commented out as upstream ships it, which is what
// makes efs-utils read the region from instance metadata instead.
func regionConf(region string) []ConfOverride {
	if region == "" {
		return nil
	}
	return []ConfOverride{{Section: "mount", Key: "region", Value: region}}
}

// writeConfigFile writes installedConf with the driver's keys set on top of it.
// driverConf goes on first and the operator's --*-conf-overrides last, so an
// operator override of a key the driver also sets wins.
//
// A driver key missing from installedConf is added with a warning rather than
// failing startup, matching the driver's behavior before it read the installed
// conf. Operator overrides stay strict: a typo there is an error.
func (w *execWatchdog) writeConfigFile(fileName, installedConf string, cfg utilsConfig, driverConf, overrides []ConfOverride) error {
	content, added := setDriverConf(installedConf, driverConf)
	for _, c := range added {
		klog.Warningf("installed %s has no [%s] %s, so the driver added it; efs-utils may have renamed or dropped this key",
			fileName, c.Section, c.Key)
	}
	content = setClientInfoSource(content, cfg.EfsClientSource)

	if len(overrides) > 0 {
		var err error
		content, err = applyConfOverrides(content, overrides)
		if err != nil {
			return fmt.Errorf("failed to apply config overrides for %s: %w", fileName, err)
		}
		for _, o := range overrides {
			klog.Infof("Applied config override in %s: [%s] %s = %s", fileName, o.Section, o.Key, o.Value)
		}
	}

	filePath := filepath.Join(w.efsUtilsCfgPath, fileName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("cannot write config file %s. Error: %v", fileName, err)
	}
	return nil
}

// stop kills the underlying process and stops the watchdog
func (w *execWatchdog) stop() {
	close(w.stopCh)

	w.mu.Lock()
	if w.cmd != nil && w.cmd.Process != nil {
		p := w.cmd.Process
		err := p.Kill()
		if err != nil {
			klog.Errorf("Failed to kill process: %s", err)
		}
	}
	w.mu.Unlock()
}

// watchdogRestartDelay spaces out relaunch attempts so a watchdog that exits
// (or fails to start) immediately cannot spin the loop into a fork storm.
var watchdogRestartDelay = 5 * time.Second

// runLoop starts the monitoring loop
func (w *execWatchdog) runLoop(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			klog.V(4).Infof("stopping...")
			return
		default:
			err := w.exec()
			if err != nil {
				klog.Errorf("Process %s exits %s", w.execCmd, err)
			}
			select {
			case <-stopCh:
				klog.V(4).Infof("stopping...")
				return
			case <-time.After(watchdogRestartDelay):
			}
		}
	}
}

func (w *execWatchdog) exec() error {
	cmd := exec.Command(w.execCmd, w.execArg...)
	cmd.Stdout = newInfoRedirect(w.execCmd)
	cmd.Stderr = newErrRedirect(w.execCmd)

	w.mu.Lock()
	err := cmd.Start()
	if err != nil {
		w.mu.Unlock()
		return err
	}
	w.cmd = cmd
	// Only cmd.Wait() below may collect this child; keep the reaper off it.
	ownChild(cmd.Process.Pid)
	w.mu.Unlock()

	defer disownChild(cmd.Process.Pid)
	return cmd.Wait()
}

/*
If upgrading directly from older 1.x version of driver without application pod deployment,
users may have existing mounts using stunnel instead of efs-proxy.
If their stunnel state files have explicitly set the libwrap option as no,
the newer stunnel binaries will fail to run as this option is no longer supported in al23.
To avoid any errors, we check for this config and remove it directly on startup.
*/
func (w *execWatchdog) removeLibwrapOption(stateDir string) error {
	stunnelFiles, err := os.ReadDir(stateDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// The state directory is only mounted into the node DaemonSet.
			// Without it there are no stunnel state files to fix, so there
			// is nothing to do (e.g. in the controller Deployment).
			klog.V(4).Infof("State directory %s does not exist, skipping libwrap cleanup", stateDir)
			return nil
		}
		return fmt.Errorf("error reading directory %s: %v", stateDir, err)
	}

	for _, file := range stunnelFiles {
		if strings.HasPrefix(file.Name(), "stunnel-config.") {
			filePath := filepath.Join(stateDir, file.Name())
			if err := removeLibwrapFromFile(filePath); err != nil {
				klog.Warningf("Error processing stunnel file %s: %v", filePath, err)
			}
		}
	}
	return nil
}

func removeLibwrapFromFile(configFile string) error {
	stunnelConfigData, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read stunnel config: %v", err)
	}

	newStunnelConfigData := strings.Replace(string(stunnelConfigData), "libwrap = no", "", -1)

	err = os.WriteFile(configFile, []byte(newStunnelConfigData), 0644)
	if err != nil {
		return fmt.Errorf("failed to update stunnel config: %v", err)
	}

	return nil
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
