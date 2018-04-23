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
	updateCmd.Flag("start", "Set when the silence should start. RFC3339 format 2006-01-02T15:04:05Z07:00").StringVar(&c.start)
	updateCmd.Flag("end", "Set when the silence should end (overwrites duration). RFC3339 format 2006-01-02T15:04:05Z07:00").StringVar(&c.end)
	updateCmd.Flag("comment", "A comment to help describe the silence").Short('c').StringVar(&c.comment)
	updateCmd.Arg("update-ids", "Silence IDs to update").StringsVar(&c.ids)

	updateCmd.Action(c.update)
}

func (c *silenceUpdateCmd) update(ctx *kingpin.ParseContext) error {
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
		silence, err := silenceAPI.Get(context.Background(), silenceID)
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

		newID, err := silenceAPI.Set(context.Background(), *silence)
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
		formatter.FormatSilences(updatedSilences)
	}
	return nil
}
