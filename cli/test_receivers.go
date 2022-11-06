// Copyright 2022 Prometheus Team
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
	"net/url"
	"time"

	"github.com/go-kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/discord"
	"github.com/prometheus/alertmanager/notify/email"
	"github.com/prometheus/alertmanager/notify/opsgenie"
	"github.com/prometheus/alertmanager/notify/pagerduty"
	"github.com/prometheus/alertmanager/notify/pushover"
	"github.com/prometheus/alertmanager/notify/slack"
	"github.com/prometheus/alertmanager/notify/sns"
	"github.com/prometheus/alertmanager/notify/telegram"
	"github.com/prometheus/alertmanager/notify/victorops"
	"github.com/prometheus/alertmanager/notify/webhook"
	"github.com/prometheus/alertmanager/notify/wechat"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"gopkg.in/alecthomas/kingpin.v2"
)

type testReceiversCmd struct {
	configFile string
}

const testReceiversHelp = `Test alertmanager receivers

Will test receivers for alertmanager config file.
`

func configureTestReceiversCmd(app *kingpin.Application) {
	var (
		t       = &testReceiversCmd{}
		testCmd = app.Command("test-receivers", testReceiversHelp)
	)
	testCmd.Arg("config.file", "Config file to be tested.").ExistingFileVar(&t.configFile)
	testCmd.Action(execWithTimeout(t.testReceivers))
}

func (t *testReceiversCmd) testReceivers(ctx context.Context, _ *kingpin.ParseContext) error {
	if len(t.configFile) == 0 {
		kingpin.Fatalf("No config file was specified")
	}

	fmt.Printf("Checking '%s'\n", t.configFile)
	cfg, err := config.LoadFile(t.configFile)
	if err != nil {
		return errors.Wrap(err, "invalid config file")
	}

	if cfg != nil {
		tmpl, err := template.FromGlobs(cfg.Templates...)
		if err != nil {
			return errors.Wrap(err, "failed to parse templates")
		}
		if alertmanagerURL != nil {
			tmpl.ExternalURL = alertmanagerURL
		} else {
			u, err := url.Parse("http://localhost:1234")
			if err != nil {
				return errors.Wrap(err, "failed to parse fake url")
			}
			tmpl.ExternalURL = u
		}

		return TestReceivers(ctx, cfg.Receivers, tmpl)
	}

	return nil
}

func TestReceivers(ctx context.Context, receivers []*config.Receiver, tmpl *template.Template) error {
	// now represents the start time of the test
	now := time.Now()
	testAlert := newTestAlert(now, now)

	// we must set a group key that is unique per test as some receivers use this key to deduplicate alerts
	ctx = notify.WithGroupKey(ctx, testAlert.Labels.String()+now.String())
	ctx = notify.WithGroupLabels(ctx, testAlert.Labels)

	logger := promlog.New(&promlog.Config{})

	// job contains all metadata required to test a receiver
	type job struct {
		Receiver    *config.Receiver
		Integration *notify.Integration
	}

	// result contains the receiver that was tested and an error that is non-nil if the test failed
	type result struct {
		Receiver    *config.Receiver
		Integration *notify.Integration
		Error       error
	}

	// invalid keeps track of all invalid receiver configurations
	var invalid []result
	// jobs keeps track of all receivers that need to be sent test notifications
	var jobs []job

	for _, receiver := range receivers {
		integrations, err := buildReceiverIntegrations(receiver, tmpl, logger)
		for _, integration := range integrations {
			if err != nil {
				invalid = append(invalid, result{
					Receiver:    receiver,
					Integration: &integration,
					Error:       err,
				})
			} else {
				jobs = append(jobs, job{
					Receiver:    receiver,
					Integration: &integration,
				})
			}
		}
	}

	fmt.Printf("Performing %v jobs!\n", len(jobs))

	for _, job := range jobs {
		v := result{
			Receiver:    job.Receiver,
			Integration: job.Integration,
		}
		if _, err := job.Integration.Notify(notify.WithReceiverName(ctx, job.Receiver.Name), &testAlert); err != nil {
			v.Error = err
		}
	}

	fmt.Printf("Done!\n")
	return nil
}

func newTestAlert(startsAt, updatedAt time.Time) types.Alert {
	var (
		defaultAnnotations = model.LabelSet{
			"summary":          "Notification test",
			"__value_string__": "[ metric='foo' labels={instance=bar} value=10 ]",
		}
		defaultLabels = model.LabelSet{
			"alertname": "TestAlert",
			"instance":  "Alertmanager",
		}
	)

	alert := types.Alert{
		Alert: model.Alert{
			Labels:      defaultLabels,
			Annotations: defaultAnnotations,
			StartsAt:    startsAt,
		},
		UpdatedAt: updatedAt,
	}

	return alert
}

// buildReceiverIntegrations builds a list of integration notifiers off of a
// receiver config.
func buildReceiverIntegrations(nc *config.Receiver, tmpl *template.Template, logger log.Logger) ([]notify.Integration, error) {
	var (
		errs         types.MultiError
		integrations []notify.Integration
		add          = func(name string, i int, rs notify.ResolvedSender, f func(l log.Logger) (notify.Notifier, error)) {
			n, err := f(log.With(logger, "integration", name))
			if err != nil {
				errs.Add(err)
				return
			}
			integrations = append(integrations, notify.NewIntegration(n, rs, name, i))
		}
	)

	for i, c := range nc.WebhookConfigs {
		add("webhook", i, c, func(l log.Logger) (notify.Notifier, error) { return webhook.New(c, tmpl, l) })
	}
	for i, c := range nc.EmailConfigs {
		add("email", i, c, func(l log.Logger) (notify.Notifier, error) { return email.New(c, tmpl, l), nil })
	}
	for i, c := range nc.PagerdutyConfigs {
		add("pagerduty", i, c, func(l log.Logger) (notify.Notifier, error) { return pagerduty.New(c, tmpl, l) })
	}
	for i, c := range nc.OpsGenieConfigs {
		add("opsgenie", i, c, func(l log.Logger) (notify.Notifier, error) { return opsgenie.New(c, tmpl, l) })
	}
	for i, c := range nc.WechatConfigs {
		add("wechat", i, c, func(l log.Logger) (notify.Notifier, error) { return wechat.New(c, tmpl, l) })
	}
	for i, c := range nc.SlackConfigs {
		add("slack", i, c, func(l log.Logger) (notify.Notifier, error) { return slack.New(c, tmpl, l) })
	}
	for i, c := range nc.VictorOpsConfigs {
		add("victorops", i, c, func(l log.Logger) (notify.Notifier, error) { return victorops.New(c, tmpl, l) })
	}
	for i, c := range nc.PushoverConfigs {
		add("pushover", i, c, func(l log.Logger) (notify.Notifier, error) { return pushover.New(c, tmpl, l) })
	}
	for i, c := range nc.SNSConfigs {
		add("sns", i, c, func(l log.Logger) (notify.Notifier, error) { return sns.New(c, tmpl, l) })
	}
	for i, c := range nc.TelegramConfigs {
		add("telegram", i, c, func(l log.Logger) (notify.Notifier, error) { return telegram.New(c, tmpl, l) })
	}
	for i, c := range nc.DiscordConfigs {
		add("discord", i, c, func(l log.Logger) (notify.Notifier, error) { return discord.New(c, tmpl, l) })
	}

	if errs.Len() > 0 {
		return nil, &errs
	}
	return integrations, nil
}
