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
