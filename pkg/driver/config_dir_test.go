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
	"path"
	"path/filepath"
	"testing"
)

func tempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("error creating directory %v", err)
	}
	return dir
}

const (
	// the directory names we will use in these tests (any valid names will work)

	// the path to the legacy mounted directory
	legacy = "etc-amazon-efs-legacy"

	// the path to the preferred mounted directory
	preferred = "var-amazon-efs"

	// the path to the canonical path used by mount.efs
	canonical = "etc"

	// this is a fixed name of a config file used by mount.efs
	configFilename = "efs-utils.conf"

	// these improve readability
	createConfig      = true
	doNotCreateConfig = false
)

// create creates a directory under tempDir named dirName and optionally adds a file to it, returning the paths
func create(t *testing.T, tempDir, dirName string, createConfigFile bool) (dirPath, configFilepath string) {
	dirPath = filepath.Join(tempDir, dirName)
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		t.Fatalf("Unable to create a directory: %v", err)
	}
	if createConfigFile {
		configFilepath = filepath.Join(dirPath, configFilename)
		if f, err := os.Create(configFilepath); err != nil {
			t.Fatalf("Unable to create a file: %v", err)
		} else {
			if err := f.Close(); err != nil {
				t.Fatalf("Unable to close a file: %v", err)
			}
		}
	}
	return dirPath, configFilepath
}

// assertSymlink asserts that a symlink exists
func assertSymlink(t *testing.T, from, to string) {
	// assert symlink is there
	if _, err := os.Lstat(from); err != nil {
		t.Fatalf("symlink was not created at %s", from)
	}

	// create some file using the symlink to be absolutely certain the symlink is correct
	symlinkFile := path.Join(from, "foo.txt")
	expectedLocation := path.Join(to, "foo.txt")

	if f, err := os.Create(symlinkFile); err != nil {
		t.Fatalf("Unable to create file using symlink: %v", err)
	} else {
		if err := f.Close(); err != nil {
			t.Fatalf("Unable to close a file: %v", err)
		}
	}

	// assert that the file was created in the legacy dir
	if _, err := os.Stat(expectedLocation); err != nil {
		t.Errorf("Something is wrong with the symlink at '%s' which should point to '%s'", from, to)
	}
}

func cleanup(t *testing.T, tempDir string) {
	if err := os.RemoveAll(tempDir); err != nil {
		t.Fatalf("Unable to delete temp dir: %v", err)
	}
}

// TestInitConfigDirPreExistingConfig asserts that a symlink is created to the legacy directory if efs-utils.conf is
// found there.
func TestInitConfigDirPreExistingConfig(t *testing.T) {
	dir := tempDir(t)
	defer cleanup(t, dir)

	// create legacy dir and a fake pre-existing conf file
	legacyDir, _ := create(t, dir, legacy, createConfig)

	// create the preferred dir which will go unused
	preferredDir, _ := create(t, dir, preferred, doNotCreateConfig)

	// the path where a symlink is expected
	etcAmazonEfs := filepath.Join(dir, canonical)

	// function under test
	if err := InitConfigDir(legacyDir, preferredDir, etcAmazonEfs); err != nil {
		t.Fatalf("InitConfigDir returned an error: %v", err)
	}

	// assert that a symlink exists: etcAmazonEfs -> legacyDir
	assertSymlink(t, etcAmazonEfs, legacyDir)
}

// TestInitConfigDirPreferred asserts that a symlink is created to the preferred directory if efs-utils.conf is
// not found in the legacy location.
func TestInitConfigDirPreferred(t *testing.T) {
	dir := tempDir(t)
	defer cleanup(t, dir)

	// create an empty legacy dir
	legacyDir, _ := create(t, dir, legacy, doNotCreateConfig)

	// create the preferred dir (symlink should be created with or without pre-existing config here)
	preferredDir, _ := create(t, dir, preferred, doNotCreateConfig)

	// the path where a symlink is expected
	etcAmazonEfs := filepath.Join(dir, canonical)

	// function under test
	if err := InitConfigDir(legacyDir, preferredDir, etcAmazonEfs); err != nil {
		t.Fatalf("InitConfigDir returned an error: %v", err)
	}

	// assert that a symlink exists: etcAmazonEfs -> preferredDir
	assertSymlink(t, etcAmazonEfs, preferredDir)
}

