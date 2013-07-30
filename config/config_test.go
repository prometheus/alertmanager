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
	"path"
	"strings"
	"testing"
)

var fixturesPath = "fixtures"

type configTest struct {
	inputFile   string
	shouldFail  bool
	errContains string
}

func (ct *configTest) test(i int, t *testing.T) {
	_, err := LoadFromFile(path.Join(fixturesPath, ct.inputFile))

	if err != nil {
		if !ct.shouldFail {
			t.Fatalf("%d. Error parsing config %v: %v", i, ct.inputFile, err)
		} else {
			if !strings.Contains(err.Error(), ct.errContains) {
				t.Fatalf("%d. Expected error containing '%v', got: %v", i, ct.errContains, err)
			}
		}
	}
}

func TestConfigs(t *testing.T) {
	var configTests = []configTest{
		{
			inputFile: "empty.conf.input",
		}, {
			inputFile: "sample.conf.input",
		}, {
			inputFile:   "missing_filter_name_re.conf.input",
			shouldFail:  true,
			errContains: "Missing name pattern",
		}, {
			inputFile:   "invalid_proto_format.conf.input",
			shouldFail:  true,
			errContains: "unknown field name",
		}, {
			inputFile:   "duplicate_nc_name.conf.input",
			shouldFail:  true,
			errContains: "not unique",
		}, {
			inputFile:   "nonexistent_nc_name.conf.input",
			shouldFail:  true,
			errContains: "No such notification config",
		}, {
			inputFile:   "missing_nc_name.conf.input",
			shouldFail:  true,
			errContains: "Missing name",
		},
	}

	for i, ct := range configTests {
		ct.test(i, t)
	}
}
