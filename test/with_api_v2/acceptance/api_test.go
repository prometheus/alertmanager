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

	"github.com/go-openapi/strfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/featurecontrol"
	a "github.com/prometheus/alertmanager/test/with_api_v2"
)

func TestAddAlerts(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  10m
  repeat_interval: 1h
receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
`

	at := a.NewAcceptanceTest(t, &a.AcceptanceOpts{
		FeatureFlags: []string{featurecontrol.FeatureClassicMode},
		Tolerance:    1 * time.Second,
	})
	co := at.Collector("webhook")
	wh := a.NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	now := time.Now()
	pa := &models.PostableAlert{
		StartsAt: strfmt.DateTime(now),
		EndsAt:   strfmt.DateTime(now.Add(5 * time.Minute)),
		Alert: models.Alert{
			Labels: models.LabelSet{
				"a": "b",
				"b": "Σ",
				"c": "\xf0\x9f\x99\x82",
				"d": "eΘ",
			},
		},
	}
	alertParams := alert.NewPostAlertsParams()
	alertParams.Alerts = models.PostableAlerts{pa}

	_, err := am.Client().Alert.PostAlerts(alertParams)
	require.NoError(t, err)
}

// TestAlertGetReturnsCurrentStatus checks that querying the API returns the
// current status of each alert, i.e. if it is silenced or inhibited.
func TestAlertGetReturnsCurrentAlertStatus(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  10m
  repeat_interval: 1h

inhibit_rules:
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname']

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

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	labelName := "alertname"
	labelValue := "test1"

	now := time.Now()
	startsAt := strfmt.DateTime(now)
	endsAt := strfmt.DateTime(now.Add(5 * time.Minute))

	labels := models.LabelSet(map[string]string{labelName: labelValue, "severity": "warning"})
	fp := model.LabelSet{model.LabelName(labelName): model.LabelValue(labelValue), "severity": "warning"}.Fingerprint()
	pa := &models.PostableAlert{
		StartsAt: startsAt,
		EndsAt:   endsAt,
		Alert:    models.Alert{Labels: labels},
	}
	alertParams := alert.NewPostAlertsParams()
	alertParams.Alerts = models.PostableAlerts{pa}
	_, err := am.Client().Alert.PostAlerts(alertParams)
	require.NoError(t, err)

	resp, err := am.Client().Alert.GetAlerts(nil)
	require.NoError(t, err)
	// No silence has been created or inhibiting alert sent, alert should
	// be active.
	for _, al := range resp.Payload {
		require.Equal(t, models.AlertStatusStateActive, *al.Status.State)
	}

	// Wait for group_wait, so that we are in the group_interval period,
	// when the pipeline won't update the alert's status.
	time.Sleep(2 * time.Second)

	// Create silence and verify that the alert is immediately marked
	// silenced via the API.
	silenceParams := silence.NewPostSilencesParams()

	cm := "a"
	isRegex := false
	ps := &models.PostableSilence{
		Silence: models.Silence{
			StartsAt:  &startsAt,
			EndsAt:    &endsAt,
			Comment:   &cm,
			CreatedBy: &cm,
			Matchers: models.Matchers{
				&models.Matcher{Name: &labelName, Value: &labelValue, IsRegex: &isRegex},
			},
		},
	}
	silenceParams.Silence = ps
	silenceResp, err := am.Client().Silence.PostSilences(silenceParams)
	require.NoError(t, err)
	silenceID := silenceResp.Payload.SilenceID

	resp, err = am.Client().Alert.GetAlerts(nil)
	require.NoError(t, err)
	for _, al := range resp.Payload {
		require.Equal(t, models.AlertStatusStateSuppressed, *al.Status.State)
		require.Equal(t, fp.String(), *al.Fingerprint)
		require.Len(t, al.Status.SilencedBy, 1)
		require.Equal(t, silenceID, al.Status.SilencedBy[0])
	}

	// Create inhibiting alert and verify that original alert is
	// immediately marked as inhibited.
	labels["severity"] = "critical"
	_, err = am.Client().Alert.PostAlerts(alertParams)
	require.NoError(t, err)

	inhibitingFP := model.LabelSet{model.LabelName(labelName): model.LabelValue(labelValue), "severity": "critical"}.Fingerprint()

	resp, err = am.Client().Alert.GetAlerts(nil)
	require.NoError(t, err)
	for _, al := range resp.Payload {
		require.Len(t, al.Status.SilencedBy, 1)
		require.Equal(t, silenceID, al.Status.SilencedBy[0])
		if fp.String() == *al.Fingerprint {
			require.Equal(t, models.AlertStatusStateSuppressed, *al.Status.State)
			require.Equal(t, fp.String(), *al.Fingerprint)
			require.Len(t, al.Status.InhibitedBy, 1)
			require.Equal(t, inhibitingFP.String(), al.Status.InhibitedBy[0])
		}
	}

	deleteParams := silence.NewDeleteSilenceParams().WithSilenceID(strfmt.UUID(silenceID))
	_, err = am.Client().Silence.DeleteSilence(deleteParams)
	require.NoError(t, err)

	resp, err = am.Client().Alert.GetAlerts(nil)
	require.NoError(t, err)
	// Silence has been deleted, inhibiting alert should be active.
	// Original alert should still be inhibited.
	for _, al := range resp.Payload {
		require.Empty(t, al.Status.SilencedBy)
		if inhibitingFP.String() == *al.Fingerprint {
			require.Equal(t, models.AlertStatusStateActive, *al.Status.State)
		} else {
			require.Equal(t, models.AlertStatusStateSuppressed, *al.Status.State)
		}
	}
}

func TestFilterAlertRequest(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  10m
  repeat_interval: 1h

inhibit_rules:
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname']

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

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	now := time.Now()
	startsAt := strfmt.DateTime(now)
	endsAt := strfmt.DateTime(now.Add(5 * time.Minute))

	labels := models.LabelSet(map[string]string{"alertname": "test1", "severity": "warning"})
	pa1 := &models.PostableAlert{
		StartsAt: startsAt,
		EndsAt:   endsAt,
		Alert:    models.Alert{Labels: labels},
	}
	labels = models.LabelSet(map[string]string{"system": "foo", "severity": "critical"})
	pa2 := &models.PostableAlert{
		StartsAt: startsAt,
		EndsAt:   endsAt,
		Alert:    models.Alert{Labels: labels},
	}
	alertParams := alert.NewPostAlertsParams()
	alertParams.Alerts = models.PostableAlerts{pa1, pa2}
	_, err := am.Client().Alert.PostAlerts(alertParams)
	require.NoError(t, err)

	filter := []string{"alertname=test1", "severity=warning"}
	resp, err := am.Client().Alert.GetAlerts(alert.NewGetAlertsParams().WithFilter(filter))
	require.NoError(t, err)
	require.Len(t, resp.Payload, 1)
	for _, al := range resp.Payload {
		require.Equal(t, models.AlertStatusStateActive, *al.Status.State)
	}
}
