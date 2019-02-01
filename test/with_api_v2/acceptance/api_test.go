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
	a "github.com/prometheus/alertmanager/test/with_api_v2"
	"github.com/prometheus/alertmanager/test/with_api_v2/api_v2_client/client/alert"
	"github.com/prometheus/alertmanager/test/with_api_v2/api_v2_client/client/silence"
	"github.com/prometheus/alertmanager/test/with_api_v2/api_v2_client/models"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

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
	wh := a.NewWebhook(co)

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
	for _, alert := range resp.Payload {
		require.Equal(t, models.AlertStatusStateActive, *alert.Status.State)
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
	for _, alert := range resp.Payload {
		require.Equal(t, models.AlertStatusStateSuppressed, *alert.Status.State)
		require.Equal(t, fp.String(), *alert.Fingerprint)
		require.Equal(t, 1, len(alert.Status.SilencedBy))
		require.Equal(t, silenceID, alert.Status.SilencedBy[0])
	}

	// Create inhibiting alert and verify that original alert is
	// immediately marked as inhibited.
	labels["severity"] = "critical"
	_, err = am.Client().Alert.PostAlerts(alertParams)
	require.NoError(t, err)

	inhibitingFP := model.LabelSet{model.LabelName(labelName): model.LabelValue(labelValue), "severity": "critical"}.Fingerprint()

	resp, err = am.Client().Alert.GetAlerts(nil)
	require.NoError(t, err)
	for _, alert := range resp.Payload {
		require.Equal(t, 1, len(alert.Status.SilencedBy))
		require.Equal(t, silenceID, alert.Status.SilencedBy[0])
		if fp.String() == *alert.Fingerprint {
			require.Equal(t, models.AlertStatusStateSuppressed, *alert.Status.State)
			require.Equal(t, fp.String(), *alert.Fingerprint)
			require.Equal(t, 1, len(alert.Status.InhibitedBy))
			require.Equal(t, inhibitingFP.String(), alert.Status.InhibitedBy[0])
		}
	}
}
