package integrationbuilder

import (
	"github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/email"
	"github.com/prometheus/alertmanager/notify/opsgenie"
	"github.com/prometheus/alertmanager/notify/pagerduty"
	"github.com/prometheus/alertmanager/notify/pushover"
	"github.com/prometheus/alertmanager/notify/slack"
	"github.com/prometheus/alertmanager/notify/victorops"
	"github.com/prometheus/alertmanager/notify/webhook"
	"github.com/prometheus/alertmanager/notify/wechat"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// BuildReceiverIntegrations builds a list of integration notifiers off of a
// receiver config.
func BuildReceiverIntegrations(nc *config.Receiver, tmpl *template.Template, logger log.Logger) ([]notify.Integration, error) {
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
	if errs.Len() > 0 {
		return nil, &errs
	}
	return integrations, nil
}
