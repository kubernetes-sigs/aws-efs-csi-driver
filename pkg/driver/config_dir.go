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
	"k8s.io/klog"
	"os"
	"path"
)

// InitConfigDir decides which of two mounted directories will be used to store driver config files. It creates a
// symlink at etcAmazonEfs to the chosen location (legacyDir or preferredDir). If neither of these locations is present
// (i.e. when the user does not need to durably store configs and thus does not mount host directories), an empty
// directory will be created at etcAmazonEfs.
//
// - legacyDir    is the path to a config directory where previous versions of this driver may have written config
//                files. In previous versions of this driver, a host path that was not writeable on Bottlerocket hosts
//                was being used, so we introduce preferredDir.
//
// - preferredDir is the path to config directory that we will use so long as we do not find files in legacyDir.
//
// - etcAmazonEfs is the path where the symlink will be written. In practice, this will always be /etc/amazon/efs, but
//                we take it as an input so the function can be tested.
// Examples:
// On a host that has EFS mounts created by an earlier version of this driver, InitConfigDir will detect a conf file in
// legacyDir and write a symlink at etcAmazonEfs pointing to legacyDir.
//
// On a host that does not have pre-existing legacy EFS mount configs, InitConfigDir will detect no files in  legacyDir
// and will create a symlink at etcAmazonEfs pointing to preferredDir.
//
// For a use case in which durable storage of the configs is not required, and no host volumes are mounted, the function
// creates an empty directory at etcAmazonEfs.
//
// If a symlink already existing at etcAmazonEfs, InitConfigDir does nothing. If something other than a symlink exists
// at etcAmazonEfs, InitConfigDir returns an error.
func InitConfigDir(legacyDir, preferredDir, etcAmazonEfs string) error {

	// if there is already a symlink or directory in place, we have nothing to do
	if _, err := os.Stat(etcAmazonEfs); err == nil {
		klog.Infof("Symlink or directory exists at '%s', no need to create one", etcAmazonEfs)
		return nil
	}

	// if neither legacyDir nor preferredDir exist, create an empty directory.
	if _, err := os.Stat(preferredDir); err != nil {
		if _, err := os.Stat(legacyDir); err != nil {
			klog.Infof("Mounted directories do not exist, creating directory at '%s'", etcAmazonEfs)
			if err := os.MkdirAll(etcAmazonEfs, 0755); err != nil {
				return fmt.Errorf(
					"unable to create directory at '%s': %s",
					etcAmazonEfs,
					err.Error())
			}
			return nil
		}
	}

	// check if a conf file exists in the legacy directory and symlink to the directory if so
	existingConfFile := path.Join(legacyDir, "efs-utils.conf")
	if _, err := os.Stat(existingConfFile); err == nil {
		if err = os.Symlink(legacyDir, etcAmazonEfs); err != nil {
			return fmt.Errorf(
				"unable to create symlink from '%s' to '%s': %s",
				etcAmazonEfs,
				legacyDir,
				err.Error())
		}
		klog.Infof("Pre-existing config files are being used from '%s'", legacyDir)
		return nil
	}

	klog.Infof("Creating symlink from '%s' to '%s'", etcAmazonEfs, preferredDir)

	// create a symlink to the config directory
	if err := os.Symlink(preferredDir, etcAmazonEfs); err != nil {
		return fmt.Errorf(
			"unable to create symlink from '%s' to '%s': %s",
			etcAmazonEfs,
			preferredDir,
			err.Error())
	}

	return nil
}
