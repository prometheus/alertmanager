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
	"fmt"
	"strconv"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-openapi/strfmt"

	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/matcher/compat"
	"github.com/prometheus/alertmanager/pkg/labels"
)

type alertAddCmd struct {
	annotations  []string
	generatorURL string
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
	addCmd.Flag("generator-url", "Set the URL of the source that generated the alert").StringVar(&a.generatorURL)
	addCmd.Flag("start", "Set when the alert should start. RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&a.start)
	addCmd.Flag("end", "Set when the alert should end. RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&a.end)
	addCmd.Flag("annotation", "Set an annotation to be included with the alert").StringsVar(&a.annotations)
	addCmd.Action(execWithTimeout(a.addAlert))
}

func (a *alertAddCmd) addAlert(ctx context.Context, _ *kingpin.ParseContext) error {
	if len(a.labels) > 0 {
		// Allow the alertname label to be defined implicitly as the first argument rather
		// than explicitly as a key=value pair.
		if _, err := compat.Matcher(a.labels[0], "cli"); err != nil {
			a.labels[0] = fmt.Sprintf("alertname=%s", strconv.Quote(a.labels[0]))
		}
	}

	ls := make(models.LabelSet, len(a.labels))
	for _, l := range a.labels {
		matcher, err := compat.Matcher(l, "cli")
		if err != nil {
			return err
		}
		if matcher.Type != labels.MatchEqual {
			return errors.New("labels must be specified as key=value pairs")
		}
		ls[matcher.Name] = matcher.Value
	}

	annotations := make(models.LabelSet, len(a.annotations))
	for _, a := range a.annotations {
		matcher, err := compat.Matcher(a, "cli")
		if err != nil {
			return err
		}
		if matcher.Type != labels.MatchEqual {
			return errors.New("annotations must be specified as key=value pairs")
		}
		annotations[matcher.Name] = matcher.Value
	}

	var startsAt, endsAt time.Time
	if a.start != "" {
		var err error
		startsAt, err = time.Parse(time.RFC3339, a.start)
		if err != nil {
			return err
		}
	}
	if a.end != "" {
		var err error
		endsAt, err = time.Parse(time.RFC3339, a.end)
		if err != nil {
			return err
		}
	}

	pa := &models.PostableAlert{
		Alert: models.Alert{
			GeneratorURL: strfmt.URI(a.generatorURL),
			Labels:       ls,
		},
		Annotations: annotations,
		StartsAt:    strfmt.DateTime(startsAt),
		EndsAt:      strfmt.DateTime(endsAt),
	}
	alertParams := alert.NewPostAlertsParams().WithContext(ctx).
		WithAlerts(models.PostableAlerts{pa})

	amclient := NewAlertmanagerClient(alertmanagerURL)

	_, err := amclient.Alert.PostAlerts(alertParams)
	return err
}
