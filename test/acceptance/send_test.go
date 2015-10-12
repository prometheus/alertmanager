// Copyright 2015 Prometheus Team
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

	. "github.com/prometheus/alertmanager/test"
)

// This file contains acceptance tests around the basic sending logic
// for notifications, which includes batching and ensuring that each
// notification is eventually sent at least once and ideally exactly
// once.

func TestRepeat(t *testing.T) {
	t.Parallel()

	conf := `
routes:
- send_to: "default"
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1s

notification_configs:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
`

	// Create a new acceptance test that instantiates new Alertmanagers
	// with the given configuration and verifies times with the given
	// tollerance.
	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
	})

	// Create a collector to which alerts can be written and verified
	// against a set of expected alert notifications.
	co := at.Collector("webhook")
	// Run something that satisfies the webhook interface to which the
	// Alertmanager pushes as defined by its configuration.
	wh := NewWebhook(co)

	// Create a new Alertmanager process listening to a random port
	am := at.Alertmanager(fmt.Sprintf(conf, wh.Address()))

	// Declare pushes to be made to the Alertmanager at the given time.
	// Times are provided in fractions of seconds.
	am.Push(At(1), Alert("alertname", "test").Active(1))

	at.Do(At(1.2), func() {
		am.Terminate()
		am.Start()
	})
	am.Push(At(3.5), Alert("alertname", "test").Active(1, 3))

	// Declare which alerts are expected to arrive at the collector within
	// the defined time intervals.
	co.Want(Between(2, 2.5), Alert("alertname", "test").Active(1))
	co.Want(Between(3, 3.5), Alert("alertname", "test").Active(1))
	co.Want(Between(4, 4.5), Alert("alertname", "test").Active(1, 3))

	// Start the flow as defined above and run the checks afterwards.
	at.Run()
}

func TestRetry(t *testing.T) {
	t.Parallel()

	// We create a notification config that fans out into two different
	// webhooks.
	// The succeeding one must still only receive the first successful
	// notifications. Sending to the succeeding one must eventually succeed.
	conf := `
routes:
- send_to: "default"
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 3s

notification_configs:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
  - url: 'http://%s'
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
	})

	co1 := at.Collector("webhook")
	wh1 := NewWebhook(co1)

	co2 := at.Collector("webhook_failing")
	wh2 := NewWebhook(co2)

	wh2.Func = func(ts float64) bool {
		// Fail the first two interval periods but eventually
		// succeed in the third interval after a few failed attempts.
		return ts < 4.5
	}

	am := at.Alertmanager(fmt.Sprintf(conf, wh1.Address(), wh2.Address()))

	am.Push(At(1), Alert("alertname", "test1"))

	co1.Want(Between(2, 2.5), Alert("alertname", "test1").Active(1))
	co1.Want(Between(5, 5.5), Alert("alertname", "test1").Active(1))

	co2.Want(Between(4.5, 5), Alert("alertname", "test1").Active(1))
}

func TestBatching(t *testing.T) {
	t.Parallel()

	conf := `
routes:
- send_to: "default"
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 5s

notification_configs:
- name:            "default"
  webhook_configs:
  - url: 'http://%s'
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
	})

	co := at.Collector("webhook")
	wh := NewWebhook(co)

	am := at.Alertmanager(fmt.Sprintf(conf, wh.Address()))

	am.Push(At(1.1), Alert("alertname", "test1").Active(1))
	am.Push(At(1.9), Alert("alertname", "test5").Active(1))
	am.Push(At(2.3),
		Alert("alertname", "test2").Active(1.5),
		Alert("alertname", "test3").Active(1.5),
		Alert("alertname", "test4").Active(1.6),
	)

	co.Want(Between(2.0, 2.5),
		Alert("alertname", "test1").Active(1),
		Alert("alertname", "test5").Active(1),
	)
	// Only expect the new ones with the next group interval.
	co.Want(Between(3, 3.5),
		Alert("alertname", "test2").Active(1.5),
		Alert("alertname", "test3").Active(1.5),
		Alert("alertname", "test4").Active(1.6),
	)

	// While no changes happen expect no additional notifications
	// until the 5s repeat interval has ended.

	// The last three notifications should sent with the first two even
	// though their repeat interval has not yet passed. This way fragmented
	// batches are unified and notification noise reduced.
	co.Want(Between(7, 7.5),
		Alert("alertname", "test1").Active(1),
		Alert("alertname", "test5").Active(1),
		Alert("alertname", "test2").Active(1.5),
		Alert("alertname", "test3").Active(1.5),
		Alert("alertname", "test4").Active(1.6),
	)

	at.Run()
}
