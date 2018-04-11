package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/client_golang/api"

	"github.com/prometheus/alertmanager/cli/format"
	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

var (
	updateCmd      = silenceCmd.Command("update", "Update silences")
	updateDuration = updateCmd.Flag("duration", "Duration of silence").Short('d').String()
	updateStart    = updateCmd.Flag("start", "Set when the silence should start. RFC3339 format 2006-01-02T15:04:05Z07:00").String()
	updateEnd      = updateCmd.Flag("end", "Set when the silence should end (overwrites duration). RFC3339 format 2006-01-02T15:04:05Z07:00").String()
	updateComment  = updateCmd.Flag("comment", "A comment to help describe the silence").Short('c').String()
	updateIds      = updateCmd.Arg("update-ids", "Silence IDs to update").Strings()
)

func init() {
	updateCmd.Action(update)
	longHelpText["silence update"] = `Extend or update existing silence in Alertmanager.`
}

func update(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	if len(*updateIds) < 1 {
		return fmt.Errorf("no silence IDs specified")
	}

	c, err := api.NewClient(api.Config{Address: (*alertmanagerUrl).String()})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(c)

	var updatedSilences []types.Silence
	for _, silenceID := range *updateIds {
		silence, err := silenceAPI.Get(context.Background(), silenceID)
		if err != nil {
			return err
		}
		if *updateStart != "" {
			silence.StartsAt, err = time.Parse(time.RFC3339, *updateStart)
			if err != nil {
				return err
			}
		}

		if *updateEnd != "" {
			silence.EndsAt, err = time.Parse(time.RFC3339, *updateEnd)
			if err != nil {
				return err
			}
		} else if *updateDuration != "" {
			d, err := model.ParseDuration(*updateDuration)
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

		if *updateComment != "" {
			silence.Comment = *updateComment
		}

		newID, err := silenceAPI.Set(context.Background(), *silence)
		if err != nil {
			return err
		}
		silence.ID = newID

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
