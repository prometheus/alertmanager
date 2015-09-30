package test

import (
	"testing"
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

func TestSomething(T *testing.T) {
	t := NewE2ETest(T, &E2ETestOpts{
		timeFactor: 1,
		tolerance:  0.2,
		conf:       somethingConfig,
	})

	am := t.alertmanager()
	co := t.collector("webhook")

	go runMockWebhook(":8088", co)

	am.push(at(1), alert("alertname", "test").active(1))
	am.push(at(3.5), alert("alertname", "test").active(1, 3))

	co.want(between(2, 2.5), alert("alertname", "test").active(1))
	co.want(between(3, 3.5), alert("alertname", "test").active(1))
	co.want(between(3.5, 4.5), alert("alertname", "test").active(1, 3))

	t.Run()
}
