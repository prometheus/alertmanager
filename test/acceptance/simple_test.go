package test

import (
	"testing"
	"time"

	. "github.com/prometheus/alertmanager/test"
)

var somethingConfig = `
routes:
- send_to: "default"
  group_wait:     1s
  group_interval: 1s

notification_configs:
- name: "default"
  send_resolved: true

  webhook_configs:
  - url: 'http://localhost:8088'
`

func TestSomething(t *testing.T) {
	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
		Config:    somethingConfig,
	})

	am := at.Alertmanager()
	co := at.Collector("webhook")

	go NewWebhook(":8088", co).Run()

	am.Push(At(1), Alert("alertname", "test").Active(1))
	am.Push(At(3.5), Alert("alertname", "test").Active(1, 3))

	co.Want(Between(2, 2.5), Alert("alertname", "test").Active(1))
	co.Want(Between(3, 3.5), Alert("alertname", "test").Active(1))
	co.Want(Between(3.5, 4.5), Alert("alertname", "test").Active(1, 3))

	at.Run()
}
