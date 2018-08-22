// Copyright 2018 Prometheus Team
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
	"errors"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/cli/format"
)

const configHelp = `View current config.

The amount of output is controlled by the output selection flag:
	- Simple: Print just the running config
	- Extended: Print the running config as well as uptime and all version info
	- Json: Print entire config object as json
`

// configCmd represents the config command
func configureConfigCmd(app *kingpin.Application) {
	configCmd := app.Command("config", configHelp)
	configCmd.Command("show", configHelp).Default().Action(execWithTimeout(queryConfig)).PreAction(requireAlertManagerURL)
	configureRoutingCmd(configCmd)
}

func queryConfig(ctx context.Context, _ *kingpin.ParseContext) error {
	status, err := getRemoteAlertmanagerConfigStatus(ctx, alertmanagerURL)
	if err != nil {
		return err
	}

	formatter, found := format.Formatters[output]
	if !found {
		return errors.New("unknown output formatter")
	}

	return formatter.FormatConfig(status)
}
