package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/types"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type getResponse struct {
	Status    string        `json:"status"`
	Data      types.Silence `json:"data,omitempty"`
	ErrorType string        `json:"errorType,omitempty"`
	Error     string        `json:"error,omitempty"`
}

var updateFlags *flag.FlagSet
var updateCmd = &cobra.Command{
	Use:     "update <id> ...",
	Aliases: []string{"extend"},
	Args:    cobra.MinimumNArgs(1),
	Short:   "Update silences",
	Long:    `Extend or update existing silence in Alertmanager.`,
	Run:     CommandWrapper(update),
}

func init() {
	updateCmd.Flags().StringP("expires", "e", "", "Duration of silence (100h)")
	updateCmd.Flags().String("expire-on", "", "Expire at a certain time (Overwrites expires) RFC3339 format 2006-01-02T15:04:05Z07:00")
	updateCmd.Flags().StringP("comment", "c", "", "A comment to help describe the silence")
	updateFlags = updateCmd.Flags()
}

func update(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("no silence IDs specified")
	}

	alertmanagerUrl, err := GetAlertmanagerURL()
	if err != nil {
		return err
	}

	var updatedSilences []types.Silence
	for _, silenceId := range args {
		silence, err := getSilenceById(silenceId, *alertmanagerUrl)
		if err != nil {
			return err
		}
		silence, err = updateSilence(silence)
		if err != nil {
			return err
		}
		updatedSilences = append(updatedSilences, *silence)
	}

	quiet := viper.GetBool("quiet")
	if quiet {
		for _, silence := range updatedSilences {
			fmt.Println(silence.ID)
		}
	} else {
		formatter, found := format.Formatters[viper.GetString("output")]
		if !found {
			return fmt.Errorf("unknown output formatter")
		}
		formatter.FormatSilences(updatedSilences)
	}
	return nil
}

// This takes an url.URL and not a pointer as we will modify it for our API call.
func getSilenceById(silenceId string, baseUrl url.URL) (*types.Silence, error) {
	baseUrl.Path = path.Join(baseUrl.Path, "/api/v1/silence", silenceId)
	res, err := http.Get(baseUrl.String())
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("couldn't read response body: %v", err)
	}

	if res.StatusCode == 404 {
		return nil, fmt.Errorf("no silence found with id: %v", silenceId)
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("received %d response from Alertmanager: %v", res.StatusCode, body)
	}

	var response getResponse
	err = json.Unmarshal(body, &response)
	return &response.Data, nil
}

func updateSilence(silence *types.Silence) (*types.Silence, error) {
	if updateFlags.Changed("expires") {
		expires, err := updateFlags.GetString("expires")
		if err != nil {
			return nil, err
		}
		duration, err := time.ParseDuration(expires)
		if err != nil {
			return nil, err
		}
		silence.EndsAt = time.Now().UTC().Add(duration)
	}

	// expire-on will override expires value if both are specified
	if updateFlags.Changed("expire-on") {
		expireOn, err := updateFlags.GetString("expire-on")
		if err != nil {
			return nil, err
		}
		endsAt, err := time.Parse(time.RFC3339, expireOn)
		if err != nil {
			return nil, err
		}
		silence.EndsAt = endsAt
	}

	if updateFlags.Changed("comment") {
		comment, err := updateFlags.GetString("comment")
		if err != nil {
			return nil, err
		}
		silence.Comment = comment
	}

	// addSilence can also be used to update an existing silence
	_, err := addSilence(silence)
	if err != nil {
		return nil, err
	}
	return silence, nil
}
