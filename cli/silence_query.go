package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/pkg/parse"
	"github.com/prometheus/alertmanager/types"
)

var (
	queryCmd     = silenceCmd.Command("query", "Query Alertmanager silences.").Default()
	queryExpired = queryCmd.Flag("expired", "Show expired silences instead of active").Bool()
	silenceQuery = queryCmd.Arg("matcher-groups", "Query filter").Strings()
	queryWithin  = queryCmd.Flag("within", "Show silences that will expire or have expired within a duration").Duration()
)

func init() {
	queryCmd.Action(query)
	longHelpText["silence query"] = `Query Alertmanager silences.

Amtool has a simplified prometheus query syntax, but contains robust support for
bash variable expansions. The non-option section of arguments constructs a list
of "Matcher Groups" that will be used to filter your query. The following
examples will attempt to show this behaviour in action:

amtool silence query alertname=foo node=bar

	This query will match all silences with the alertname=foo and node=bar label
	value pairs set.

amtool silence query foo node=bar

	If alertname is omitted and the first argument does not contain a '=' or a
	'=~' then it will be assumed to be the value of the alertname pair.

amtool silence query 'alertname=~foo.*'

	As well as direct equality, regex matching is also supported. The '=~' syntax
	(similar to prometheus) is used to represent a regex match. Regex matching
	can be used in combination with a direct match.

In addition to filtering by silence labels, one can also query for silences
that are due to expire soon with the "--within" parameter. In the event that
you want to preemptively act upon expiring silences by either fixing them or
extending them. For example:

amtool silence query --within 8h

returns all the silences due to expire within the next 8 hours. This syntax can
also be combined with the label based filtering above for more flexibility.

The "--expired" parameter returns only expired silences. Used in combination
with "--within=TIME", amtool returns the silences that expired within the
preceding duration.

amtool silence query --within 2h --expired

returns all silences that expired within the preceeding 2 hours.`
}

func fetchSilences(filter string) ([]types.Silence, error) {
	silenceResponse := alertmanagerSilenceResponse{}

	u := GetAlertmanagerURL("/api/v1/silences")
	u.RawQuery = "filter=" + url.QueryEscape(filter)

	res, err := http.Get(u.String())
	if err != nil {
		return []types.Silence{}, err
	}

	defer res.Body.Close()

	err = json.NewDecoder(res.Body).Decode(&silenceResponse)
	if err != nil {
		return []types.Silence{}, err
	}

	if silenceResponse.Status != "success" {
		return []types.Silence{}, fmt.Errorf("[%s] %s", silenceResponse.ErrorType, silenceResponse.Error)
	}

	return silenceResponse.Data, nil
}

func query(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	var filterString = ""
	if len(*silenceQuery) == 1 {
		// If we only have one argument then it's possible that the user wants me to assume alertname=<arg>
		// Attempt to use the parser to pare the argument
		// If the parser fails then we likely don't have a (=|=~|!=|!~) so lets prepend `alertname=` to the front
		_, err := parse.Matcher((*silenceQuery)[0])
		if err != nil {
			filterString = fmt.Sprintf("{alertname=%s}", (*silenceQuery)[0])
		} else {
			filterString = fmt.Sprintf("{%s}", strings.Join(*silenceQuery, ","))
		}
	} else if len(*silenceQuery) > 1 {
		filterString = fmt.Sprintf("{%s}", strings.Join(*silenceQuery, ","))
	}

	fetchedSilences, err := fetchSilences(filterString)
	if err != nil {
		return err
	}

	displaySilences := []types.Silence{}
	for _, silence := range fetchedSilences {
		// skip expired silences if --expired is not set
		if !*queryExpired && silence.EndsAt.Before(time.Now()) {
			continue
		}
		// skip active silences if --expired is set
		if *queryExpired && silence.EndsAt.After(time.Now()) {
			continue
		}
		// skip active silences expiring after "--within"
		if !*queryExpired && int64(*queryWithin) > 0 && silence.EndsAt.After(time.Now().UTC().Add(*queryWithin)) {
			continue
		}
		// skip silences that expired before "--within"
		if *queryExpired && int64(*queryWithin) > 0 && silence.EndsAt.Before(time.Now().UTC().Add(-*queryWithin)) {
			continue
		}

		displaySilences = append(displaySilences, silence)
	}

	if *silenceQuiet {
		for _, silence := range displaySilences {
			fmt.Println(silence.ID)
		}
	} else {
		formatter, found := format.Formatters[*output]
		if !found {
			return errors.New("unknown output formatter")
		}
		formatter.FormatSilences(displaySilences)
	}
	return nil
}
