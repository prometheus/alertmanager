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
	"fmt"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/integrationbuilder"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"gopkg.in/yaml.v2"
	"net/url"
	"time"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

type templatesRenderData struct {
	configFile   string
	receiverName string
	receiverId   int
	receiverType string

	alertLabels       string
	alertAnnotations  string
	alertGeneratorURL string
}

const (
	templatingHelp = `` // TODO add docs
)

func configureTemplatingCmd(app *kingpin.CmdClause) {
	var (
		c                  = &templatesRenderData{}
		templatesCmd       = app.Command("templates", templatingHelp)
		templatesRenderCmd = templatesCmd.Command("render", templatingHelp).Default()
	)
	templatesCmd.Flag("config.file", "Config file to be used.").Required().ExistingFileVar(&c.configFile)
	templatesCmd.Flag("receiver.name", "Name of receiver to be used for templating.").StringVar(&c.receiverName)
	templatesCmd.Flag("receiver.type", "Type of receiver to be used for templating.").StringVar(&c.receiverType)
	templatesCmd.Flag("receiver.id", "Id of receiver to be used for templating.").Default("-1").IntVar(&c.receiverId)

	templatesCmd.Flag("additional.labels", "Labels of the alert used for rendering of the templates.").Default("label-name=label-value").StringVar(&c.alertLabels)
	templatesCmd.Flag("additional.annotations", "Annotations of the alert used for rendering of the templates.").Default("annotation-name=annotation-value").StringVar(&c.alertAnnotations)

	templatesCmd.Flag("alert.count", "Number of alerts to be passed to the rendering.").Default("annotation-name=annotation-value").StringVar(&c.alertAnnotations)

	templatesRenderCmd.Action(execWithTimeout(c.renderReceiverNotification))
}

func (c *templatesRenderData) renderReceiverNotification(ctx context.Context, _ *kingpin.ParseContext) error {
	cfg, err := config.LoadFile(c.configFile)
	if err != nil {
		kingpin.Fatalf("%s", err)
		return err
	}
	tmpl, err := template.FromGlobs(cfg.Templates...)
	if err != nil {
		return err
	}
	tmpl.ExternalURL, _ = url.Parse("http://external.url/")
	ctx = notify.WithNow(ctx, time.Time{})
	ctx = notify.WithGroupKey(ctx, "<group-key>")
	ctx = notify.WithGroupLabels(ctx, model.LabelSet{})
	ctx = notify.WithReceiverName(ctx, c.receiverName)
	ctx = notify.WithRepeatInterval(ctx, time.Hour)

	var integrationsToRender []*notify.Integration
	for _, r := range cfg.Receivers {
		if c.receiverName != "" && r.Name != c.receiverName {
			continue
		}
		receiverIntegrations, err := integrationbuilder.BuildReceiverIntegrations(r, tmpl, promlog.New(&promlog.Config{}))
		if err != nil {
			return err
		}
		for _, i := range receiverIntegrations {
			if c.receiverType != "" && i.Name() != c.receiverType {
				continue
			}
			if c.receiverId != -1 && i.Index() != c.receiverId {
				continue
			}
			integrationsToRender = append(integrationsToRender, &i)
		}
	}

	results := make([]string, len(integrationsToRender))
	for i, integration := range integrationsToRender {
		fmt.Printf("--- # receiver: %s type: %s index: %d\n\n", "", integration.Name(), integration.Index())
		renderedConfig, err := integration.RenderConfiguration(ctx, &types.Alert{
			Alert: model.Alert{
				Labels:       map[model.LabelName]model.LabelValue{"foo": "bar"},
				Annotations:  map[model.LabelName]model.LabelValue{"foo": "bar"},
				StartsAt:     time.Time{},
				EndsAt:       time.Time{},
				GeneratorURL: "http://foo.bar",
			},
			UpdatedAt: time.Time{},
			Timeout:   false,
		})
		if err != nil {
			return err
		}
		configData, err := yaml.Marshal(renderedConfig)
		if err != nil {
			return err
		}
		results[i] = string(configData)
		fmt.Println(string(configData))
	}
	return nil
}
