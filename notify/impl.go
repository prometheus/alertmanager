package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

const contentTypeJSON = "application/json"

func Build(confs []*config.NotificationConfig) map[string]Notifier {
	// Create new notifiers. If the type is not implemented yet, fallback
	// to logging notifiers.
	res := map[string]Notifier{}
	for _, nc := range confs {
		var all Notifiers

		for _, wc := range nc.WebhookConfigs {
			all = append(all, &LogNotifier{
				Log:      log.With("notifier", "webhook"),
				Notifier: NewWebhook(wc),
			})
		}
		for range nc.EmailConfigs {
			all = append(all, &LogNotifier{Log: log.With("name", nc.Name)})
		}

		res[nc.Name] = all
	}
	return res
}

type Webhook struct {
	URL string
}

func NewWebhook(conf *config.WebhookConfig) *Webhook {
	return &Webhook{URL: conf.URL}
}

type WebhookMessage struct {
	Version string            `json:"version"`
	Status  model.AlertStatus `json:"status"`
	Alerts  model.Alerts      `json:"alert"`
}

func (w *Webhook) Notify(ctx context.Context, alerts ...*types.Alert) error {
	as := types.Alerts(alerts...)

	msg := &WebhookMessage{
		Version: "1",
		Status:  as.Status(),
		Alerts:  as,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return err
	}

	// TODO(fabxc): implement retrying as long as context is not canceled.
	resp, err := ctxhttp.Post(ctx, http.DefaultClient, w.URL, contentTypeJSON, &buf)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	return nil
}
