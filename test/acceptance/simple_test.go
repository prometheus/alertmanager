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
	// Create a new acceptance test that instantiates new Alertmanagers
	// with the given configuration and verifies times with the given
	// tollerance.
	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
		Config:    somethingConfig,
	})

	// Create a new Alertmanager process listening to a random port
	am := at.Alertmanager()
	// Create a collector to which alerts can be written and verified
	// against a set of expected alert notifications.
	co := at.Collector("webhook")

	// Run something that satisfies the webhook interface to which the
	// Alertmanager pushes as defined by its configuration.
	go NewWebhook(":8088", co).Run()

	// Declare pushes to be made to the Alertmanager at the given time.
	// Times are provided in fractions of seconds.
	am.Push(At(1), Alert("alertname", "test").Active(1))
	am.Push(At(3.5), Alert("alertname", "test").Active(1, 3))

	// Declare which alerts are expected to arrive at the collector within
	// the defined time intervals.
	co.Want(Between(2, 2.5), Alert("alertname", "test").Active(1))
	co.Want(Between(3, 3.5), Alert("alertname", "test").Active(1))
	co.Want(Between(3.5, 4.5), Alert("alertname", "test").Active(1, 3))

	// Start the flow as defined above and run the checks afterwards.
	at.Run()
}
