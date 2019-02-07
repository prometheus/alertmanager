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

	"github.com/prometheus/client_golang/api"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/client"
)

type silenceExpireCmd struct {
	ids []string
}

func configureSilenceExpireCmd(cc *kingpin.CmdClause) {
	var (
		c         = &silenceExpireCmd{}
		expireCmd = cc.Command("expire", "expire an alertmanager silence")
	)
	expireCmd.Arg("silence-ids", "Ids of silences to expire").StringsVar(&c.ids)
	expireCmd.Action(execWithTimeout(c.expire))
}

func (c *silenceExpireCmd) expire(ctx context.Context, _ *kingpin.ParseContext) error {
	if len(c.ids) < 1 {
		return errors.New("no silence IDs specified")
	}

	apiClient, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(apiClient)

	for _, id := range c.ids {
		err := silenceAPI.Expire(ctx, id)
		if err != nil {
			return err
		}
	}

	return nil
}
