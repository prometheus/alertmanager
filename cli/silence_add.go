package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/user"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

type addResponse struct {
	Status string `json:"status"`
	Data   struct {
		SilenceID string `json:"silenceId"`
	} `json:"data,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
	Error     string `json:"error,omitempty"`
}

func username() string {
	user, err := user.Current()
	if err != nil {
		return ""
	}
	return user.Username
}

var (
	addCmd         = silenceCmd.Command("add", "Add a new alertmanager silence")
	author         = addCmd.Flag("author", "Username for CreatedBy field").Short('a').Default(username()).String()
	requireComment = addCmd.Flag("require-comment", "Require comment to be set").Hidden().Default("true").Bool()
	expires        = addCmd.Flag("expires", "Duration of silence").Short('e').Default("1h").String()
	expireOn       = addCmd.Flag("expire-on", "Expire at a certain time (Overwrites expires) RFC3339 format 2006-01-02T15:04:05Z07:00").String()
	comment        = addCmd.Flag("comment", "A comment to help describe the silence").Short('c').String()
	addArgs        = addCmd.Arg("matcher-groups", "Query filter").Strings()
)

func init() {
	addCmd.Action(add)
	longHelpText["silence add"] = `Add a new alertmanager silence

  Amtool uses a simplified prometheus syntax to represent silences. The
  non-option section of arguments constructs a list of "Matcher Groups"
  that will be used to create a number of silences. The following examples
  will attempt to show this behaviour in action:

  amtool silence add alertname=foo node=bar

	This statement will add a silence that matches alerts with the
	alertname=foo and node=bar label value pairs set.

  amtool silence add foo node=bar

	If alertname is ommited and the first argument does not contain a '=' or a
	'=~' then it will be assumed to be the value of the alertname pair.

  amtool silence add 'alertname=~foo.*'

	As well as direct equality, regex matching is also supported. The '=~' syntax
	(similar to prometheus) is used to represent a regex match. Regex matching
	can be used in combination with a direct match.
`
}

func add(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	var err error

	matchers, err := parseMatchers(*addArgs)
	if err != nil {
		return err
	}

	if len(matchers) < 1 {
		return fmt.Errorf("no matchers specified")
	}

	var endsAt time.Time
	if *expireOn != "" {
		endsAt, err = time.Parse(time.RFC3339, *expireOn)
		if err != nil {
			return err
		}
	} else {
		duration, err := model.ParseDuration(*expires)
		if err != nil {
			return err
		}
		if duration == 0 {
			return fmt.Errorf("silence duration must be greater than 0")
		}
		endsAt = time.Now().UTC().Add(time.Duration(duration))
	}

	if *requireComment && *comment == "" {
		return errors.New("comment required by config")
	}

	typeMatchers, err := TypeMatchers(matchers)
	if err != nil {
		return err
	}

	silence := types.Silence{
		Matchers:  typeMatchers,
		StartsAt:  time.Now().UTC(),
		EndsAt:    endsAt,
		CreatedBy: *author,
		Comment:   *comment,
	}

	silenceId, err := addSilence(&silence)
	if err != nil {
		return err
	}

	_, err = fmt.Println(silenceId)
	return err
}

func addSilence(silence *types.Silence) (string, error) {
	u := GetAlertmanagerURL("/api/v1/silences")

	buf := bytes.NewBuffer([]byte{})
	err := json.NewEncoder(buf).Encode(silence)
	if err != nil {
		return "", err
	}

	res, err := http.Post(u.String(), "application/json", buf)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()
	response := addResponse{}
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return "", fmt.Errorf("unable to parse silence json response from %s", u.String())
	}

	if response.Status == "error" {
		return "", fmt.Errorf("[%s] %s", response.ErrorType, response.Error)
	}

	return response.Data.SilenceID, nil
}
