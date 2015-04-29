// Copyright 2013 Prometheus Team
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
	"io/ioutil"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"

	pb "github.com/prometheus/alertmanager/config/generated"
)

func LoadFromString(configStr string) (Config, error) {
	configProto := pb.AlertManagerConfig{}
	if err := proto.UnmarshalText(configStr, &configProto); err != nil {
		return Config{}, err
	}

	config := Config{AlertManagerConfig: configProto}
	err := config.Validate()

	return config, err
}

func LoadFromFile(fileName string) (Config, error) {
	configStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return Config{}, err
	}

	return LoadFromString(string(configStr))
}

func MustLoadFromFile(fileName string) Config {
	conf, err := LoadFromFile(fileName)
	if err != nil {
		glog.Fatalf("Error loading configuration from %s: %s", fileName, err)
	}
	return conf
}
