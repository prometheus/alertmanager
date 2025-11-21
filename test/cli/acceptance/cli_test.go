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
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)

	am := amc.Members()[0]

	alert1 := Alert("alertname", "test1").Active(1, 2)
	am.AddAlertsAt(false, 0, alert1)
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	alert1 := Alert("alertname", "test1", "severity", "warning").Active(1)
	alert2 := Alert("alertname", "alertname=test2", "severity", "info").Active(1)
	alert3 := Alert("alertname", "{alertname=test3}", "severity", "info").Active(1)
	am.AddAlerts(true, alert1, alert2, alert3)

	alerts, err := am.QueryAlerts()
	require.NoError(t, err)
	require.Len(t, alerts, 3)

	// Get the first alert using the alertname heuristic
	alerts, err = am.QueryAlerts("test1")
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	// QueryAlerts uses the simple output option, which means just the alertname
	// label is printed. We can assert that querying works as expected as we know
	// there are two alerts called "test1" and "test2".
	expectedLabels := models.LabelSet{"alertname": "test1"}
	require.True(t, alerts[0].HasLabels(expectedLabels))

	// Get the second alert
	alerts, err = am.QueryAlerts("alertname=test2")
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	expectedLabels = models.LabelSet{"alertname": "test2"}
	require.True(t, alerts[0].HasLabels(expectedLabels))

	// Get the third alert
	alerts, err = am.QueryAlerts("{alertname=test3}")
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	expectedLabels = models.LabelSet{"alertname": "{alertname=test3}"}
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	silence1 := Silence(0, 4).Match("test1", "severity=warn").Comment("test1")
	silence2 := Silence(0, 4).Match("alertname=test2", "severity=warn").Comment("test2")
	silence3 := Silence(0, 4).Match("{alertname=test3}", "severity=warn").Comment("test3")

	am.SetSilence(0, silence1)
	am.SetSilence(0, silence2)
	am.SetSilence(0, silence3)

	// Get all silences
	sils, err := am.QuerySilence()
	require.NoError(t, err)
	require.Len(t, sils, 3)
	expected1 := []string{"alertname=\"test1\"", "severity=\"warn\""}
	require.Equal(t, expected1, sils[0].GetMatches())
	expected2 := []string{"alertname=\"test2\"", "severity=\"warn\""}
	require.Equal(t, expected2, sils[1].GetMatches())
	expected3 := []string{"alertname=\"{alertname=test3}\"", "severity=\"warn\""}
	require.Equal(t, expected3, sils[2].GetMatches())

	// Get the first silence using the alertname heuristic
	sils, err = am.QuerySilence("test1")
	require.NoError(t, err)
	require.Len(t, sils, 1)
	require.Equal(t, expected1, sils[0].GetMatches())
}

func TestRoutesShow(t *testing.T) {
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]
	_, err := am.ShowRoute()
	require.NoError(t, err)
}

func TestRoutesTest(t *testing.T) {
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]
	_, err := am.TestRoute()
	require.NoError(t, err)

	// Bad labels should return error
	out, err := am.TestRoute("{foo=bar}")
	require.EqualError(t, err, "exit status 1")
	require.Equal(t, "amtool: error: Failed to parse labels: unexpected open or close brace: {foo=bar}\n\n", string(out))
}

func TestSilenceImport(t *testing.T) {
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	// Add some test silences
	silence1 := Silence(0, 4).Match("alertname=test1", "severity=warning").Comment("test silence 1")
	silence2 := Silence(0, 4).Match("alertname=test2", "severity=critical").Comment("test silence 2")

	am.SetSilence(0, silence1)
	am.SetSilence(0, silence2)

	// Export silences to JSON file
	tmpDir := t.TempDir()
	exportFile := tmpDir + "/silences.json"

	exportOut, err := am.ExportSilences()
	require.NoError(t, err)

	// Write to file
	err = os.WriteFile(exportFile, exportOut, 0o644)
	require.NoError(t, err)

	// Query current silences to get their IDs, then expire them
	sils, err := am.QuerySilence()
	require.NoError(t, err)
	require.Len(t, sils, 2)
	silIDs := make([]string, 0, len(sils))

	// Expire all silences by ID
	for _, sil := range sils {
		id := sil.ID()
		_, err := am.ExpireSilenceByID(id)
		require.NoError(t, err)
		silIDs = append(silIDs, id)
	}

	// Verify silences show as expired
	sils, err = am.QueryExpiredSilence()
	require.NoError(t, err)
	// Silences should still be queryable but in expired state
	require.Len(t, sils, 2, "expired silences should still be queryable")
	// Check that the silences are actually expired (endsAt is in the past or equal to now)
	now := float64(time.Now().Unix())
	for _, sil := range sils {
		require.Contains(t, silIDs, sil.ID(), "silence ID should be in the expired list")
		require.LessOrEqual(t, sil.EndsAt(), now, "silence %s should be expired", sil.ID())
	}

	// Import silences back
	importOut, err := am.ImportSilences(exportFile)
	require.NoError(t, err, "import failed: %s", string(importOut))

	// Verify silences were imported
	sils, err = am.QuerySilence()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(sils), 2, "expected at least 2 silences after import")
}

