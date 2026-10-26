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

package notify

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/alert"
	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

// BenchmarkMuteStage measures the steady-state cost of running a group of
// alerts through the mute stages of the notification pipeline: an inhibitor
// stage, a silencer stage, and both chained as the pipeline does. The alerts
// carry realistic label sets and are neither silenced nor inhibited, which is
// the common case for every alert on every pipeline run.
func BenchmarkMuteStage(b *testing.B) {
	const (
		numAlerts   = 1000
		numSilences = 1000
		numRules    = 10
	)
	alerts := newBenchAlerts(numAlerts)
	metrics := NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{})
	logger := promslog.NewNopLogger()

	b.Run("inhibitor/rules="+strconv.Itoa(numRules)+"/alerts="+strconv.Itoa(numAlerts), func(b *testing.B) {
		stage := NewMuteStage(newBenchInhibitor(b, numRules), metrics)
		ctx := marker.WithContext(context.Background(), marker.NewAlertMarker())
		warmUp(ctx, b, stage, alerts)
		for b.Loop() {
			_, _, err := stage.Exec(ctx, logger, alerts...)
			require.NoError(b, err)
		}
	})

	b.Run("silencer/silences="+strconv.Itoa(numSilences)+"/alerts="+strconv.Itoa(numAlerts), func(b *testing.B) {
		stage := NewMuteStage(newBenchSilencer(b, numSilences), metrics)
		ctx := marker.WithContext(context.Background(), marker.NewAlertMarker())
		warmUp(ctx, b, stage, alerts)
		for b.Loop() {
			_, _, err := stage.Exec(ctx, logger, alerts...)
			require.NoError(b, err)
		}
	})

	b.Run("pipeline/alerts="+strconv.Itoa(numAlerts), func(b *testing.B) {
		stages := MultiStage{
			NewMuteStage(newBenchInhibitor(b, numRules), metrics),
			NewMuteStage(newBenchSilencer(b, numSilences), metrics),
		}
		ctx := marker.WithContext(context.Background(), marker.NewAlertMarker())
		warmUp(ctx, b, stages, alerts)
		for b.Loop() {
			_, _, err := stages.Exec(ctx, logger, alerts...)
			require.NoError(b, err)
		}
	})
}

// warmUp runs the alerts through the stage once so that the timed loop
// measures the steady state, with the silencer's per-alert cache populated.
func warmUp(ctx context.Context, b *testing.B, stage Stage, alerts []*alert.Alert) {
	b.Helper()
	_, _, err := stage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(b, err)
}

// newBenchAlerts returns n alerts with realistic, distinct label sets.
func newBenchAlerts(n int) []*alert.Alert {
	now := time.Now()
	alerts := make([]*alert.Alert, 0, n)
	for i := range n {
		id := strconv.Itoa(i)
		a := alert.New(model.Alert{
			Labels: model.LabelSet{
				"alertname": "HighRequestLatency",
				"cluster":   "prod-eu-west-1",
				"env":       "production",
				"instance":  model.LabelValue("10.0." + strconv.Itoa(i/256) + "." + strconv.Itoa(i%256) + ":9090"),
				"job":       "api-server",
				"namespace": "payments",
				"pod":       model.LabelValue("api-server-" + id),
				"region":    "eu-west-1",
				"severity":  "warning",
				"team":      "payments-oncall",
			},
			Annotations: model.LabelSet{"summary": "Request latency is high"},
			StartsAt:    now.Add(-time.Hour),
			EndsAt:      now.Add(time.Hour),
		}, now, false)
		alerts = append(alerts, a)
	}
	return alerts
}

// newBenchSilencer returns a silencer with n active silences, none of which
// match the benchmark alerts.
func newBenchSilencer(b *testing.B, n int) *silence.Silencer {
	silences, err := silence.New(silence.Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
	require.NoError(b, err)
	now := time.Now()
	for i := range n {
		require.NoError(b, silences.Set(b.Context(), &silencepb.Silence{
			Matchers: []*silencepb.Matcher{{
				Type:    silencepb.Matcher_EQUAL,
				Name:    "job",
				Pattern: "job" + strconv.Itoa(i),
			}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(24 * time.Hour)),
		}))
	}
	return silence.NewSilencer(silences, promslog.NewNopLogger(), eventrecorder.NopRecorder())
}

// newBenchInhibitor returns a running inhibitor with n rules whose target
// matchers do not match the benchmark alerts.
func newBenchInhibitor(b *testing.B, n int) *inhibit.Inhibitor {
	alerts, err := mem.NewAlerts(b.Context(), time.Minute, 0, nil, promslog.NewNopLogger(), eventrecorder.NopRecorder(), prometheus.NewRegistry(), nil)
	require.NoError(b, err)
	b.Cleanup(alerts.Close)

	rules := make([]amcommoncfg.InhibitRule, 0, n)
	for i := range n {
		src, err := labels.NewMatcher(labels.MatchEqual, "alertname", "Source"+strconv.Itoa(i))
		require.NoError(b, err)
		dst, err := labels.NewMatcher(labels.MatchEqual, "alertname", "Target"+strconv.Itoa(i))
		require.NoError(b, err)
		rules = append(rules, amcommoncfg.InhibitRule{
			SourceMatchers: amcommoncfg.Matchers{src},
			TargetMatchers: amcommoncfg.Matchers{dst},
			Equal:          []string{"cluster", "namespace"},
		})
	}
	ih := inhibit.NewInhibitor(alerts, rules, promslog.NewNopLogger(), eventrecorder.NopRecorder())
	go ih.Run()
	b.Cleanup(ih.Stop)
	return ih
}
