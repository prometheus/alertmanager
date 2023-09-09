// Copyright 2019 Prometheus Team
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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/api/v2/models"
	. "github.com/prometheus/alertmanager/test/cli"
)

func TestMain(m *testing.M) {
	if ok, err := AmtoolOk(); !ok {
		panic("unable to access amtool binary: " + err.Error())
	}
	os.Exit(m.Run())
}

// TestAmtoolVersion checks that amtool is executable and
// is reporting valid version info.
func TestAmtoolVersion(t *testing.T) {
	t.Parallel()
	version, err := Version()
	if err != nil {
		t.Fatal("Unable to get amtool version", err)
	}
	t.Logf("testing amtool version: %v", version)
}

func TestAddAlert(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: [alertname]
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1ms

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
    send_resolved: true
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)

	am := amc.Members()[0]

	alert1 := Alert("alertname", "test1").Active(1, 2)
	am.AddAlertsAt(0, alert1)
	co.Want(Between(1, 2), Alert("alertname", "test1").Active(1))

	at.Run()

	t.Log(co.Check())
}

func TestQueryAlert(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: [alertname]
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1ms

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
    send_resolved: true
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 1 * time.Second,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	alert1 := Alert("alertname", "test1", "severity", "warning").Active(1)
	alert2 := Alert("alertname", "test2", "severity", "info").Active(1)
	am.AddAlerts(alert1, alert2)

	alerts, err := am.QueryAlerts()
	require.NoError(t, err)
	require.Len(t, alerts, 2)

	// Get the first alert using the alertname heuristic
	alerts, err = am.QueryAlerts("test1")
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	// QueryAlerts uses the simple output option, which means just the alertname
	// label is printed. We can assert that querying works as expected as we know
	// there are two alerts called "test1" and "test2".
	expectedLabels := models.LabelSet{"name": "test1"}
	require.True(t, alerts[0].HasLabels(expectedLabels))

	// Get the second alert
	alerts, err = am.QueryAlerts("alertname=test2")
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	expectedLabels = models.LabelSet{"name": "test2"}
	require.True(t, alerts[0].HasLabels(expectedLabels))
}

func TestQuerySilence(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: [alertname]
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1ms

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
    send_resolved: true
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 1 * time.Second,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	silence1 := Silence(0, 4).Match("test1", "severity=warn").Comment("test1")
	silence2 := Silence(0, 4).Match("foo").Comment("test foo")

	am.SetSilence(0, silence1)
	am.SetSilence(0, silence2)

	// Get all silences
	sils, err := am.QuerySilence()
	require.NoError(t, err)
	require.Len(t, sils, 2)

	// Get the first silence using the alertname heuristic
	sils, err = am.QuerySilence("test1")
	require.NoError(t, err)
	require.Len(t, sils, 1)
	require.Equal(t, []string{"alertname=\"test1\"", "severity=\"warn\""}, sils[0].GetMatches())
}
