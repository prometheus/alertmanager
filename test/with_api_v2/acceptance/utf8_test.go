package test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
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

	// add an alert with UTF-8 labels
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

	// can get same alert from the API
	resp, err := am.Client().Alert.GetAlerts(nil)
	require.NoError(t, err)
	require.Len(t, resp.Payload, 1)
	require.Equal(t, labels, resp.Payload[0].Labels)

	// can filter alerts on UTF-8 labels
	getAlertParams := alert.NewGetAlertsParams()
	getAlertParams = getAlertParams.WithFilter([]string{"00=b", "Σ=c", "\"\\xf0\\x9f\\x99\\x82\"=dΘ"})
	resp, err = am.Client().Alert.GetAlerts(getAlertParams)
	require.NoError(t, err)
	require.Len(t, resp.Payload, 1)
	require.Equal(t, labels, resp.Payload[0].Labels)
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
		FeatureFlags: []string{"classic-matchers-parsing"},
		Tolerance:    1 * time.Second,
	})
	co := at.Collector("webhook")
	wh := NewWebhook(t, co)
	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()
	am := amc.Members()[0]

	// cannot add an alert with UTF-8 labels
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
	require.NotNil(t, err)
	require.True(t, strings.Contains(err.Error(), "invalid label set"))
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

	// add a silence with UTF-8 label matchers
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

	// can get the same silence from the API
	resp, err := am.Client().Silence.GetSilences(nil)
	require.NoError(t, err)
	require.Len(t, resp.Payload, 1)
	require.Equal(t, matchers, resp.Payload[0].Matchers)

	// can filter silences on UTF-8 label matchers
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
		FeatureFlags: []string{"classic-matchers-parsing"},
		Tolerance:    150 * time.Millisecond,
	})

	co := at.Collector("webhook")
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	// cannot create a silence with UTF-8 matchers
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
	require.NotNil(t, err)
	require.True(t, strings.Contains(err.Error(), "silence invalid: invalid label matcher"))
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
