package cli

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

type getResponse struct {
	Status    string        `json:"status"`
	Data      types.Silence `json:"data,omitempty"`
	ErrorType string        `json:"errorType,omitempty"`
	Error     string        `json:"error,omitempty"`
}

var (
	updateCmd       = silenceCmd.Command("update", "Update silences")
	updateExpires   = updateCmd.Flag("expires", "Duration of silence").Short('e').Default("1h").String()
	updateExpiresOn = updateCmd.Flag("expire-on", "Expire at a certain time (Overwrites expires) RFC3339 format 2006-01-02T15:04:05Z07:00").Time(time.RFC3339)
	updateComment   = updateCmd.Flag("comment", "A comment to help describe the silence").Short('c').String()
	updateIds       = updateCmd.Arg("update-ids", "Silence IDs to update").Strings()
)

func init() {
	updateCmd.Action(update)
	longHelpText["silence update"] = `Extend or update existing silence in Alertmanager.`
}

func update(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	if len(*updateIds) < 1 {
		return fmt.Errorf("no silence IDs specified")
	}

	alertmanagerUrl := GetAlertmanagerURL("/api/v1/silence")
	var updatedSilences []types.Silence
	for _, silenceId := range *updateIds {
		silence, err := getSilenceById(silenceId, alertmanagerUrl)
		if err != nil {
			return err
		}
		silence, err = updateSilence(silence)
		if err != nil {
			return err
		}
		updatedSilences = append(updatedSilences, *silence)
	}

	if *silenceQuiet {
		for _, silence := range updatedSilences {
			fmt.Println(silence.ID)
		}
	} else {
		formatter, found := format.Formatters[*output]
		if !found {
			return fmt.Errorf("unknown output formatter")
		}
		formatter.FormatSilences(updatedSilences)
	}
	return nil
}

// This takes an url.URL and not a pointer as we will modify it for our API call.
func getSilenceById(silenceId string, baseUrl url.URL) (*types.Silence, error) {
	baseUrl.Path = path.Join(baseUrl.Path, silenceId)
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
	if *updateExpires != "" {
		duration, err := model.ParseDuration(*updateExpires)
		if err != nil {
			return nil, err
		}
		if duration == 0 {
			return nil, fmt.Errorf("silence duration must be greater than 0")
		}
		silence.EndsAt = time.Now().UTC().Add(time.Duration(duration))
	}

	// expire-on will override expires value if both are specified
	if !(*updateExpiresOn).IsZero() {
		silence.EndsAt = *updateExpiresOn
	}

	if *updateComment != "" {
		silence.Comment = *updateComment
	}

	// addSilence can also be used to update an existing silence
	_, err := addSilence(silence)
	if err != nil {
		return nil, err
	}
	return silence, nil
}
