// Copyright 2018 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"errors"
	"fmt"
	"os/user"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-openapi/strfmt"
	new_matchers "github.com/grobinson-grafana/matchers"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
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
	addCmd.Flag("start", "Set when the silence should start. RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&c.start)
	addCmd.Flag("end", "Set when the silence should end (overwrites duration). RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&c.end)
	addCmd.Flag("comment", "A comment to help describe the silence").Short('c').StringVar(&c.comment)
	addCmd.Arg("matcher-groups", "Query filter").StringsVar(&c.matchers)
	addCmd.Action(execWithTimeout(c.add))
}

// isLabelValue returns true if the input is a label value without the rest of
// the matcher. For example, foo and "foo" are both possible label values,
// but =foo is ambiguous as it could be intended as either alertname="foo"
// or alertname="=foo", and in such cases this function returns false.
func isLabelValue(s string) bool {
	var (
		err error
		tok new_matchers.Token
		l   = new_matchers.NewLexer(s)
	)
	// If the next token is invalid or ErrorEOF then this cannot be a label value
	if tok, err = l.Scan(); err != nil {
		return false
	}
	// If the next token is neither an ident or a quoted then it cannot be
	// a label value
	if tok.Kind != new_matchers.TokenIdent && tok.Kind != new_matchers.TokenQuoted {
		return false
	}
	// Last if this is a label value there should be no tokens after it
	if tok, _ = l.Peek(); tok.Kind != new_matchers.TokenNone {
		return false
	}
	return true
}

func (c *silenceAddCmd) add(ctx context.Context, _ *kingpin.ParseContext) error {
	var err error

	if len(c.matchers) > 0 {
		s := c.matchers[0]
		// If the parser fails then we likely don't have a (=|=~|!=|!~) so lets
		// assume that the user wants alertname=<arg> and prepend `alertname=`
		// to the front.
		if isLabelValue(s) {
			c.matchers[0] = fmt.Sprintf("alertname=%s", s)
		}
	}

	matchers, err := parseMatchers(c.matchers)
	if err != nil {
		return err
	}

	if len(matchers) < 1 {
		return fmt.Errorf("no matchers specified")
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
		endsAt = startsAt.UTC().Add(time.Duration(d))
	}

	if startsAt.After(endsAt) {
		return errors.New("silence cannot start after it ends")
	}

	if c.requireComment && c.comment == "" {
		return errors.New("comment required by config")
	}

	start := strfmt.DateTime(startsAt)
	end := strfmt.DateTime(endsAt)
	ps := &models.PostableSilence{
		Silence: models.Silence{
			Matchers:  TypeMatchers(matchers),
			StartsAt:  &start,
			EndsAt:    &end,
			CreatedBy: &c.author,
			Comment:   &c.comment,
		},
	}
	silenceParams := silence.NewPostSilencesParams().WithContext(ctx).
		WithSilence(ps)

	amclient := NewAlertmanagerClient(alertmanagerURL)

	postOk, err := amclient.Silence.PostSilences(silenceParams)
	if err != nil {
		return err
	}
	_, err = fmt.Println(postOk.Payload.SilenceID)
	return err
}
