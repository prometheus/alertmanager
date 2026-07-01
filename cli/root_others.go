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
)

func defaultConfigFiles() []string {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		userConfigDir = filepath.Join(os.ExpandEnv("$HOME"), ".config")
	}

	systemConfigDir := "/etc"
	if envConfigDir := os.Getenv("XDG_CONFIG_DIRS"); envConfigDir != "" {
		systemConfigDir = filepath.SplitList(envConfigDir)[0]
	}

	return []string{
		filepath.Join(userConfigDir, "amtool", "config.yml"),
		filepath.Join(systemConfigDir, "amtool", "config.yml"),
	}
}
