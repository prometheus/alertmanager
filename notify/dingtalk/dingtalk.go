// Copyright 2021 Prometheus Team
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

package dingtalk

import (
	"context"

	"github.com/CatchZeng/dingtalk/pkg/dingtalk"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

type Notifier struct {
	conf   *config.DingtalkConfig
	tmpl   *template.Template
	logger log.Logger
	client *dingtalk.Client
}

func New(c *config.DingtalkConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	client := dingtalk.NewClient(c.AccessToken, c.Secret)
	return &Notifier{conf: c, tmpl: t, logger: l, client: client}, nil
}

func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var err error
	var (
		data     = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmplText = notify.TmplText(n.tmpl, data, &err)
	)
	title := tmplText(n.conf.Title)
	if err != nil {
		return false, errors.Wrap(err, "execute 'Title' template")
	}
	text := tmplText(n.conf.Text)
	if err != nil {
		return false, errors.Wrap(err, "execute 'Text' template")
	}

	msg := dingtalk.NewMarkdownMessage().
		SetMarkdown(title, text).
		SetAt(n.conf.AtMobiles, n.conf.IsAtAll)

	reqString, _, err := n.client.Send(msg)
	if err != nil {
		level.Warn(n.logger).Log("reqString", reqString)
		return false, err
	}

	return true, nil
}
