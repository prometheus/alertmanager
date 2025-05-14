// Copyright 2023 Prometheus Team
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
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/client/alertgroup"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/featurecontrol"
	. "github.com/prometheus/alertmanager/test/with_api_v2"
)

func TestAddUTF8Alerts(t *testing.T) {
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

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 1 * time.Second,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(t, co)
	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()
	am := amc.Members()[0]

	// Add an alert with UTF-8 labels.
	now := time.Now()
	labels := models.LabelSet{
		"a":                "a",
		"00":               "b",
		"Σ":                "c",
		"\xf0\x9f\x99\x82": "dΘ",
	}
	pa := &models.PostableAlert{
		StartsAt: strfmt.DateTime(now),
		EndsAt:   strfmt.DateTime(now.Add(5 * time.Minute)),
		Alert:    models.Alert{Labels: labels},
	}
	postAlertParams := alert.NewPostAlertsParams()
	postAlertParams.Alerts = models.PostableAlerts{pa}
	_, err := am.Client().Alert.PostAlerts(postAlertParams)
	require.NoError(t, err)

	// Can get same alert from the API.
	resp, err := am.Client().Alert.GetAlerts(nil)
	require.NoError(t, err)
	require.Len(t, resp.Payload, 1)
	require.Equal(t, labels, resp.Payload[0].Labels)

	// Can filter alerts on UTF-8 labels.
	getAlertParams := alert.NewGetAlertsParams()
	getAlertParams = getAlertParams.WithFilter([]string{"00=b", "Σ=c", "\"\\xf0\\x9f\\x99\\x82\"=dΘ"})
	resp, err = am.Client().Alert.GetAlerts(getAlertParams)
	require.NoError(t, err)
	require.Len(t, resp.Payload, 1)
	require.Equal(t, labels, resp.Payload[0].Labels)

	// Can get same alert in alert group from the API.
	alertGroupResp, err := am.Client().Alertgroup.GetAlertGroups(nil)
	require.NoError(t, err)
	require.Len(t, alertGroupResp.Payload, 1)
	require.Len(t, alertGroupResp.Payload[0].Alerts, 1)
	require.Equal(t, labels, alertGroupResp.Payload[0].Alerts[0].Labels)

	// Can filter alertGroups on UTF-8 labels.
	getAlertGroupsParams := alertgroup.NewGetAlertGroupsParams()
	getAlertGroupsParams.Filter = []string{"00=b", "Σ=c", "\"\\xf0\\x9f\\x99\\x82\"=dΘ"}
	alertGroupResp, err = am.Client().Alertgroup.GetAlertGroups(getAlertGroupsParams)
	require.NoError(t, err)
	require.Len(t, alertGroupResp.Payload, 1)
	require.Len(t, alertGroupResp.Payload[0].Alerts, 1)
	require.Equal(t, labels, alertGroupResp.Payload[0].Alerts[0].Labels)
}

func TestCannotAddUTF8AlertsInClassicMode(t *testing.T) {
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

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		FeatureFlags: []string{featurecontrol.FeatureClassicMode},
		Tolerance:    1 * time.Second,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(t, co)
	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()
	am := amc.Members()[0]

	// Cannot add an alert with UTF-8 labels.
	now := time.Now()
	pa := &models.PostableAlert{
		StartsAt: strfmt.DateTime(now),
		EndsAt:   strfmt.DateTime(now.Add(5 * time.Minute)),
		Alert: models.Alert{
			Labels: models.LabelSet{
				"a":                "a",
				"00":               "b",
				"Σ":                "c",
				"\xf0\x9f\x99\x82": "dΘ",
			},
		},
	}
	alertParams := alert.NewPostAlertsParams()
	alertParams.Alerts = models.PostableAlerts{pa}

	_, err := am.Client().Alert.PostAlerts(alertParams)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid label set")
}

func TestAddUTF8Silences(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1ms

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(t, co)
	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()
	am := amc.Members()[0]

	// Add a silence with UTF-8 label matchers.
	now := time.Now()
	matchers := models.Matchers{{
		Name:    stringPtr("fooΣ"),
		IsEqual: boolPtr(true),
		IsRegex: boolPtr(false),
		Value:   stringPtr("bar🙂"),
	}}
	ps := models.PostableSilence{
		Silence: models.Silence{
			Comment:   stringPtr("test"),
			CreatedBy: stringPtr("test"),
			Matchers:  matchers,
			StartsAt:  dateTimePtr(strfmt.DateTime(now)),
			EndsAt:    dateTimePtr(strfmt.DateTime(now.Add(24 * time.Hour))),
		},
	}
	postSilenceParams := silence.NewPostSilencesParams()
	postSilenceParams.Silence = &ps
	_, err := am.Client().Silence.PostSilences(postSilenceParams)
	require.NoError(t, err)

	// Can get the same silence from the API.
	resp, err := am.Client().Silence.GetSilences(nil)
	require.NoError(t, err)
	require.Len(t, resp.Payload, 1)
	require.Equal(t, matchers, resp.Payload[0].Matchers)

	// Can filter silences on UTF-8 label matchers.
	getSilenceParams := silence.NewGetSilencesParams()
	getSilenceParams = getSilenceParams.WithFilter([]string{"fooΣ=bar🙂"})
	resp, err = am.Client().Silence.GetSilences(getSilenceParams)
	require.NoError(t, err)
	require.Len(t, resp.Payload, 1)
	require.Equal(t, matchers, resp.Payload[0].Matchers)
}

func TestCannotAddUTF8SilencesInClassicMode(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1ms

receivers:
- name: "default"
  webhook_configs:
  - url: 'http://%s'
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		FeatureFlags: []string{featurecontrol.FeatureClassicMode},
		Tolerance:    150 * time.Millisecond,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(t, co)
	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()
	am := amc.Members()[0]

	// Cannot create a silence with UTF-8 matchers.
	now := time.Now()
	ps := models.PostableSilence{
		Silence: models.Silence{
			Comment:   stringPtr("test"),
			CreatedBy: stringPtr("test"),
			Matchers: models.Matchers{{
				Name:    stringPtr("fooΣ"),
				IsEqual: boolPtr(true),
				IsRegex: boolPtr(false),
				Value:   stringPtr("bar🙂"),
			}},
			StartsAt: dateTimePtr(strfmt.DateTime(now)),
			EndsAt:   dateTimePtr(strfmt.DateTime(now.Add(24 * time.Hour))),
		},
	}
	silenceParams := silence.NewPostSilencesParams()
	silenceParams.Silence = &ps

	_, err := am.Client().Silence.PostSilences(silenceParams)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid silence: invalid label matcher")
}

func TestSendAlertsToUTF8Route(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: default
  routes:
    - receiver: webhook
      matchers:
        - foo🙂=bar
      group_by:
        - foo🙂
      group_wait: 1s
receivers:
- name: default
- name: webhook
  webhook_configs:
  - url: 'http://%s'
`

	at := NewAcceptanceTest(t, &AcceptanceOpts{
		Tolerance: 150 * time.Millisecond,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(t, co)
	am := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)

	am.Push(At(1), Alert("foo🙂", "bar").Active(1))
	co.Want(Between(2, 2.5), Alert("foo🙂", "bar").Active(1))
	at.Run()
	t.Log(co.Check())
}
