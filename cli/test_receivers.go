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
	"os"

	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

type testReceiversCmd struct {
	configFile string
	alertFile  string
}

const testReceiversHelp = `Test alertmanager receivers

Send test notifications to every receiver for an alertmanager config file.
`

func configureTestReceiversCmd(app *kingpin.Application) {
	var (
		t       = &testReceiversCmd{}
		testCmd = app.Command("test-receivers", testReceiversHelp)
	)
	testCmd.Arg("config.file", "Config file to be tested.").ExistingFileVar(&t.configFile)
	testCmd.Flag("alert.file", "Mock alert file with annotations and labels to add to test alert.").ExistingFileVar(&t.alertFile)
	testCmd.Action(execWithTimeout(t.testReceivers))
}

func (t *testReceiversCmd) testReceivers(ctx context.Context, _ *kingpin.ParseContext) error {
	if len(t.configFile) == 0 {
		kingpin.Fatalf("No config file was specified")
	}

	fmt.Printf("Checking alertmanager config '%s'...\n", t.configFile)
	cfg, err := config.LoadFile(t.configFile)
	if err != nil {
		kingpin.Fatalf("Invalid alertmanager config file")
	}

	if cfg != nil {
		tmpl, err := getTemplate(cfg)
		if err != nil {
			return err
		}

		c := TestReceiversParams{
			Receivers: cfg.Receivers,
		}

		if t.alertFile != "" {
			alert, err := loadAlertConfigFile(t.alertFile)
			if err != nil {
				kingpin.Fatalf("Invalid alert config file")
			}
			c.Alert = alert
		}

		fmt.Printf("Testing %d receivers...\n", len(cfg.Receivers))
		result, err := TestReceivers(ctx, c, tmpl)
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

func getTemplate(cfg *config.Config) (*template.Template, error) {
	tmpl, err := template.FromGlobs(cfg.Templates...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse templates")
	}
	if alertmanagerURL != nil {
		tmpl.ExternalURL = alertmanagerURL
	} else {
		u, err := url.Parse("http://localhost:1234")
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse mock url")
		}
		tmpl.ExternalURL = u
	}
	return tmpl, nil
}

func loadAlertConfigFile(filename string) (*TestReceiversAlertParams, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	alert := &TestReceiversAlertParams{}
	err = yaml.UnmarshalStrict(b, alert)
	if err != nil {
		return nil, err
	}

	return alert, nil
}
