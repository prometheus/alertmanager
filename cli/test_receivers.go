// Copyright 2022 Prometheus Team
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

package cli

import (
	"fmt"

	"github.com/prometheus/alertmanager/config"
	"gopkg.in/alecthomas/kingpin.v2"
)

type testReceiversCmd struct {
	configFile string
}

const testReceiversHelp = `Test alertmanager receivers

Will test receivers for alertmanager config file.
`

func configureTestReceiversCmd(app *kingpin.Application) {
	var (
		t       = &testReceiversCmd{}
		testCmd = app.Command("test-receivers", testReceiversHelp)
	)
	testCmd.Arg("config-file", "Config file to be validated").ExistingFileVar(&t.configFile)
	testCmd.Action(t.testReceivers)
}

func (t *testReceiversCmd) testReceivers(ctx *kingpin.ParseContext) error {
	if len(t.configFile) == 0 {
		kingpin.Fatalf("No config file was specified")
	}

	fmt.Printf("Checking '%s'\n", t.configFile)
	cfg, err := config.LoadFile(t.configFile)
	if err != nil {
		kingpin.Fatalf("Invalid config file")
	}

	//successful := 0
	if cfg != nil {
		for _, receiver := range cfg.Receivers {
			fmt.Printf("Testing receiver '%s'\n", receiver.Name)
		}
	}

	return nil
}
