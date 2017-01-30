package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"time"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type alertmanagerAlertResponse struct {
	Status    string       `json:"status"`
	Data      model.Alerts `json:"data,omitempty"`
	ErrorType string       `json:"errorType,omitempty"`
	Error     string       `json:"error,omitempty"`
}

var alertFlags *flag.FlagSet

// alertCmd represents the alert command
var alertCmd = &cobra.Command{
	Use:   "alert",
	Short: "View and search through current alerts",
	Long: `View and search through current alerts.

  Amtool has a simplified prometheus query syntax, but contains robust support for
  bash variable expansions. The non-option section of arguments constructs a list
  of "Matcher Groups" that will be used to filter your query. The following
  examples will attempt to show this behaviour in action:

  amtool alert query alertname=foo node=bar

  	This query will match all alerts with the alertname=foo and node=bar label
  	value pairs set.

  amtool alert query foo node=bar

  	If alertname is ommited and the first argument does not contain a '=' or a
  	'=~' then it will be assumed to be the value of the alertname pair.

  amtool alert query 'alertname=~foo.*'

  	As well as direct equality, regex matching is also supported. The '=~' syntax
  	(similar to prometheus) is used to represent a regex match. Regex matching
  	can be used in combination with a direct match.

  amtool alert query alertname=foo node={bar,baz}

  	This query will match all alerts with the alertname=foo label value pair
  	and EITHER node=bar or node=baz.

  amtool alert query alertname=foo{a,b} node={bar,baz}

	Similar to the previous example this query will match all alerts with any
	combination of alertname=fooa or alertname=foob AND node=bar or node=baz.

	`,
	RunE: queryAlerts,
}

func init() {
	RootCmd.AddCommand(alertCmd)
	alertCmd.Flags().Bool("expired", false, "Show expired alerts as well as active")
	alertCmd.Flags().BoolP("silenced", "s", false, "Show silenced alerts")
	alertFlags = alertCmd.Flags()
}

func fetchAlerts() (model.Alerts, error) {
	alertResponse := alertmanagerAlertResponse{}

	u, err := GetAlertmanagerURL()
	if err != nil {
		return model.Alerts{}, err
	}

	u.Path = path.Join(u.Path, "/api/v1/alerts")
	res, err := http.Get(u.String())
	if err != nil {
		return model.Alerts{}, err
	}

	defer res.Body.Close()
	decoder := json.NewDecoder(res.Body)

	err = decoder.Decode(&alertResponse)
	if err != nil {
		return model.Alerts{}, errors.New("Unable to decode json response")
	}
	return alertResponse.Data, nil
}

func queryAlerts(cmd *cobra.Command, args []string) error {
	alerts, err := fetchAlerts()
	if err != nil {
		return err
	}

	silences, err := fetchSilences()
	if err != nil {
		return err
	}

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

	expired, err := alertFlags.GetBool("expired")
	if err != nil {
		return err
	}

	showSilenced, err := alertFlags.GetBool("silenced")
	if err != nil {
		return err
	}

	displayAlerts := model.Alerts{}
	for _, alert := range alerts {
		// If we are only returning current alerts and this one has already expired skip it
		if !expired {
			if !alert.EndsAt.IsZero() && alert.EndsAt.Before(time.Now()) {
				continue
			}
		}

		if !showSilenced {
			// If any silence mutes this alert don't show it
			silenced := false
			for _, silence := range silences {
				// Need to call Init before Mutes
				err = silence.Init()
				if err != nil {
					return err
				}

				if silence.Mutes(alert.Labels) {
					silenced = true
					break
				}
			}
			if silenced {
				continue
			}
		}

		// If the user hasn't specified and match groups then let it through
		if len(groups) < 1 {
			displayAlerts = append(displayAlerts, alert)
			continue
		}

		for _, matchers := range groups {
			if matchers.Match(alert.Labels) {
				displayAlerts = append(displayAlerts, alert)
				break
			}
		}
	}

	formatter, found := format.Formatters[viper.GetString("output")]
	if !found {
		return errors.New("Unknown output formatter")
	}
	return formatter.FormatAlerts(displayAlerts)
}
