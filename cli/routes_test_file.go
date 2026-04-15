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
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"gopkg.in/yaml.v2"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

// testFileAlertDef is a single alert definition within a test case.
type testFileAlertDef struct {
	// Labels is the set of labels for this alert.
	Labels map[string]string `yaml:"labels"`
	// ExpectedReceivers lists the receiver names the alert should route to.
	// Mutually exclusive with ExpectedInhibited.
	ExpectedReceivers []string `yaml:"expected_receivers,omitempty"`
	// ExpectedInhibited asserts that this alert should be inhibited.
	// Mutually exclusive with ExpectedReceivers.
	ExpectedInhibited bool `yaml:"expected_inhibited,omitempty"`
}

// testFileCase is a named test case containing one or more alert definitions.
type testFileCase struct {
	// Name is the human-readable name for this test case.
	Name string `yaml:"name"`
	// Alerts is the set of alerts to fire together in this test case.
	Alerts []testFileAlertDef `yaml:"alerts"`
}

// routesTestFile is the top-level structure for a routes test file.
type routesTestFile struct {
	// Tests is the list of test cases to run.
	Tests []testFileCase `yaml:"tests"`
}

// fakeAlertsProvider is a minimal provider.Alerts implementation used for
// inhibition testing. It pre-loads a fixed set of alerts and makes them
// available via SlurpAndSubscribe.
type fakeAlertsProvider struct {
	alerts []*types.Alert
}

// newFakeAlertsProvider returns a fakeAlertsProvider pre-loaded with the given alerts.
func newFakeAlertsProvider(alerts []*types.Alert) *fakeAlertsProvider {
	return &fakeAlertsProvider{alerts: alerts}
}

// Subscribe implements provider.Alerts. Returns an iterator with all alerts buffered.
func (f *fakeAlertsProvider) Subscribe(_ string) provider.AlertIterator {
	ch := make(chan *provider.Alert, len(f.alerts))
	done := make(chan struct{})
	for _, a := range f.alerts {
		ch <- &provider.Alert{Data: a}
	}
	return provider.NewAlertIterator(ch, done, nil)
}

// SlurpAndSubscribe implements provider.Alerts. Returns all alerts as the
// initial batch and an idle iterator for subsequent updates.
func (f *fakeAlertsProvider) SlurpAndSubscribe(_ string) ([]*types.Alert, provider.AlertIterator) {
	// Return the alerts as the initial set so that the inhibitor's run()
	// processes them before WaitForLoading() returns.
	ch := make(chan *provider.Alert)
	done := make(chan struct{})
	return f.alerts, provider.NewAlertIterator(ch, done, nil)
}

// GetPending implements provider.Alerts.
func (f *fakeAlertsProvider) GetPending() provider.AlertIterator {
	ch := make(chan *provider.Alert)
	done := make(chan struct{})
	return provider.NewAlertIterator(ch, done, nil)
}

// Get implements provider.Alerts.
func (f *fakeAlertsProvider) Get(fp model.Fingerprint) (*types.Alert, error) {
	for _, a := range f.alerts {
		if a.Fingerprint() == fp {
			return a, nil
		}
	}
	return nil, provider.ErrNotFound
}

// Put implements provider.Alerts.
func (f *fakeAlertsProvider) Put(_ context.Context, alerts ...*types.Alert) error {
	f.alerts = append(f.alerts, alerts...)
	return nil
}

// nopPrometheusRegisterer is a prometheus.Registerer that silently discards
// all metric registrations, used to avoid metric conflicts during testing.
type nopPrometheusRegisterer struct{}

func (nopPrometheusRegisterer) Register(prometheus.Collector) error  { return nil }
func (nopPrometheusRegisterer) MustRegister(...prometheus.Collector) {}
func (nopPrometheusRegisterer) Unregister(prometheus.Collector) bool { return true }

