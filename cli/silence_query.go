package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/client_golang/api"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/pkg/parse"
	"github.com/prometheus/alertmanager/types"
)

type silenceQueryCmd struct {
	expired  bool
	quiet    bool
	matchers []string
	within   time.Duration
}

func configureSilenceQueryCmd(cc *kingpin.CmdClause, longHelpText map[string]string) {
	var (
		c        = &silenceQueryCmd{}
		queryCmd = cc.Command("query", "Query Alertmanager silences.").Default()
	)

	queryCmd.Flag("expired", "Show expired silences instead of active").BoolVar(&c.expired)
	queryCmd.Flag("quiet", "Only show silence ids").Short('q').BoolVar(&c.quiet)
	queryCmd.Arg("matcher-groups", "Query filter").StringsVar(&c.matchers)
	queryCmd.Flag("within", "Show silences that will expire or have expired within a duration").DurationVar(&c.within)
	queryCmd.Action(c.query)
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

func (c *silenceQueryCmd) query(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	var filterString = ""
	if len(c.matchers) == 1 {
		// If the parser fails then we likely don't have a (=|=~|!=|!~) so lets
		// assume that the user wants alertname=<arg> and prepend `alertname=`
		// to the front.
		_, err := parse.Matcher(c.matchers[0])
		if err != nil {
			filterString = fmt.Sprintf("{alertname=%s}", c.matchers[0])
		} else {
			filterString = fmt.Sprintf("{%s}", strings.Join(c.matchers, ","))
		}
	} else if len(c.matchers) > 1 {
		filterString = fmt.Sprintf("{%s}", strings.Join(c.matchers, ","))
	}

	apiClient, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(apiClient)
	fetchedSilences, err := silenceAPI.List(context.Background(), filterString)
	if err != nil {
		return err
	}

	displaySilences := []types.Silence{}
	for _, silence := range fetchedSilences {
		// skip expired silences if --expired is not set
		if !c.expired && silence.EndsAt.Before(time.Now()) {
			continue
		}
		// skip active silences if --expired is set
		if c.expired && silence.EndsAt.After(time.Now()) {
			continue
		}
		// skip active silences expiring after "--within"
		if !c.expired && int64(c.within) > 0 && silence.EndsAt.After(time.Now().UTC().Add(c.within)) {
			continue
		}
		// skip silences that expired before "--within"
		if c.expired && int64(c.within) > 0 && silence.EndsAt.Before(time.Now().UTC().Add(-c.within)) {
			continue
		}

		displaySilences = append(displaySilences, *silence)
	}

	if c.quiet {
		for _, silence := range displaySilences {
			fmt.Println(silence.ID)
		}
	} else {
		formatter, found := format.Formatters[output]
		if !found {
			return errors.New("unknown output formatter")
		}
		formatter.FormatSilences(displaySilences)
	}
	return nil
}
