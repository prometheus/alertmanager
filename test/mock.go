package test

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/alertmanager/notify"
)

type mockWebhook struct {
	collector *collector
}

func runMockWebhook(addr string, c *collector) {
	http.ListenAndServe(addr, &mockWebhook{
		collector: c,
	})
}

func (ws *mockWebhook) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	dec := json.NewDecoder(req.Body)
	defer req.Body.Close()

	var v notify.WebhookMessage
	if err := dec.Decode(&v); err != nil {
		panic(err)
	}

	ws.collector.add(v.Alerts...)
}
