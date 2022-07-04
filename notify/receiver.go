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

package notify

type Receiver struct {
	name         string
	integrations []Integration

	// A receiver is considered active if a route is using it.
	active bool
}

func (r *Receiver) Name() string {
	return r.name
}

func (r *Receiver) Active() bool {
	return r.active
}

func (r *Receiver) Integrations() []Integration {
	return r.integrations
}

func NewReceiver(name string, active bool, integrations []Integration) *Receiver {
	return &Receiver{
		name:         name,
		active:       active,
		integrations: integrations,
	}
}

//func NewReceivers(conf []*config.Receiver, activeReceivers map[string]struct{}, tmpl *template.Template, logger log.Logger) ([]*Receiver, error) {
//	//  Build the map of receiver to integrations.
//	receivers := make([]*Receiver, len(activeReceivers))
//
//	for _, rcv := range conf {
//		r := &Receiver{name: rcv.Name}
//		if _, found := activeReceivers[rcv.Name]; !found {
//			// No need to build a receiver if no route is using it.
//			level.Info(logger).Log("msg", "skipping creation of receiver not referenced by any route", "receiver", rcv.Name)
//			r.active = false
//			receivers = append(receivers, r)
//			continue
//		}
//
//		var err error
//		r.integrations, err = buildReceiverIntegrations(rcv, tmpl, logger)
//		if err != nil {
//			return nil, err
//		}
//
//		receivers = append(receivers, r)
//	}
//
//	return receivers, nil
//}

//
//// buildReceiverIntegrations builds a list of integration notifiers off of a
//// receiver config.
//func buildReceiverIntegrations(nc *config.Receiver, tmpl *template.Template, logger log.Logger) ([]Integration, error) {
//	var (
//		errs         types.MultiError
//		integrations []Integration
//		add          = func(name string, i int, rs ResolvedSender, f func(l log.Logger) (Notifier, error)) {
//			n, err := f(log.With(logger, "integration", name))
//			if err != nil {
//				errs.Add(err)
//				return
//			}
//			integrations = append(integrations, NewIntegration(n, rs, name, i))
//		}
//	)
//
//	for i, c := range nc.WebhookConfigs {
//		add("webhook", i, c, func(l log.Logger) (Notifier, error) { return webhook.New(c, tmpl, l) })
//	}
//	for i, c := range nc.EmailConfigs {
//		add("email", i, c, func(l log.Logger) (Notifier, error) { return email.New(c, tmpl, l), nil })
//	}
//	for i, c := range nc.PagerdutyConfigs {
//		add("pagerduty", i, c, func(l log.Logger) (Notifier, error) { return pagerduty.New(c, tmpl, l) })
//	}
//	for i, c := range nc.OpsGenieConfigs {
//		add("opsgenie", i, c, func(l log.Logger) (Notifier, error) { return opsgenie.New(c, tmpl, l) })
//	}
//	for i, c := range nc.WechatConfigs {
//		add("wechat", i, c, func(l log.Logger) (Notifier, error) { return wechat.New(c, tmpl, l) })
//	}
//	for i, c := range nc.SlackConfigs {
//		add("slack", i, c, func(l log.Logger) (Notifier, error) { return slack.New(c, tmpl, l) })
//	}
//	for i, c := range nc.VictorOpsConfigs {
//		add("victorops", i, c, func(l log.Logger) (Notifier, error) { return victorops.New(c, tmpl, l) })
//	}
//	for i, c := range nc.PushoverConfigs {
//		add("pushover", i, c, func(l log.Logger) (Notifier, error) { return pushover.New(c, tmpl, l) })
//	}
//	for i, c := range nc.SNSConfigs {
//		add("sns", i, c, func(l log.Logger) (Notifier, error) { return sns.New(c, tmpl, l) })
//	}
//	for i, c := range nc.TelegramConfigs {
//		add("telegram", i, c, func(l log.Logger) (Notifier, error) { return telegram.New(c, tmpl, l) })
//	}
//	if errs.Len() > 0 {
//		return nil, &errs
//	}
//	return integrations, nil
//}
