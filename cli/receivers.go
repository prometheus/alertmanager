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
	"errors"
	"sort"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"golang.org/x/sync/errgroup"

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
)

const (
	maxTestReceiversWorkers = 10
)

var ErrNoReceivers = errors.New("no receivers with configuration set")

type TestReceiversParams struct {
	Alert     *TestReceiversAlertParams
	Receivers []config.Receiver
}

type TestReceiversAlertParams struct {
	Annotations model.LabelSet `yaml:"annotations,omitempty" json:"annotations,omitempty"`
	Labels      model.LabelSet `yaml:"labels,omitempty" json:"labels,omitempty"`
}

type TestReceiversResult struct {
	Alert     types.Alert
	Receivers []TestReceiverResult
	NotifedAt time.Time
}

type TestReceiverResult struct {
	Name          string
	ConfigResults []TestReceiverConfigResult
}

type TestReceiverConfigResult struct {
	Name   string
	Status string
	Error  error
}

func TestReceivers(ctx context.Context, c TestReceiversParams, tmpl *template.Template) (*TestReceiversResult, error) {
	// now represents the start time of the test
	now := time.Now()
	testAlert := newTestAlert(c, now, now)

	// we must set a group key that is unique per test as some receivers use this key to deduplicate alerts
	ctx = notify.WithGroupKey(ctx, testAlert.Labels.String()+now.String())
	// we must set group labels to avoid issues with templating
	ctx = notify.WithGroupLabels(ctx, testAlert.Labels)

	logger := promlog.New(&promlog.Config{})

	// job contains all metadata required to test a receiver
	type job struct {
		Receiver    config.Receiver
		Integration *notify.Integration
	}

	// result contains the receiver that was tested and an error that is non-nil if the test failed
	type result struct {
		Receiver    config.Receiver
		Integration *notify.Integration
		Error       error
	}

	newTestReceiversResult := func(alert types.Alert, results []result, notifiedAt time.Time) *TestReceiversResult {
		m := make(map[string]TestReceiverResult)
		for _, receiver := range c.Receivers {
			// set up the result for this receiver
			m[receiver.Name] = TestReceiverResult{
				Name:          receiver.Name,
				ConfigResults: []TestReceiverConfigResult{},
			}
		}
		for _, result := range results {
			tmp := m[result.Receiver.Name]
			status := "ok"
			if result.Error != nil {
				status = "failed"
			}
			tmp.ConfigResults = append(tmp.ConfigResults, TestReceiverConfigResult{
				Name:   result.Integration.Name(),
				Status: status,
				Error:  result.Error,
			})
			m[result.Receiver.Name] = tmp
		}
		v := new(TestReceiversResult)
		v.Alert = alert
		v.Receivers = make([]TestReceiverResult, 0, len(c.Receivers))
		v.NotifedAt = notifiedAt
		for _, result := range m {
			v.Receivers = append(v.Receivers, result)
		}

		// Make sure the return order is deterministic.
		sort.Slice(v.Receivers, func(i, j int) bool {
			return v.Receivers[i].Name < v.Receivers[j].Name
		})

		return v
	}

	// invalid keeps track of all invalid receiver configurations
	var invalid []result
	// jobs keeps track of all receivers that need to be sent test notifications
	var jobs []job

	for _, receiver := range c.Receivers {
		integrations := buildReceiverIntegrations(receiver, tmpl, logger)
		for _, integration := range integrations {
			if integration.Error != nil {
				invalid = append(invalid, result{
					Receiver:    receiver,
					Integration: &integration.Integration,
					Error:       integration.Error,
				})
			} else {
				jobs = append(jobs, job{
					Receiver:    receiver,
					Integration: &integration.Integration,
				})
			}
		}
	}

	if len(invalid)+len(jobs) == 0 {
		return nil, ErrNoReceivers
	}

	if len(jobs) == 0 {
		return newTestReceiversResult(testAlert, invalid, now), nil
	}

	numWorkers := maxTestReceiversWorkers
	if numWorkers > len(jobs) {
		numWorkers = len(jobs)
	}

	resultCh := make(chan result, len(jobs))
	jobCh := make(chan job, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	g, ctx := errgroup.WithContext(ctx)
	for i := 0; i < numWorkers; i++ {
		g.Go(func() error {
			for job := range jobCh {
				v := result{
					Receiver:    job.Receiver,
					Integration: job.Integration,
				}
				if _, err := job.Integration.Notify(notify.WithReceiverName(ctx, job.Receiver.Name), &testAlert); err != nil {
					v.Error = err
				}
				resultCh <- v
			}
			return nil
		})
	}
	g.Wait() // nolint
	close(resultCh)

	results := make([]result, 0, len(jobs))
	for next := range resultCh {
		results = append(results, next)
	}

	return newTestReceiversResult(testAlert, append(invalid, results...), now), nil
}

func newTestAlert(c TestReceiversParams, startsAt, updatedAt time.Time) types.Alert {
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

	if c.Alert != nil {
		if c.Alert.Annotations != nil {
			for k, v := range c.Alert.Annotations {
				alert.Annotations[k] = v
			}
		}
		if c.Alert.Labels != nil {
			for k, v := range c.Alert.Labels {
				alert.Labels[k] = v
			}
		}
	}

	return alert
}

type ReceiverIntegration struct {
	Integration notify.Integration
	Error       error
}

// buildReceiverIntegrations builds a list of integration notifiers off of a
// receiver config.
func buildReceiverIntegrations(nc config.Receiver, tmpl *template.Template, logger log.Logger) []ReceiverIntegration {
	var (
		integrations []ReceiverIntegration
		add          = func(name string, i int, rs notify.ResolvedSender, f func(l log.Logger) (notify.Notifier, error)) {
			n, err := f(log.With(logger, "integration", name))
			if err != nil {
				integrations = append(integrations, ReceiverIntegration{
					Integration: notify.NewIntegration(nil, rs, name, i),
					Error:       err,
				})
			} else {
				integrations = append(integrations, ReceiverIntegration{
					Integration: notify.NewIntegration(n, rs, name, i),
				})
			}
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

	return integrations
}
