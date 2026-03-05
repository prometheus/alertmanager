// Copyright 2024 Prometheus Team
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

//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigFilesWindows(t *testing.T) {
	appData := t.TempDir()
	programData := t.TempDir()
	t.Setenv("APPDATA", appData)
	t.Setenv("ProgramData", programData)

	files := defaultConfigFiles()

	if len(files) != 2 {
		t.Fatalf("expected 2 config file paths, got %d", len(files))
	}

	// os.UserConfigDir() on Windows returns %AppData%, which we overrode via APPDATA.
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		userConfigDir = appData
	}
	expectedUser := filepath.Join(userConfigDir, "amtool", "config.yml")
	if files[0] != expectedUser {
		t.Errorf("expected user config path %q, got %q", expectedUser, files[0])
	}

	expectedSystem := filepath.Join(programData, "amtool", "config.yml")
	if files[1] != expectedSystem {
		t.Errorf("expected system config path %q, got %q", expectedSystem, files[1])
	}
}
