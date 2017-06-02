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
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/pkg/parse"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type alertmanagerAlertResponse struct {
	Status    string        `json:"status"`
	Data      []*alertGroup `json:"data,omitempty"`
	ErrorType string        `json:"errorType,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type alertGroup struct {
	Labels   model.LabelSet `json:"labels"`
	GroupKey string         `json:"groupKey"`
	Blocks   []*alertBlock  `json:"blocks"`
}

type alertBlock struct {
	RouteOpts interface{}          `json:"routeOpts"`
	Alerts    []*dispatch.APIAlert `json:"alerts"`
}

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
	`,
	Run: CommandWrapper(queryAlerts),
}

var alertQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "View and search through current alerts",
	Long:  alertCmd.Long,
	RunE:  queryAlerts,
}

func init() {
	RootCmd.AddCommand(alertCmd)
	alertCmd.AddCommand(alertQueryCmd)
	alertQueryCmd.Flags().Bool("expired", false, "Show expired alerts as well as active")
	alertQueryCmd.Flags().BoolP("silenced", "s", false, "Show silenced alerts")
	viper.BindPFlag("expired", alertQueryCmd.Flags().Lookup("expired"))
	viper.BindPFlag("silenced", alertQueryCmd.Flags().Lookup("silenced"))
}

func fetchAlerts(filter string) ([]*dispatch.APIAlert, error) {
	alertResponse := alertmanagerAlertResponse{}

	u, err := GetAlertmanagerURL()
	if err != nil {
		return []*dispatch.APIAlert{}, err
	}

	u.Path = path.Join(u.Path, "/api/v1/alerts/groups")
	u.RawQuery = "filter=" + url.QueryEscape(filter)

	res, err := http.Get(u.String())
	if err != nil {
		return []*dispatch.APIAlert{}, err
	}

	defer res.Body.Close()

	err = json.NewDecoder(res.Body).Decode(&alertResponse)
	if err != nil {
		return []*dispatch.APIAlert{}, fmt.Errorf("Unable to decode json response: %s", err)
	}

	if alertResponse.Status != "success" {
		return []*dispatch.APIAlert{}, fmt.Errorf("[%s] %s", alertResponse.ErrorType, alertResponse.Error)
	}

	return flattenAlertOverview(alertResponse.Data), nil
}

func flattenAlertOverview(overview []*alertGroup) []*dispatch.APIAlert {
	alerts := []*dispatch.APIAlert{}
	for _, group := range overview {
		for _, block := range group.Blocks {
			alerts = append(alerts, block.Alerts...)
		}
	}
	return alerts
}

func queryAlerts(cmd *cobra.Command, args []string) error {
	expired := viper.GetBool("expired")
	showSilenced := viper.GetBool("silenced")

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

	fetchedAlerts, err := fetchAlerts(filterString)
	if err != nil {
		return err
	}

	displayAlerts := []*dispatch.APIAlert{}
	for _, alert := range fetchedAlerts {
		// If we are only returning current alerts and this one has already expired skip it
		if !expired {
			if !alert.EndsAt.IsZero() && alert.EndsAt.Before(time.Now()) {
				continue
			}
		}

		if !showSilenced {
			// If any silence mutes this alert don't show it
			if alert.Status.State == types.AlertStateSuppressed && len(alert.Status.SilencedBy) > 0 {
				continue
			}
		}

		displayAlerts = append(displayAlerts, alert)
	}

	formatter, found := format.Formatters[viper.GetString("output")]
	if !found {
		return errors.New("Unknown output formatter")
	}
	return formatter.FormatAlerts(displayAlerts)
}
