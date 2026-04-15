// Copyright The Prometheus Authors
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

package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/types"
)

// loadTestRoute is a helper that loads a dispatch.Route from a config file.
func loadTestRoute(t *testing.T, cfgFile string) (*dispatch.Route, []amcommoncfg.InhibitRule) {
	t.Helper()
	cfg, err := config.LoadFile(cfgFile)
	require.NoError(t, err, "loading config file %s", cfgFile)
	return dispatch.NewRoute(cfg.Route, nil), cfg.InhibitRules
}

// TestMapToLabelSet verifies the label-map conversion helper.
func TestMapToLabelSet(t *testing.T) {
	m := map[string]string{"alertname": "Test", "severity": "critical"}
	ls := mapToLabelSet(m)
	require.Equal(t, model.LabelValue("Test"), ls["alertname"])
	require.Equal(t, model.LabelValue("critical"), ls["severity"])
}

// TestEqualStringSlices covers the slice-comparison helper.
func TestEqualStringSlices(t *testing.T) {
	require.True(t, equalStringSlices([]string{"a", "b"}, []string{"a", "b"}))
	require.False(t, equalStringSlices([]string{"a"}, []string{"b"}))
	require.False(t, equalStringSlices([]string{"a"}, []string{"a", "b"}))
	require.True(t, equalStringSlices(nil, nil))
	require.False(t, equalStringSlices(nil, []string{"a"}))
}

// TestLabelMapString verifies the human-readable label representation.
func TestLabelMapString(t *testing.T) {
	got := labelMapString(map[string]string{"b": "2", "a": "1"})
	// Keys are sorted.
	require.Equal(t, `{a="1", b="2"}`, got)
}

// TestBuildTestAlert checks that buildTestAlert produces an active alert.
func TestBuildTestAlert(t *testing.T) {
	now := time.Now()
	a := buildTestAlert(map[string]string{"alertname": "MyAlert"}, now)
	require.Equal(t, model.LabelValue("MyAlert"), a.Labels["alertname"])
	require.False(t, a.Resolved())
}

// TestRunRouteTestCase_RoutingPass checks basic routing assertions pass.
func TestRunRouteTestCase_RoutingPass(t *testing.T) {
	mainRoute, inhibitRules := loadTestRoute(t, "testdata/conf.routing.yml")

	tc := testFileCase{
		Name: "unmatched alert routes to default",
		Alerts: []testFileAlertDef{
			{
				Labels:            map[string]string{"alertname": "Unmatched"},
				ExpectedReceivers: []string{"default"},
			},
		},
	}

	passed, detail := runRouteTestCase(tc, mainRoute, inhibitRules)
	require.True(t, passed, "expected pass but got failure: %s", detail)
}

// TestRunRouteTestCase_RoutingFail checks that a wrong receiver triggers failure.
func TestRunRouteTestCase_RoutingFail(t *testing.T) {
	mainRoute, inhibitRules := loadTestRoute(t, "testdata/conf.routing.yml")

	tc := testFileCase{
		Name: "wrong receiver expectation",
		Alerts: []testFileAlertDef{
			{
				Labels:            map[string]string{"test": "1"},
				ExpectedReceivers: []string{"default"}, // actually routes to test1
			},
		},
	}

	passed, detail := runRouteTestCase(tc, mainRoute, inhibitRules)
	require.False(t, passed)
	require.NotEmpty(t, detail)
}

// TestRunRouteTestCase_MultipleRoutes verifies that continue:true produces
// multiple matched receivers in the correct order.
func TestRunRouteTestCase_MultipleRoutes(t *testing.T) {
	mainRoute, inhibitRules := loadTestRoute(t, "testdata/conf.routing.yml")

	tc := testFileCase{
		Name: "label test=2 routes to test1 and test2",
		Alerts: []testFileAlertDef{
			{
				Labels:            map[string]string{"test": "2"},
				ExpectedReceivers: []string{"test1", "test2"},
			},
		},
	}

	passed, detail := runRouteTestCase(tc, mainRoute, inhibitRules)
	require.True(t, passed, "expected pass but got failure: %s", detail)
}

