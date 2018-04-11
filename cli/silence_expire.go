package cli

import (
	"context"
	"errors"

	"github.com/alecthomas/kingpin"
	"github.com/prometheus/client_golang/api"

	"github.com/prometheus/alertmanager/client"
)

var (
	expireCmd = silenceCmd.Command("expire", "expire an alertmanager silence")
	expireIds = expireCmd.Arg("silence-ids", "Ids of silences to expire").Strings()
)

func init() {
	expireCmd.Action(expire)
	longHelpText["silence expire"] = `Expire an alertmanager silence`
}

func expire(element *kingpin.ParseElement, ctx *kingpin.ParseContext) error {
	if len(*expireIds) < 1 {
		return errors.New("no silence IDs specified")
	}

	c, err := api.NewClient(api.Config{Address: (*alertmanagerUrl).String()})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(c)

	for _, id := range *expireIds {
		err := silenceAPI.Expire(context.Background(), id)
		if err != nil {
			return err
		}
	}

	return nil
}
