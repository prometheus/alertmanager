package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/client_golang/api"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/pkg/parse"
)

var (
	alertCmd      = app.Command("alert", "View and search through current alerts")
	alertQueryCmd = alertCmd.Command("query", "View and search through current alerts").Default()
	expired       = alertQueryCmd.Flag("expired", "Show expired alerts as well as active").Bool()
	silenced      = alertQueryCmd.Flag("silenced", "Show silenced alerts").Short('s').Bool()
	alertQuery    = alertQueryCmd.Arg("matcher-groups", "Query filter").Strings()
)

func init() {
	alertQueryCmd.Action(queryAlerts)
	longHelpText["alert"] = `View and search through current alerts.

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
	can be used in combination with a direct match.`
	longHelpText["alert query"] = longHelpText["alert"]
}

func queryAlerts(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	var filterString = ""
	if len(*alertQuery) == 1 {
		// If the parser fails then we likely don't have a (=|=~|!=|!~) so lets
		// assume that the user wants alertname=<arg> and prepend `alertname=`
		// to the front.
		_, err := parse.Matcher((*alertQuery)[0])
		if err != nil {
			filterString = fmt.Sprintf("{alertname=%s}", (*alertQuery)[0])
		} else {
			filterString = fmt.Sprintf("{%s}", strings.Join(*alertQuery, ","))
		}
	} else if len(*alertQuery) > 1 {
		filterString = fmt.Sprintf("{%s}", strings.Join(*alertQuery, ","))
	}

	c, err := api.NewClient(api.Config{Address: (*alertmanagerUrl).String()})
	if err != nil {
		return err
	}
	alertAPI := client.NewAlertAPI(c)
	fetchedAlerts, err := alertAPI.List(context.Background(), filterString, *expired, *silenced)
	if err != nil {
		return err
	}

	formatter, found := format.Formatters[*output]
	if !found {
		return errors.New("unknown output formatter")
	}
	return formatter.FormatAlerts(fetchedAlerts)
}
