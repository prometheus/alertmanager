// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigFilesOthers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_DIRS", "")

	files := defaultConfigFiles()

	if len(files) != 2 {
		t.Fatalf("expected 2 config file paths, got %d", len(files))
	}

	// os.UserConfigDir() on Unix returns $XDG_CONFIG_HOME if set, otherwise $HOME/.config.
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		userConfigDir = filepath.Join(home, ".config")
	}
	expectedUser := filepath.Join(userConfigDir, "amtool", "config.yml")
	if files[0] != expectedUser {
		t.Errorf("expected user config path %q, got %q", expectedUser, files[0])
	}

	expectedSystem := "/etc/amtool/config.yml"
	if files[1] != expectedSystem {
		t.Errorf("expected system config path %q, got %q", expectedSystem, files[1])
	}
}

func TestDefaultConfigFilesOthersWithXDGConfigDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_DIRS", "/custom/config:/another/config")

	files := defaultConfigFiles()

	if len(files) != 2 {
		t.Fatalf("expected 2 config file paths, got %d", len(files))
	}

	// os.UserConfigDir() on Unix returns $XDG_CONFIG_HOME if set, otherwise $HOME/.config.
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		userConfigDir = filepath.Join(home, ".config")
	}
	expectedUser := filepath.Join(userConfigDir, "amtool", "config.yml")
	if files[0] != expectedUser {
		t.Errorf("expected user config path %q, got %q", expectedUser, files[0])
	}

	expectedSystem := "/custom/config/amtool/config.yml"
	if files[1] != expectedSystem {
		t.Errorf("expected system config path %q, got %q", expectedSystem, files[1])
	}
}

func TestDefaultConfigFilesOthersWithXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	customConfigHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", customConfigHome)
	t.Setenv("XDG_CONFIG_DIRS", "")

	files := defaultConfigFiles()

	if len(files) != 2 {
		t.Fatalf("expected 2 config file paths, got %d", len(files))
	}

	expectedUser := filepath.Join(customConfigHome, "amtool", "config.yml")
	if files[0] != expectedUser {
		t.Errorf("expected user config path %q, got %q", expectedUser, files[0])
	}

	expectedSystem := "/etc/amtool/config.yml"
	if files[1] != expectedSystem {
		t.Errorf("expected system config path %q, got %q", expectedSystem, files[1])
	}
}
