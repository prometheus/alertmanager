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
	"context"
	"fmt"
	"net/url"

	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
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
	testCmd.Arg("config.file", "Config file to be tested.").ExistingFileVar(&t.configFile)
	testCmd.Action(execWithTimeout(t.testReceivers))
}

func (t *testReceiversCmd) testReceivers(ctx context.Context, _ *kingpin.ParseContext) error {
	if len(t.configFile) == 0 {
		kingpin.Fatalf("No config file was specified")
	}

	fmt.Printf("Checking '%s'\n", t.configFile)
	cfg, err := config.LoadFile(t.configFile)
	if err != nil {
		kingpin.Fatalf("Invalid config file")
	}

	if cfg != nil {
		tmpl, err := template.FromGlobs(cfg.Templates...)
		if err != nil {
			return errors.Wrap(err, "failed to parse templates")
		}
		if alertmanagerURL != nil {
			tmpl.ExternalURL = alertmanagerURL
		} else {
			u, err := url.Parse("http://localhost:1234")
			if err != nil {
				return errors.Wrap(err, "failed to parse mock url")
			}
			tmpl.ExternalURL = u
		}

		fmt.Printf("Testing %d receivers...\n", len(cfg.Receivers))
		result, err := TestReceivers(ctx, cfg.Receivers, tmpl)
		if err != nil {
			return err
		}
		printTestReceiversResults(result)
	}

	return nil
}

func printTestReceiversResults(result *TestReceiversResult) {
	successful := 0
	successfulCounts := make(map[string]int)
	for _, rcv := range result.Receivers {
		successfulCounts[rcv.Name] = 0
		for _, cfg := range rcv.ConfigResults {
			if cfg.Error == nil {
				successful += 1
				successfulCounts[rcv.Name] += 1
			}
		}
	}

	fmt.Printf("\nSuccessfully notified %d/%d receivers at %v:\n", successful, len(result.Receivers), result.NotifedAt.Format("2006-01-02 15:04:05"))

	for _, rcv := range result.Receivers {
		fmt.Printf("   %d/%d - '%s'\n", successfulCounts[rcv.Name], len(rcv.ConfigResults), rcv.Name)
		for _, cfg := range rcv.ConfigResults {
			if cfg.Error != nil {
				fmt.Printf("     - %s - %s: %s\n", cfg.Name, cfg.Status, cfg.Error.Error())
			} else {
				fmt.Printf("     - %s - %s\n", cfg.Name, cfg.Status)
			}
		}
	}
}
