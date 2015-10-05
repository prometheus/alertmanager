package test

import (
	"fmt"
	"testing"
	"time"

	. "github.com/prometheus/alertmanager/test"
)

func TestInhibiting(t *testing.T) {
	t.Parallel()

	conf := `
routes:
- send_to: "default"
  group_wait:     1s
  group_interval: 1s

notification_configs:
- name: "default"
  send_resolved: true
  repeat_interval: 1s
  webhook_configs:
  - url: 'http://%s'

inhibit_rules:
- source_match:
    alertname: JobDown
  target_match:
    alertname: InstanceDown
  equal:
    - job
    - zone
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
	})

	co := at.Collector("webhook")
	wh := NewWebhook(co)

	am := at.Alertmanager(fmt.Sprintf(conf, wh.Address()))

	am.Push(At(1), Alert("alertname", "test1", "job", "testjob", "zone", "aa"))
	am.Push(At(1), Alert("alertname", "InstanceDown", "job", "testjob", "zone", "aa"))
	am.Push(At(1), Alert("alertname", "InstanceDown", "job", "testjob", "zone", "ab"))

	// This JobDown in zone aa should inhibit InstanceDown in zone aa in the
	// second batch of notifications.
	am.Push(At(2.2), Alert("alertname", "JobDown", "job", "testjob", "zone", "aa"))

	co.Want(Between(2, 2.5),
		Alert("alertname", "test1", "job", "testjob", "zone", "aa"),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "aa"),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "ab"),
	)

	co.Want(Between(3, 3.5),
		Alert("alertname", "test1", "job", "testjob", "zone", "aa"),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "ab"),
		Alert("alertname", "JobDown", "job", "testjob", "zone", "aa"),
	)

	at.Run()
}
