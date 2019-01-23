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

	"fmt"
	"time"

	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/client_golang/api"
	"gopkg.in/alecthomas/kingpin.v2"
)

type alertAddCmd struct {
	annotations  []string
	generatorUrl string
	labels       []string
	start        string
	end          string
}

const alertAddHelp = `Add a new alert.

This command is used to add a new alert to Alertmanager.

To add a new alert with labels:

	amtool alert add alertname=foo node=bar

If alertname is omitted and the first argument does not contain a '=' then it will
be assumed to be the value of the alertname pair.

	amtool alert add foo node=bar

One or more annotations can be added using the --annotation flag:

	amtool alert add foo node=bar \
		--annotation=runbook='http://runbook.biz' \
		--annotation=summary='summary of the alert' \
		--annotation=description='description of the alert'

Additional flags such as --generator-url, --start, and --end are also supported.
`

func configureAddAlertCmd(cc *kingpin.CmdClause) {
	var (
		a      = &alertAddCmd{}
		addCmd = cc.Command("add", alertAddHelp)
	)
	addCmd.Arg("labels", "List of labels to be included with the alert").StringsVar(&a.labels)
	addCmd.Flag("generator-url", "Set the URL of the source that generated the alert").StringVar(&a.generatorUrl)
	addCmd.Flag("start", "Set when the alert should start. RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&a.start)
	addCmd.Flag("end", "Set when the alert should should end. RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&a.end)
	addCmd.Flag("annotation", "Set an annotation to be included with the alert").StringsVar(&a.annotations)
	addCmd.Action(execWithTimeout(a.addAlert))
}

func (a *alertAddCmd) addAlert(ctx context.Context, _ *kingpin.ParseContext) error {
	c, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return err
	}
	alertAPI := client.NewAlertAPI(c)

	if len(a.labels) > 0 {
		// Allow the alertname label to be defined implicitly as the first argument rather
		// than explicitly as a key=value pair.
		if _, err := parseLabels([]string{a.labels[0]}); err != nil {
			a.labels[0] = fmt.Sprintf("alertname=%s", a.labels[0])
		}
	}

	labels, err := parseLabels(a.labels)
	if err != nil {
		return err
	}

	annotations, err := parseLabels(a.annotations)
	if err != nil {
		return err
	}

	var startsAt, endsAt time.Time
	if a.start != "" {
		startsAt, err = time.Parse(time.RFC3339, a.start)
		if err != nil {
			return err
		}
	}
	if a.end != "" {
		endsAt, err = time.Parse(time.RFC3339, a.end)
		if err != nil {
			return err
		}
	}

	return alertAPI.Push(ctx, client.Alert{
		Labels:       labels,
		Annotations:  annotations,
		StartsAt:     startsAt,
		EndsAt:       endsAt,
		GeneratorURL: a.generatorUrl,
	})
}
