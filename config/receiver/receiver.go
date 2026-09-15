// Copyright 2023 Prometheus Team
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

package receiver

import (
	"errors"
	"log/slog"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/discord"
	"github.com/prometheus/alertmanager/notify/email"
	"github.com/prometheus/alertmanager/notify/incidentio"
	"github.com/prometheus/alertmanager/notify/jira"
	"github.com/prometheus/alertmanager/notify/mattermost"
	"github.com/prometheus/alertmanager/notify/msteams"
	"github.com/prometheus/alertmanager/notify/msteamsv2"
	"github.com/prometheus/alertmanager/notify/opsgenie"
	"github.com/prometheus/alertmanager/notify/pagerduty"
	"github.com/prometheus/alertmanager/notify/pushover"
	"github.com/prometheus/alertmanager/notify/rocketchat"
	"github.com/prometheus/alertmanager/notify/slack"
	"github.com/prometheus/alertmanager/notify/sns"
	"github.com/prometheus/alertmanager/notify/telegram"
	"github.com/prometheus/alertmanager/notify/victorops"
	"github.com/prometheus/alertmanager/notify/webex"
	"github.com/prometheus/alertmanager/notify/webhook"
	"github.com/prometheus/alertmanager/notify/wechat"
	"github.com/prometheus/alertmanager/template"
)

// BuildReceiverIntegrations builds a list of integration notifiers off of a
// receiver config.
func BuildReceiverIntegrations(nc config.Receiver, tmpl *template.Template, logger *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) ([]notify.Integration, error) {
	if logger == nil {
		logger = promslog.NewNopLogger()
	}

	var (
		errs         error
		integrations []notify.Integration
		add          = func(name string, i int, rs notify.ResolvedSender, f func(l *slog.Logger) (notify.Notifier, error)) {
			n, err := f(logger.With("integration", name))
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			integrations = append(integrations, notify.NewIntegration(n, rs, name, i, nc.Name))
		}
	)

	for i, c := range nc.WebhookConfigs {
		add("webhook", i, c, func(l *slog.Logger) (notify.Notifier, error) { return webhook.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.EmailConfigs {
		add("email", i, c, func(l *slog.Logger) (notify.Notifier, error) { return email.New(c, tmpl, l), nil })
	}
	for i, c := range nc.PagerdutyConfigs {
		add("pagerduty", i, c, func(l *slog.Logger) (notify.Notifier, error) { return pagerduty.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.OpsGenieConfigs {
		add("opsgenie", i, c, func(l *slog.Logger) (notify.Notifier, error) { return opsgenie.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.WechatConfigs {
		add("wechat", i, c, func(l *slog.Logger) (notify.Notifier, error) { return wechat.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.SlackConfigs {
		add("slack", i, c, func(l *slog.Logger) (notify.Notifier, error) { return slack.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.VictorOpsConfigs {
		add("victorops", i, c, func(l *slog.Logger) (notify.Notifier, error) { return victorops.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.PushoverConfigs {
		add("pushover", i, c, func(l *slog.Logger) (notify.Notifier, error) { return pushover.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.SNSConfigs {
		add("sns", i, c, func(l *slog.Logger) (notify.Notifier, error) { return sns.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.TelegramConfigs {
		add("telegram", i, c, func(l *slog.Logger) (notify.Notifier, error) { return telegram.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.DiscordConfigs {
		add("discord", i, c, func(l *slog.Logger) (notify.Notifier, error) { return discord.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.WebexConfigs {
		add("webex", i, c, func(l *slog.Logger) (notify.Notifier, error) { return webex.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.MSTeamsConfigs {
		add("msteams", i, c, func(l *slog.Logger) (notify.Notifier, error) { return msteams.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.MSTeamsV2Configs {
		add("msteamsv2", i, c, func(l *slog.Logger) (notify.Notifier, error) { return msteamsv2.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.JiraConfigs {
		add("jira", i, c, func(l *slog.Logger) (notify.Notifier, error) { return jira.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.IncidentioConfigs {
		add("incidentio", i, c, func(l *slog.Logger) (notify.Notifier, error) { return incidentio.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.RocketchatConfigs {
		add("rocketchat", i, c, func(l *slog.Logger) (notify.Notifier, error) { return rocketchat.New(c, tmpl, l, httpOpts...) })
	}
	for i, c := range nc.MattermostConfigs {
		add("mattermost", i, c, func(l *slog.Logger) (notify.Notifier, error) { return mattermost.New(c, tmpl, l, httpOpts...) })
	}

	return integrations, errs
}
