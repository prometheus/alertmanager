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
	"context"
	"testing"

	"github.com/go-openapi/strfmt"

	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"

	"github.com/prometheus/alertmanager/test/testutils"
)

// Re-export common types and functions from testutils.
type (
	Collector      = testutils.Collector
	AcceptanceOpts = testutils.AcceptanceOpts
)

var CompareCollectors = testutils.CompareCollectors

// AcceptanceTest wraps testutils.AcceptanceTest for API-based testing.
type AcceptanceTest struct {
	*testutils.AcceptanceTest
}

// NewAcceptanceTest returns a new acceptance test.
func NewAcceptanceTest(t *testing.T, opts *AcceptanceOpts) *AcceptanceTest {
	return &AcceptanceTest{
		AcceptanceTest: testutils.NewAcceptanceTest(t, opts),
	}
}

// Alertmanager wraps testutils.Alertmanager and adds API-specific methods.
type Alertmanager struct {
	*testutils.Alertmanager
}

// AlertmanagerCluster wraps testutils.AlertmanagerCluster and adds API-specific methods.
type AlertmanagerCluster struct {
	*testutils.AlertmanagerCluster
}

// AlertmanagerCluster returns a new AlertmanagerCluster.
func (t *AcceptanceTest) AlertmanagerCluster(conf string, size int) *AlertmanagerCluster {
	return &AlertmanagerCluster{
		AlertmanagerCluster: t.AcceptanceTest.AlertmanagerCluster(conf, size),
	}
}

// Members returns the underlying Alertmanager instances wrapped for API testing.
func (amc *AlertmanagerCluster) Members() []*Alertmanager {
	baseMembers := amc.AlertmanagerCluster.Members()
	wrapped := make([]*Alertmanager, len(baseMembers))
	for i, am := range baseMembers {
		wrapped[i] = &Alertmanager{Alertmanager: am}
	}
	return wrapped
}

// Push declares alerts that are to be pushed to the Alertmanager
// servers at a relative point in time.
func (amc *AlertmanagerCluster) Push(at float64, alerts ...*TestAlert) {
	for _, am := range amc.Members() {
		am.Push(at, alerts...)
	}
}

// Push declares alerts that are to be pushed to the Alertmanager
// server at a relative point in time.
func (am *Alertmanager) Push(at float64, alerts ...*TestAlert) {
	am.T.Do(at, func() {
		var cas models.PostableAlerts
		for i := range alerts {
			a := alerts[i].NativeAlert(am.Opts)
			alert := &models.PostableAlert{
				Alert: models.Alert{
					Labels:       a.Labels,
					GeneratorURL: a.GeneratorURL,
				},
				Annotations: a.Annotations,
			}
			if a.StartsAt != nil {
				alert.StartsAt = *a.StartsAt
			}
			if a.EndsAt != nil {
				alert.EndsAt = *a.EndsAt
			}
			cas = append(cas, alert)
		}

		params := alert.PostAlertsParams{}
		params.WithContext(context.Background()).WithAlerts(cas)

		_, err := am.Client().Alert.PostAlerts(&params)
		if err != nil {
			am.T.Errorf("Error pushing %v: %v", cas, err)
		}
	})
}

// SetSilence updates or creates the given Silence.
func (amc *AlertmanagerCluster) SetSilence(at float64, sil *TestSilence) {
	for _, am := range amc.Members() {
		am.SetSilence(at, sil)
	}
}

// SetSilence updates or creates the given Silence.
func (am *Alertmanager) SetSilence(at float64, sil *TestSilence) {
	am.T.Do(at, func() {
		resp, err := am.Client().Silence.PostSilences(
			silence.NewPostSilencesParams().WithSilence(
				&models.PostableSilence{
					Silence: *sil.nativeSilence(am.Opts),
				},
			),
		)
		if err != nil {
			am.T.Errorf("Error setting silence %v: %s", sil, err)
			return
		}
		sil.SetID(resp.Payload.SilenceID)
	})
}

// DelSilence deletes the silence with the sid at the given time.
func (amc *AlertmanagerCluster) DelSilence(at float64, sil *TestSilence) {
	for _, am := range amc.Members() {
		am.DelSilence(at, sil)
	}
}

// DelSilence deletes the silence with the sid at the given time.
func (am *Alertmanager) DelSilence(at float64, sil *TestSilence) {
	am.T.Do(at, func() {
		_, err := am.Client().Silence.DeleteSilence(
			silence.NewDeleteSilenceParams().WithSilenceID(strfmt.UUID(sil.ID())),
		)
		if err != nil {
			am.T.Errorf("Error deleting silence %v: %s", sil, err)
		}
	})
}
