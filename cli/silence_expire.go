package cli

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/api"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/client"
)

type silenceExpireCmd struct {
	ids []string
}

func configureSilenceExpireCmd(cc *kingpin.CmdClause, longHelpText map[string]string) {
	var (
		c         = &silenceExpireCmd{}
		expireCmd = cc.Command("expire", "expire an alertmanager silence")
	)
	expireCmd.Arg("silence-ids", "Ids of silences to expire").StringsVar(&c.ids)

	expireCmd.Action(c.expire)
	longHelpText["silence expire"] = `Expire an alertmanager silence`
}

func (c *silenceExpireCmd) expire(ctx *kingpin.ParseContext) error {
	if len(c.ids) < 1 {
		return errors.New("no silence IDs specified")
	}

	apiClient, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(apiClient)

	for _, id := range c.ids {
		err := silenceAPI.Expire(context.Background(), id)
		if err != nil {
			return err
		}
	}

	return nil
}
