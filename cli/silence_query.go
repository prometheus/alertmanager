package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/pkg/parse"
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
				`,
	Run: CommandWrapper(query),
}

func init() {
	queryCmd.Flags().Bool("expired", false, "Show expired silences as well as active")
	queryFlags = queryCmd.Flags()
}

func fetchSilences(filter string) ([]types.Silence, error) {
	silenceResponse := alertmanagerSilenceResponse{}

	u, err := GetAlertmanagerURL()
	if err != nil {
		return []types.Silence{}, err
	}

	u.Path = path.Join(u.Path, "/api/v1/silences")
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

func query(cmd *cobra.Command, args []string) error {
	expired, err := queryFlags.GetBool("expired")
	if err != nil {
		return err
	}

	quiet := viper.GetBool("quiet")

	var filterString = ""
	if len(args) == 1 {
		// If we only have one argument then it's possible that the user wants me to assume alertname=<arg>
		// Attempt to use the parser to pare the argument
		// If the parser fails then we likely don't have a (=|=~|!=|!~) so lets prepend `alertname=` to the front
		_, err := parse.Matcher(args[0])
		if err != nil {
			filterString = fmt.Sprintf("{alertname=%s}", args[0])
		} else {
			filterString = fmt.Sprintf("{%s}", strings.Join(args, ","))
		}
	} else if len(args) > 1 {
		filterString = fmt.Sprintf("{%s}", strings.Join(args, ","))
	}

	fetchedSilences, err := fetchSilences(filterString)
	if err != nil {
		return err
	}

	displaySilences := []types.Silence{}
	for _, silence := range fetchedSilences {
		// If we are only returning current silences and this one has already expired skip it
		if !expired && silence.EndsAt.Before(time.Now()) {
			continue
		}
		displaySilences = append(displaySilences, silence)
	}

	if quiet {
		for _, silence := range displaySilences {
			fmt.Println(silence.ID)
		}
	} else {
		formatter, found := format.Formatters[viper.GetString("output")]
		if !found {
			return errors.New("Unknown output formatter")
		}
		formatter.FormatSilences(displaySilences)
	}
	return nil
}
