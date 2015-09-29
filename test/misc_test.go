package test

import (
	"testing"
)

var somethingConfig = `
routes:
- send_to: "default"

notification_configs:
- name: "default"
  webhook_configs:
  - url: 'http://localhost:8088'
`

func TestSomething(T *testing.T) {
	t := NewE2ETest(T, &E2ETestOpts{
		timeFactor: 1,
		conf:       somethingConfig,
	})

	t.push(at(1), alert("alertname", "test"))

	t.want(between(2, 4), alert("alertname", "test"))

	t.Run()
}
