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

	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/go-openapi/strfmt"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/cli/format"
)

type silenceDisplayCmd struct {
	ids []string
}

func configureSilenceDisplayCmd(cc *kingpin.CmdClause) {
	var (
		c          = &silenceDisplayCmd{}
		displayCmd = cc.Command("display", "display alertmanager silence")
	)
	displayCmd.Arg("silence-ids", "IDs of silences to display").StringsVar(&c.ids)
	displayCmd.Action(execWithTimeout(c.display))
}

func (c *silenceDisplayCmd) display(ctx context.Context, _ *kingpin.ParseContext) error {
	if len(c.ids) < 1 {
		return errors.New("no silence IDs specified")
	}

	amclient := NewAlertmanagerClient(alertmanagerURL)
	params := silence.NewGetSilenceParams()

	displaySilences := []models.GettableSilence{}
	for _, silenceID := range c.ids {
		if !strfmt.IsUUID(silenceID) {
			return fmt.Errorf("%s is not a valid UUID", silenceID)
		}
		params.SilenceID = strfmt.UUID(silenceID)
		response, err := amclient.Silence.GetSilence(params)
		if err != nil {
			return err
		}
		displaySilences = append(displaySilences, *response.Payload)
	}

	formatter, found := format.Formatters[output]
	if !found {
		return fmt.Errorf("unknown output formatter")
	}
	if err := formatter.FormatSilences(displaySilences); err != nil {
		return fmt.Errorf("error formatting silences: %v", err)
	}

	return nil
}
