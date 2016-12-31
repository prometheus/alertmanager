package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
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
	Long:  `Query Alertmanager silences`,
	RunE:  query,
}

func init() {
	queryCmd.Flags().Bool("all", false, "Show expired silences as well as active")
	queryCmd.Flags().StringP("output", "o", "simple", "Output formatter (simple, extended, json)")
	viper.BindPFlag("output", queryCmd.Flags().Lookup("output"))
	queryFlags = queryCmd.Flags()
}

func query(cmd *cobra.Command, args []string) error {
	silenceResponse := alertmanagerResponse{}
	u, err := url.Parse(viper.GetString("alertmanager"))
	if err != nil {
		return err
	}

	u.Path = path.Join(u.Path, "/api/v1/silences")
	res, err := http.Get(u.String())
	if err != nil {
		return err
	}

	defer res.Body.Close()
	decoder := json.NewDecoder(res.Body)

	err = decoder.Decode(&silenceResponse)
	if err != nil {
		return err
	}

	all, err := queryFlags.GetBool("all")
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

	for _, silence := range silenceResponse.Data {
		// If we are only returning current silences and this one has already expired skip it
		if !all && silence.EndsAt.Before(time.Now()) {
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
	formatter.Init(os.Stdout)
	formatter.Format(displaySilences)
	//fmt.Printf("%+v\n", displaySilences)
	return nil
}
