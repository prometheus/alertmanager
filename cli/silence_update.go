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
	"time"

	"github.com/prometheus/client_golang/api"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

type silenceUpdateCmd struct {
	quiet    bool
	duration string
	start    string
	end      string
	comment  string
	ids      []string
}

func configureSilenceUpdateCmd(cc *kingpin.CmdClause) {
	var (
		c         = &silenceUpdateCmd{}
		updateCmd = cc.Command("update", "Update silences")
	)
	updateCmd.Flag("quiet", "Only show silence ids").Short('q').BoolVar(&c.quiet)
	updateCmd.Flag("duration", "Duration of silence").Short('d').StringVar(&c.duration)
	updateCmd.Flag("start", "Set when the silence should start. RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&c.start)
	updateCmd.Flag("end", "Set when the silence should end (overwrites duration). RFC3339 format 2006-01-02T15:04:05-07:00").StringVar(&c.end)
	updateCmd.Flag("comment", "A comment to help describe the silence").Short('c').StringVar(&c.comment)
	updateCmd.Arg("update-ids", "Silence IDs to update").StringsVar(&c.ids)

	updateCmd.Action(execWithTimeout(c.update))
}

func (c *silenceUpdateCmd) update(ctx context.Context, _ *kingpin.ParseContext) error {
	if len(c.ids) < 1 {
		return fmt.Errorf("no silence IDs specified")
	}

	apiClient, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(apiClient)

	var updatedSilences []types.Silence
	for _, silenceID := range c.ids {
		silence, err := silenceAPI.Get(ctx, silenceID)
		if err != nil {
			return err
		}
		if c.start != "" {
			silence.StartsAt, err = time.Parse(time.RFC3339, c.start)
			if err != nil {
				return err
			}
		}

		if c.end != "" {
			silence.EndsAt, err = time.Parse(time.RFC3339, c.end)
			if err != nil {
				return err
			}
		} else if c.duration != "" {
			d, err := model.ParseDuration(c.duration)
			if err != nil {
				return err
			}
			if d == 0 {
				return fmt.Errorf("silence duration must be greater than 0")
			}
			silence.EndsAt = silence.StartsAt.UTC().Add(time.Duration(d))
		}

		if silence.StartsAt.After(silence.EndsAt) {
			return errors.New("silence cannot start after it ends")
		}

		if c.comment != "" {
			silence.Comment = c.comment
		}

		newID, err := silenceAPI.Set(ctx, *silence)
		if err != nil {
			return err
		}
		silence.ID = newID

		updatedSilences = append(updatedSilences, *silence)
	}

	if c.quiet {
		for _, silence := range updatedSilences {
			fmt.Println(silence.ID)
		}
	} else {
		formatter, found := format.Formatters[output]
		if !found {
			return fmt.Errorf("unknown output formatter")
		}
		if err := formatter.FormatSilences(updatedSilences); err != nil {
			return fmt.Errorf("error formatting silences: %v", err)
		}
	}
	return nil
}
