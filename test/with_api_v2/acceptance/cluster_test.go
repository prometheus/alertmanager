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
	"sync"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/featurecontrol"
	a "github.com/prometheus/alertmanager/test/with_api_v2"
)

// TestClusterDeduplication tests, that in an Alertmanager cluster of 3
// instances, only one should send a notification for a given alert.
func TestClusterDeduplication(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1h

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
`

	at := a.NewAcceptanceTest(t, &a.AcceptanceOpts{
		Tolerance: 1 * time.Second,
	})
	co := at.Collector("webhook")
	wh := a.NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 3)

	amc.Push(a.At(1), a.Alert("alertname", "test1"))

	co.Want(a.Between(2, 3), a.Alert("alertname", "test1").Active(1))

	at.Run()

	t.Log(co.Check())
}

// TestClusterVSInstance compares notifications sent by Alertmanager cluster to
// notifications sent by single instance.
func TestClusterVSInstance(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: [ "alertname" ]
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1h

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
`

	acceptanceOpts := func() *a.AcceptanceOpts {
		return &a.AcceptanceOpts{
			Tolerance: 2 * time.Second,
		}
	}

	clusterSizes := []int{1, 3}

	tests := []*a.AcceptanceTest{
		a.NewAcceptanceTest(t, acceptanceOpts()),
		a.NewAcceptanceTest(t, acceptanceOpts()),
	}

	collectors := []*a.Collector{}
	amClusters := []*a.AlertmanagerCluster{}
	wg := sync.WaitGroup{}

	for i, tc := range tests {
		collectors = append(collectors, tc.Collector("webhook"))
		webhook := a.NewWebhook(t, collectors[i])

		amClusters = append(amClusters, tc.AlertmanagerCluster(fmt.Sprintf(conf, webhook.Address()), clusterSizes[i]))

		wg.Add(1)
	}

	for _, alertTime := range []float64{0, 2, 4, 6, 8} {
		for i, amc := range amClusters {
			alrt := a.Alert("alertname", fmt.Sprintf("test1-%v", alertTime))
			amc.Push(a.At(alertTime), alrt)
			collectors[i].Want(a.Between(alertTime, alertTime+5), alrt.Active(alertTime))
		}
	}

	for _, t := range tests {
		go func(t *a.AcceptanceTest) {
			t.Run()
			wg.Done()
		}(t)
	}

	wg.Wait()

	_, err := a.CompareCollectors(collectors[0], collectors[1], acceptanceOpts())
	if err != nil {
		t.Fatal(err)
	}
}

// nflogMergeConf is a single aggregation group notifying through one webhook,
// used to observe what the notification log merge does to dedup state.
const nflogMergeConf = `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1h

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
`

// pushUnevenly gives every instance alert1 and alert2, gives alert3 to all but
// the last instance, and gives alert4 only to that last instance. The last
// instance then notifies for alert4 and records an entry whose firing set omits
// alert3, which is newer than what its peers hold.
func pushUnevenly(amc *a.AlertmanagerCluster) {
	ams := amc.Members()

	amc.Push(a.At(1), a.Alert("alertname", "alert1"), a.Alert("alertname", "alert2"))

	for _, am := range ams[:len(ams)-1] {
		am.Push(a.At(4), a.Alert("alertname", "alert3"))
	}

	ams[len(ams)-1].Push(a.At(8), a.Alert("alertname", "alert4"))
}

func wantInitialNotifications(co *a.Collector) {
	co.Want(a.Between(2, 4),
		a.Alert("alertname", "alert1").Active(1),
		a.Alert("alertname", "alert2").Active(1),
	)
	co.Want(a.Between(4, 6),
		a.Alert("alertname", "alert1").Active(1),
		a.Alert("alertname", "alert2").Active(1),
		a.Alert("alertname", "alert3").Active(4),
	)
	co.Want(a.Between(9, 12),
		a.Alert("alertname", "alert1").Active(1),
		a.Alert("alertname", "alert2").Active(1),
		a.Alert("alertname", "alert4").Active(8),
	)
}

// TestClusterNflogMergeRetainsFiringAlerts covers the same sequence with
// retain-firing-alerts-on-merge enabled. The peers keep alert3, which the entry
// for alert4 lists as neither firing nor resolved, so they do not notify again.
func TestClusterNflogMergeRetainsFiringAlerts(t *testing.T) {
	t.Parallel()

	at := a.NewAcceptanceTest(t, &a.AcceptanceOpts{
		Tolerance:    2 * time.Second,
		FeatureFlags: []string{featurecontrol.FeatureRetainFiringAlertsOnMerge},
	})
	co := at.Collector("webhook")
	wh := a.NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(nflogMergeConf, wh.Address()), 3)
	pushUnevenly(amc)

	wantInitialNotifications(co)

	at.Run("--cluster.peer-timeout=1s")

	t.Log(co.Check())
}
