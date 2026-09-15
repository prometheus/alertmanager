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

	"github.com/alecthomas/kingpin/v2"

	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/matcher/compat"
)

type alertQueryCmd struct {
	inhibited, silenced, active, unprocessed bool
	receiver                                 string
	matcherGroups                            []string
}

const alertQueryHelp = `View and search through current alerts.

Amtool has a simplified prometheus query syntax, but contains robust support for
bash variable expansions. The non-option section of arguments constructs a list
of "Matcher Groups" that will be used to filter your query. The following
examples will attempt to show this behaviour in action:

amtool alert query alertname=foo node=bar

	This query will match all alerts with the alertname=foo and node=bar label
	value pairs set.

amtool alert query foo node=bar

	If alertname is omitted and the first argument does not contain a '=' or a
	'=~' then it will be assumed to be the value of the alertname pair.

amtool alert query 'alertname=~foo.*'

	As well as direct equality, regex matching is also supported. The '=~' syntax
	(similar to prometheus) is used to represent a regex match. Regex matching
	can be used in combination with a direct match.

Amtool supports several flags for filtering the returned alerts by state
(inhibited, silenced, active, unprocessed). If none of these flags is given,
only active alerts are returned.
`

func configureQueryAlertsCmd(cc *kingpin.CmdClause) {
	var (
		a        = &alertQueryCmd{}
		queryCmd = cc.Command("query", alertQueryHelp).Default()
	)
	queryCmd.Flag("inhibited", "Show inhibited alerts").Short('i').BoolVar(&a.inhibited)
	queryCmd.Flag("silenced", "Show silenced alerts").Short('s').BoolVar(&a.silenced)
	queryCmd.Flag("active", "Show active alerts").Short('a').BoolVar(&a.active)
	queryCmd.Flag("unprocessed", "Show unprocessed alerts").Short('u').BoolVar(&a.unprocessed)
	queryCmd.Flag("receiver", "Show alerts matching receiver (Supports regex syntax)").Short('r').StringVar(&a.receiver)
	queryCmd.Arg("matcher-groups", "Query filter").StringsVar(&a.matcherGroups)
	queryCmd.Action(execWithTimeout(a.queryAlerts))
}

func (a *alertQueryCmd) queryAlerts(ctx context.Context, _ *kingpin.ParseContext) error {
	if len(a.matcherGroups) > 0 {
		// Attempt to parse the first argument. If the parser fails
		// then we likely don't have a (=|=~|!=|!~) so lets assume that
		// the user wants alertname=<arg> and prepend `alertname=` to
		// the front.
		m := a.matcherGroups[0]
		_, err := compat.Matcher(m, "cli")
		if err != nil {
			a.matcherGroups[0] = fmt.Sprintf("alertname=%s", strconv.Quote(m))
		}
	}

	// If no selector was passed, default to showing active alerts.
	if !a.silenced && !a.inhibited && !a.active && !a.unprocessed {
		a.active = true
	}

	alertParams := alert.NewGetAlertsParams().WithContext(ctx).
		WithActive(&a.active).
		WithInhibited(&a.inhibited).
		WithSilenced(&a.silenced).
		WithUnprocessed(&a.unprocessed).
		WithReceiver(&a.receiver).
		WithFilter(a.matcherGroups)

	amclient := NewAlertmanagerClient(alertmanagerURL)

	getOk, err := amclient.Alert.GetAlerts(alertParams)
	if err != nil {
		return err
	}

	formatter, found := format.Formatters[output]
	if !found {
		return errors.New("unknown output formatter")
	}
	return formatter.FormatAlerts(getOk.Payload)
}
