// Copyright 2021 Prometheus Team
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

package config

import (
	"os"
	"path/filepath"

	promconfig "github.com/prometheus/common/config"
	"gopkg.in/yaml.v2"
)

// LoadHTTPConfigFile returns HTTPClientConfig for the given http_config file.
func LoadHTTPConfigFile(filename string) (*promconfig.HTTPClientConfig, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	httpConfig := &promconfig.HTTPClientConfig{}
	err = yaml.UnmarshalStrict(b, httpConfig)
	if err != nil {
		return nil, err
	}
	httpConfig.SetDirectory(filepath.Dir(filepath.Dir(filename)))

	return httpConfig, nil
}
