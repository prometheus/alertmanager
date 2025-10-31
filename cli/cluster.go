// Copyright 2020 Prometheus Team
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

	"github.com/alecthomas/kingpin/v2"

	"github.com/prometheus/alertmanager/cli/format"
)

const clusterHelp = `View cluster status and peers.`

// configureClusterCmd represents the cluster command.
func configureClusterCmd(app *kingpin.Application) {
	clusterCmd := app.Command("cluster", clusterHelp)
	clusterCmd.Command("show", clusterHelp).Default().Action(execWithTimeout(showStatus)).PreAction(requireAlertManagerURL)
}

func showStatus(ctx context.Context, _ *kingpin.ParseContext) error {
	alertManagerStatus, err := getRemoteAlertmanagerConfigStatus(ctx, alertmanagerURL)
	if err != nil {
		return err
	}
	formatter, found := format.Formatters[output]
	if !found {
		return errors.New("unknown output formatter")
	}
	return formatter.FormatClusterStatus(alertManagerStatus.Cluster)
}