// executeRoutesTestFile loads the test file at testFilePath, runs all test
// cases against the Alertmanager configuration, prints PASS/FAIL for each
// case, and returns true when all cases pass.
func executeRoutesTestFile(ctx context.Context, testFilePath string, configFile string) (bool, error) {
	data, err := os.ReadFile(testFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to read test file %q: %w", testFilePath, err)
	}

	var tf routesTestFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return false, fmt.Errorf("failed to parse test file %q: %w", testFilePath, err)
	}

	cfg, err := loadAlertmanagerConfig(ctx, alertmanagerURL, configFile)
	if err != nil {
		return false, fmt.Errorf("failed to load Alertmanager config: %w", err)
	}

	mainRoute := dispatch.NewRoute(cfg.Route, nil)
	inhibitRules := cfg.InhibitRules

	allPassed := true
	for _, tc := range tf.Tests {
		passed, detail := runRouteTestCase(tc, mainRoute, inhibitRules)
		if passed {
			fmt.Printf("PASS %s\n", tc.Name)
		} else {
			fmt.Printf("FAIL %s / %s\n", tc.Name, detail)
			allPassed = false
		}
	}

	return allPassed, nil
}

// runRouteTestCase executes a single named test case. It returns (true, "")
// when all per-alert assertions pass, or (false, <reason>) on the first
// failure.
func runRouteTestCase(
	tc testFileCase,
	mainRoute *dispatch.Route,
	inhibitRules []amcommoncfg.InhibitRule,
) (bool, string) {
	now := time.Now()

	// Build types.Alert slice for all alerts in this case so the inhibitor
	// can see the whole set at once.
	allAlerts := make([]*types.Alert, 0, len(tc.Alerts))
	for _, ad := range tc.Alerts {
		allAlerts = append(allAlerts, buildTestAlert(ad.Labels, now))
	}

	// Construct inhibitor when rules are present.
	var ih *inhibit.Inhibitor
	if len(inhibitRules) > 0 {
		marker := types.NewMarker(nopPrometheusRegisterer{})
		fp := newFakeAlertsProvider(allAlerts)
		ih = inhibit.NewInhibitor(fp, inhibitRules, marker, promslog.NewNopLogger(), eventrecorder.NopRecorder())
		go ih.Run()
		// Wait until the inhibitor has consumed the initial alert batch.
		ih.WaitForLoading()
		defer ih.Stop()
	}

	// Validate each alert definition.
	for i, ad := range tc.Alerts {
		lset := mapToLabelSet(ad.Labels)

		if ad.ExpectedInhibited {
			if ih == nil {
				return false, fmt.Sprintf(
					"alert[%d] %s: expected_inhibited=true but no inhibit_rules are configured",
					i, labelMapString(ad.Labels),
				)
			}
			muted := ih.Mutes(context.Background(), lset)
			if !muted {
				return false, fmt.Sprintf(
					"alert[%d] %s: expected to be inhibited but was not",
					i, labelMapString(ad.Labels),
				)
			}
		} else {
			matchedRoutes := mainRoute.Match(lset)
			receivers := make([]string, 0, len(matchedRoutes))
			for _, r := range matchedRoutes {
				receivers = append(receivers, r.RouteOpts.Receiver)
			}

			if !equalStringSlices(ad.ExpectedReceivers, receivers) {
				return false, fmt.Sprintf(
					"alert[%d] %s: expected receivers %v but got %v",
					i, labelMapString(ad.Labels), ad.ExpectedReceivers, receivers,
				)
			}
		}
	}

	return true, ""
}

// buildTestAlert constructs a firing types.Alert from a label map.
func buildTestAlert(labelMap map[string]string, now time.Time) *types.Alert {
	return &types.Alert{
		Alert: model.Alert{
			Labels:   mapToLabelSet(labelMap),
			StartsAt: now,
			EndsAt:   now.Add(1 * time.Hour),
		},
		UpdatedAt: now,
	}
}

// mapToLabelSet converts a map[string]string into model.LabelSet.
func mapToLabelSet(m map[string]string) model.LabelSet {
	ls := make(model.LabelSet, len(m))
	for k, v := range m {
		ls[model.LabelName(k)] = model.LabelValue(v)
	}
	return ls
}

// labelMapString returns a human-readable representation of a label map.
func labelMapString(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := "{"
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += k + "=" + `"` + m[k] + `"`
	}
	out += "}"
	return out
}

// equalStringSlices returns true when two string slices have identical contents
// in the same order.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
