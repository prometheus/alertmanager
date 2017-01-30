package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"time"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/types"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var queryFlags *flag.FlagSet
var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query silences",
	Long: `Query Alertmanager silences.

  Amtool has a simplified prometheus query syntax, but contains robust support for
  bash variable expansions. The non-option section of arguments constructs a list
  of "Matcher Groups" that will be used to filter your query. The following
  examples will attempt to show this behaviour in action:

  amtool silence query alertname=foo node=bar

  	This query will match all silences with the alertname=foo and node=bar label
  	value pairs set.

  amtool silence query foo node=bar

  	If alertname is ommited and the first argument does not contain a '=' or a
  	'=~' then it will be assumed to be the value of the alertname pair.

  amtool silence query 'alertname=~foo.*'

  	As well as direct equality, regex matching is also supported. The '=~' syntax
  	(similar to prometheus) is used to represent a regex match. Regex matching
  	can be used in combination with a direct match.

  amtool silence query alertname=foo node={bar,baz}

  	This query will match all silences with the alertname=foo label value pair
  	and EITHER node=bar or node=baz.

  amtool silence query alertname=foo{a,b} node={bar,baz}

	Similar to the previous example this query will match all silences with any
	combination of alertname=fooa or alertname=foob AND node=bar or node=baz.
				`,
	RunE: query,
}

func init() {
	queryCmd.Flags().Bool("expired", false, "Show expired silences as well as active")
	queryFlags = queryCmd.Flags()
}

func fetchSilences() ([]types.Silence, error) {
	silenceResponse := alertmanagerSilenceResponse{}
	u, err := GetAlertmanagerURL()
	if err != nil {
		return []types.Silence{}, err
	}

	u.Path = path.Join(u.Path, "/api/v1/silences")
	res, err := http.Get(u.String())
	if err != nil {
		return []types.Silence{}, err
	}

	defer res.Body.Close()
	decoder := json.NewDecoder(res.Body)

	err = decoder.Decode(&silenceResponse)
	if err != nil {
		return []types.Silence{}, err
	}

	return silenceResponse.Data, nil
}

func query(cmd *cobra.Command, args []string) error {
	silences, err := fetchSilences()
	if err != nil {
		return err
	}

	expired, err := queryFlags.GetBool("expired")
	if err != nil {
		return err
	}

	displaySilences := []types.Silence{}
	var groups []types.Matchers
	if len(args) > 0 {
		matchers, err := parseMatchers(args)
		if err != nil {
			return err
		}
		groups = parseMatcherGroups(matchers)
		if err != nil {
			return err
		}
	}

	for _, silence := range silences {
		// If we are only returning current silences and this one has already expired skip it
		if !expired && silence.EndsAt.Before(time.Now()) {
			continue
		}

		// If the user hasn't specified and match groups then let it through
		if len(groups) < 1 {
			displaySilences = append(displaySilences, silence)
			continue
		}

		for _, matchers := range groups {
			if groupMatch(silence.Matchers, matchers) {
				displaySilences = append(displaySilences, silence)
				break
			}
		}
	}

	formatter, found := format.Formatters[viper.GetString("output")]
	if !found {
		return errors.New("Unknown output formatter")
	}
	formatter.FormatSilences(displaySilences)
	return nil
}