// TestInitConfigDirDoNothing asserts that a pre-existing symlink is not altered.
func TestInitConfigDirDoNothing(t *testing.T) {
	dir := tempDir(t)
	defer cleanup(t, dir)

	// create an empty legacy dir
	legacyDir, _ := create(t, dir, legacy, doNotCreateConfig)

	// create the preferred dir
	preferredDir, _ := create(t, dir, preferred, doNotCreateConfig)

	// the path where a symlink is expected
	etcAmazonEfs := filepath.Join(dir, canonical)

	// create a symlink
	if err := InitConfigDir(legacyDir, preferredDir, etcAmazonEfs); err != nil {
		t.Fatalf("InitConfigDir returned an error: %v", err)
	}

	// run the function again, as if the container has been started a second time
	if err := InitConfigDir(legacyDir, preferredDir, etcAmazonEfs); err != nil {
		t.Fatalf("InitConfigDir returned an error: %v", err)
	}

	// assert that a symlink exists: etcAmazonEfs -> preferredDir
	assertSymlink(t, etcAmazonEfs, preferredDir)
}

// TestInitConfigDirNoMounts asserts that a directory is created at etcAmazonEfs if legacyDir and preferredDir are not
// mounted.
func TestInitConfigDirNoMounts(t *testing.T) {
	dir := tempDir(t)
	defer cleanup(t, dir)

	missingLegacyDir := filepath.Join(dir, legacy)
	missingPreferredDir := filepath.Join(dir, legacy)
	etcAmazonEfs := filepath.Join(dir, canonical)

	// function under test
	if err := InitConfigDir(missingLegacyDir, missingPreferredDir, etcAmazonEfs); err != nil {
		t.Fatalf("InitConfigDir returned an error: %v", err)
	}

	// check that a directory was created
	if stat, err := os.Stat(etcAmazonEfs); err != nil {
		t.Errorf("etcAmazonEfs dir was not created: %v", err)
	} else if !stat.IsDir() {
		t.Errorf("Something other than directory was created")
	}
}

// TestInitConfigDirPreferredError asserts that an error is returned when a symlink cannot be created to the
// preferredDir
func TestInitConfigDirPreferredError(t *testing.T) {
	dir := tempDir(t)
	defer cleanup(t, dir)

	// create an empty legacy dir
	legacyDir, _ := create(t, dir, legacy, doNotCreateConfig)

	// create the preferred dir
	preferredDir, _ := create(t, dir, preferred, doNotCreateConfig)

	etcAmazonEfs := filepath.Join(dir, "bad", "path")

	// function under test
	if err := InitConfigDir(legacyDir, preferredDir, etcAmazonEfs); err == nil {
		t.Errorf("Expected an error when calling InitConfigDir")
	}
}

// TestInitConfigDirPreExistingConfigError asserts that an error is returned if a symlink cannot be created to the
// legacyDir.
func TestInitConfigDirPreExistingConfigError(t *testing.T) {
	dir := tempDir(t)
	defer cleanup(t, dir)

	// create legacy dir and a fake pre-existing conf file
	legacyDir, _ := create(t, dir, legacy, createConfig)

	// create the preferred dir which will go unused
	preferredDir, _ := create(t, dir, preferred, doNotCreateConfig)

	etcAmazonEfs := filepath.Join(dir, "bad", "path")

	// function under test
	if err := InitConfigDir(legacyDir, preferredDir, etcAmazonEfs); err == nil {
		t.Errorf("Expected an error when calling InitConfigDir")
	}
}
