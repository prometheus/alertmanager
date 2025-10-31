// Copyright 2018 Prometheus Team
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

package test

import (
	"fmt"
	"testing"
	"time"

	. "github.com/prometheus/alertmanager/test/with_api_v2"
)

func TestInhibiting(t *testing.T) {
	t.Parallel()

	// This integration test checks that alerts can be inhibited and that an
	// inhibited alert will be notified again as soon as the inhibiting alert
	// gets resolved.

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1s

receivers:
- name: "default"
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)

	amc.Push(At(1), Alert("alertname", "test1", "job", "testjob", "zone", "aa"))
	amc.Push(At(1), Alert("alertname", "InstanceDown", "job", "testjob", "zone", "aa"))
	amc.Push(At(1), Alert("alertname", "InstanceDown", "job", "testjob", "zone", "ab"))

	// This JobDown in zone aa should inhibit InstanceDown in zone aa in the
	// second batch of notifications.
	amc.Push(At(2.2), Alert("alertname", "JobDown", "job", "testjob", "zone", "aa"))

	// InstanceDown in zone aa should fire again in the third batch of
	// notifications once JobDown in zone aa gets resolved.
	amc.Push(At(3.6), Alert("alertname", "JobDown", "job", "testjob", "zone", "aa").Active(2.2, 3.6))

	co.Want(Between(2, 2.5),
		Alert("alertname", "test1", "job", "testjob", "zone", "aa").Active(1),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "aa").Active(1),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "ab").Active(1),
	)

	co.Want(Between(3, 3.5),
		Alert("alertname", "test1", "job", "testjob", "zone", "aa").Active(1),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "ab").Active(1),
		Alert("alertname", "JobDown", "job", "testjob", "zone", "aa").Active(2.2),
	)

	co.Want(Between(4, 4.5),
		Alert("alertname", "test1", "job", "testjob", "zone", "aa").Active(1),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "aa").Active(1),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "ab").Active(1),
		Alert("alertname", "JobDown", "job", "testjob", "zone", "aa").Active(2.2, 3.6),
	)

	at.Run()

	t.Log(co.Check())
}

func TestAlwaysInhibiting(t *testing.T) {
	t.Parallel()

	// This integration test checks that when inhibited and inhibiting alerts
	// gets resolved at the same time, the final notification contains both
	// alerts.

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1s

receivers:
- name: "default"
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)

	amc.Push(At(1), Alert("alertname", "InstanceDown", "job", "testjob", "zone", "aa"))
	amc.Push(At(1), Alert("alertname", "JobDown", "job", "testjob", "zone", "aa"))

	amc.Push(At(2.6), Alert("alertname", "JobDown", "job", "testjob", "zone", "aa").Active(1, 2.6))
	amc.Push(At(2.6), Alert("alertname", "InstanceDown", "job", "testjob", "zone", "aa").Active(1, 2.6))

	co.Want(Between(2, 2.5),
		Alert("alertname", "JobDown", "job", "testjob", "zone", "aa").Active(1),
	)

	co.Want(Between(3, 3.5),
		Alert("alertname", "InstanceDown", "job", "testjob", "zone", "aa").Active(1, 2.6),
		Alert("alertname", "JobDown", "job", "testjob", "zone", "aa").Active(1, 2.6),
	)

	at.Run()

	t.Log(co.Check())
}

func TestEmptyInhibitionRule(t *testing.T) {
	t.Parallel()

	// This integration test checks that when we have empty inhibition rules,
	// there is no panic caused by null-pointer references.

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1s

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'

inhibit_rules:
-
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
	})

	co := at.Collector("webhook")
	wh := NewWebhook(t, co)

	at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	at.Run()

	t.Log(co.Check())
}