func TestSilenceImportInvalidJSON(t *testing.T) {
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
	wh := NewWebhook(t, co)

	amc := at.AlertmanagerCluster(fmt.Sprintf(conf, wh.Address()), 1)
	require.NoError(t, amc.Start())
	defer amc.Terminate()

	am := amc.Members()[0]

	// Create file with invalid JSON
	tmpDir := t.TempDir()
	invalidFile := tmpDir + "/invalid.json"
	err := os.WriteFile(invalidFile, []byte(`[{"broken": "json"`), 0o644)
	require.NoError(t, err)

	// Try to import - should fail
	out, err := am.ImportSilences(invalidFile)
	require.Error(t, err, "import should fail with invalid JSON")
	require.Contains(t, string(out), "couldn't unmarshal", "error message should mention JSON parsing")
}

func TestSilenceImportInvalidSilence(t *testing.T) {
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

	// Create file with valid JSON but invalid silence (zero timestamps)
	tmpDir := t.TempDir()
	invalidFile := tmpDir + "/invalid_silence.json"
	invalidSilence := `[
	{
		"matchers": [
			{"name": "alertname", "value": "test", "isRegex": false}
		],
		"startsAt": "0001-01-01T00:00:00.000Z",
		"endsAt": "0001-01-01T00:00:00.000Z",
		"createdBy": "test",
		"comment": "invalid silence with zero timestamps"
	}
]`
	err := os.WriteFile(invalidFile, []byte(invalidSilence), 0o644)
	require.NoError(t, err)

	// Try to import - should fail with error from addSilenceWorker
	out, err := am.ImportSilences(invalidFile)
	require.Error(t, err, "import should fail with invalid silence")
	require.Contains(t, string(out), "couldn't import 1 out of 1 silences", "error message should report exact count")
}

func TestSilenceImportPartialFailure(t *testing.T) {
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

	// Create array of PostableSilence directly
	now := time.Now()
	future := now.Add(4 * time.Hour)
	silences := []models.PostableSilence{
		// Valid silence 1
		{
			Silence: models.Silence{
				Matchers: models.Matchers{
					&models.Matcher{Name: ptrString("alertname"), Value: ptrString("test1"), IsRegex: ptrBool(false)},
				},
				StartsAt:  ptrTime(now),
				EndsAt:    ptrTime(future),
				CreatedBy: ptrString("test"),
				Comment:   ptrString("valid silence 1"),
			},
		},
		// Invalid silence 2 (endsAt before startsAt)
		{
			Silence: models.Silence{
				Matchers: models.Matchers{
					&models.Matcher{Name: ptrString("alertname"), Value: ptrString("test2"), IsRegex: ptrBool(false)},
				},
				StartsAt:  ptrTime(future), // Swapped!
				EndsAt:    ptrTime(now),    // Swapped!
				CreatedBy: ptrString("test"),
				Comment:   ptrString("invalid silence 2"),
			},
		},
		// Valid silence 3
		{
			Silence: models.Silence{
				Matchers: models.Matchers{
					&models.Matcher{Name: ptrString("alertname"), Value: ptrString("test3"), IsRegex: ptrBool(false)},
				},
				StartsAt:  ptrTime(now),
				EndsAt:    ptrTime(future),
				CreatedBy: ptrString("test"),
				Comment:   ptrString("valid silence 3"),
			},
		},
		// Invalid silence 4 (endsAt before startsAt)
		{
			Silence: models.Silence{
				Matchers: models.Matchers{
					&models.Matcher{Name: ptrString("alertname"), Value: ptrString("test4"), IsRegex: ptrBool(false)},
				},
				StartsAt:  ptrTime(future), // Swapped!
				EndsAt:    ptrTime(now),    // Swapped!
				CreatedBy: ptrString("test"),
				Comment:   ptrString("invalid silence 4"),
			},
		},
		// Valid silence 5
		{
			Silence: models.Silence{
				Matchers: models.Matchers{
					&models.Matcher{Name: ptrString("alertname"), Value: ptrString("test5"), IsRegex: ptrBool(false)},
				},
				StartsAt:  ptrTime(now),
				EndsAt:    ptrTime(future),
				CreatedBy: ptrString("test"),
				Comment:   ptrString("valid silence 5"),
			},
		},
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(silences)
	require.NoError(t, err)

	// Write to file
	tmpDir := t.TempDir()
	mixedFile := tmpDir + "/mixed_silences.json"
	err = os.WriteFile(mixedFile, jsonData, 0o644)
	require.NoError(t, err)

	// Try to import - should partially succeed
	out, err := am.ImportSilences(mixedFile)
	require.Error(t, err, "import should fail with partial import")
	require.Contains(t, string(out), "couldn't import 2 out of 5 silences", "error message should report 2 failures out of 5")
}

func ptrString(s string) *string { return &s }
func ptrBool(b bool) *bool       { return &b }
func ptrTime(t time.Time) *strfmt.DateTime {
	st := strfmt.DateTime(t)
	return &st
}