// TestRunRouteTestCase_InhibitionPass checks that an alert is correctly
// detected as inhibited when a source alert is also firing.
func TestRunRouteTestCase_InhibitionPass(t *testing.T) {
	mainRoute, inhibitRules := loadTestRoute(t, "testdata/conf.inhibit.yml")
	if len(inhibitRules) == 0 {
		t.Skip("inhibit config file not present or has no rules")
	}

	tc := testFileCase{
		Name: "critical suppresses warning",
		Alerts: []testFileAlertDef{
			{
				Labels:            map[string]string{"alertname": "SomeAlert", "severity": "critical"},
				ExpectedReceivers: []string{"default"},
			},
			{
				Labels:            map[string]string{"alertname": "SomeAlert", "severity": "warning"},
				ExpectedInhibited: true,
			},
		},
	}

	passed, detail := runRouteTestCase(tc, mainRoute, inhibitRules)
	require.True(t, passed, "expected inhibition pass but got failure: %s", detail)
}

// TestRunRouteTestCase_InhibitionExpectedButNoRules checks that requesting
// inhibition without configured rules fails gracefully.
func TestRunRouteTestCase_InhibitionExpectedButNoRules(t *testing.T) {
	mainRoute, _ := loadTestRoute(t, "testdata/conf.routing.yml")
	// Pass empty inhibit rules.
	var inhibitRules []amcommoncfg.InhibitRule

	tc := testFileCase{
		Name: "expected inhibited but no rules",
		Alerts: []testFileAlertDef{
			{
				Labels:            map[string]string{"alertname": "X"},
				ExpectedInhibited: true,
			},
		},
	}

	passed, detail := runRouteTestCase(tc, mainRoute, inhibitRules)
	require.False(t, passed)
	require.Contains(t, detail, "no inhibit_rules")
}

// TestFakeAlertsProvider_SlurpAndSubscribe verifies the provider returns the
// pre-loaded alerts as the initial batch.
func TestFakeAlertsProvider_SlurpAndSubscribe(t *testing.T) {
	now := time.Now()
	alerts := []*types.Alert{
		buildTestAlert(map[string]string{"alertname": "A"}, now),
		buildTestAlert(map[string]string{"alertname": "B"}, now),
	}
	fp := newFakeAlertsProvider(alerts)
	initial, it := fp.SlurpAndSubscribe("test")
	defer it.Close()

	require.Len(t, initial, 2)
}

// TestExecuteRoutesTestFile_AllPass runs a full integration test via a temp
// test-file written to disk, verifying the happy path returns true.
func TestExecuteRoutesTestFile_AllPass(t *testing.T) {
	content := `
tests:
  - name: "Unmatched alert routes to default"
    alerts:
      - labels:
          alertname: SomeAlert
        expected_receivers:
          - default
  - name: "test=1 routes to test1"
    alerts:
      - labels:
          test: "1"
        expected_receivers:
          - test1
`
	tmpDir := t.TempDir()
	testFilePath := filepath.Join(tmpDir, "tests.yml")
	require.NoError(t, os.WriteFile(testFilePath, []byte(content), 0600))

	// Point alertmanagerURL to nil (config file path used instead).
	passed, err := executeRoutesTestFile(context.Background(), testFilePath, "testdata/conf.routing.yml")
	require.NoError(t, err)
	require.True(t, passed)
}

// TestExecuteRoutesTestFile_Failure checks that a failing test case causes
// the function to return false.
func TestExecuteRoutesTestFile_Failure(t *testing.T) {
	content := `
tests:
  - name: "Wrong receiver"
    alerts:
      - labels:
          test: "1"
        expected_receivers:
          - wrong-receiver
`
	tmpDir := t.TempDir()
	testFilePath := filepath.Join(tmpDir, "tests.yml")
	require.NoError(t, os.WriteFile(testFilePath, []byte(content), 0600))

	passed, err := executeRoutesTestFile(context.Background(), testFilePath, "testdata/conf.routing.yml")
	require.NoError(t, err)
	require.False(t, passed)
}
