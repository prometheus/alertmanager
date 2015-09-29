package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

func Build(confs []*config.NotificationConfig) map[string]Notifier {
	// Create new notifiers. If the type is not implemented yet, fallback
	// to logging notifiers.
	res := map[string]Notifier{}
	for _, nc := range confs {
		var all Notifiers
		for _, wc := range nc.WebhookConfigs {
			all = append(all, NewWebhook(wc))
		}
		for range nc.EmailConfigs {
			all = append(&LogNotifier{name: cn.Name})
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
	Status  types.AlertStatus `json:"status"`
	Alerts  []*types.Alert    `json:"alert"`
}

func (w *Webhook) Notify(ctx context.Context, alerts ...*types.Alert) error {
	msg := &WebhookMessage{
		Version: "1",
		Status:  types.AlertResolved,
		Alerts:  alerts,
	}
	for _, a := range alerts {
		if !a.Resolved() {
			msg.Status = types.AlertFiring
			break
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		return err
	}

	// TODO(fabxc): implement retrying as long as context is not canceled.
	resp, err := ctxhttp.Post(ctx, http.DefaultClient, w.URL, "application/json", &buf)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected status code %v", resp.StatusCode)
	}

	return nil
}
