package cli

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"time"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/model"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/types"
)

func username() string {
	user, err := user.Current()
	if err != nil {
		return ""
	}
	return user.Username
}

type silenceAddCmd struct {
	author         string
	requireComment bool
	duration       string
	start          string
	end            string
	comment        string
	matchers       []string
}

const silenceAddHelp = `Add a new alertmanager silence

  Amtool uses a simplified Prometheus syntax to represent silences. The
  non-option section of arguments constructs a list of "Matcher Groups"
  that will be used to create a number of silences. The following examples
  will attempt to show this behaviour in action:

  amtool silence add alertname=foo node=bar

	This statement will add a silence that matches alerts with the
	alertname=foo and node=bar label value pairs set.

  amtool silence add foo node=bar

	If alertname is omitted and the first argument does not contain a '=' or a
	'=~' then it will be assumed to be the value of the alertname pair.

  amtool silence add 'alertname=~foo.*'

	As well as direct equality, regex matching is also supported. The '=~' syntax
	(similar to Prometheus) is used to represent a regex match. Regex matching
	can be used in combination with a direct match.
`

func configureSilenceAddCmd(cc *kingpin.CmdClause) {
	var (
		c      = &silenceAddCmd{}
		addCmd = cc.Command("add", silenceAddHelp)
	)
	addCmd.Flag("author", "Username for CreatedBy field").Short('a').Default(username()).StringVar(&c.author)
	addCmd.Flag("require-comment", "Require comment to be set").Hidden().Default("true").BoolVar(&c.requireComment)
	addCmd.Flag("duration", "Duration of silence").Short('d').Default("1h").StringVar(&c.duration)
	addCmd.Flag("start", "Set when the silence should start. RFC3339 format 2006-01-02T15:04:05Z07:00").StringVar(&c.start)
	addCmd.Flag("end", "Set when the silence should end (overwrites duration). RFC3339 format 2006-01-02T15:04:05Z07:00").StringVar(&c.end)
	addCmd.Flag("comment", "A comment to help describe the silence").Short('c').StringVar(&c.comment)
	addCmd.Arg("matcher-groups", "Query filter").StringsVar(&c.matchers)
	addCmd.Action(c.add)

}

func (c *silenceAddCmd) add(ctx *kingpin.ParseContext) error {
	var err error

	matchers, err := parseMatchers(c.matchers)
	if err != nil {
		return err
	}

	if len(matchers) < 1 {
		return fmt.Errorf("no matchers specified")
	}

	var endsAt time.Time
	if c.end != "" {
		endsAt, err = time.Parse(time.RFC3339, c.end)
		if err != nil {
			return err
		}
	} else {
		d, err := model.ParseDuration(c.duration)
		if err != nil {
			return err
		}
		if d == 0 {
			return fmt.Errorf("silence duration must be greater than 0")
		}
		endsAt = time.Now().UTC().Add(time.Duration(d))
	}

	if c.requireComment && c.comment == "" {
		return errors.New("comment required by config")
	}

	var startsAt time.Time
	if c.start != "" {
		startsAt, err = time.Parse(time.RFC3339, c.start)
		if err != nil {
			return err
		}

	} else {
		startsAt = time.Now().UTC()
	}

	if startsAt.After(endsAt) {
		return errors.New("silence cannot start after it ends")
	}

	typeMatchers, err := TypeMatchers(matchers)
	if err != nil {
		return err
	}

	silence := types.Silence{
		Matchers:  typeMatchers,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
		CreatedBy: c.author,
		Comment:   c.comment,
	}

	apiClient, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(apiClient)
	silenceID, err := silenceAPI.Set(context.Background(), silence)
	if err != nil {
		return err
	}

	_, err = fmt.Println(silenceID)
	return err
}
