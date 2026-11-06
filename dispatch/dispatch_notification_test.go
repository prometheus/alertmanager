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

package dispatch

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/mem"
)

func TestDispatcherSameKeyNotificationTimings(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		conf, err := config.Load(`
route:
  receiver: team
  group_by: [alertname]
  routes:
  - matchers: [service=api]
    group_wait: 2s
    group_interval: 3s
    repeat_interval: 4s
    continue: true
  - matchers: [service=api]
    group_wait: 1s
    group_interval: 1s
    repeat_interval: 10s
receivers:
- name: team
`)
		require.NoError(t, err)

		alerts := newDispatcherTestAlerts(t)
		pipeline, recorder := newTestNotificationPipeline(t)
		dispatcher := newDispatcherForTest(t, alerts, NewRoute(conf.Route, nil), pipeline)
		go dispatcher.Run(time.Now())
		dispatcher.WaitForLoading()
		synctest.Wait()

		time.Sleep(8 * time.Second)
		synctest.Wait()

		groups := dispatcherTestGroups(t, dispatcher)
		require.Len(t, groups, 2)
		require.Equal(t, groups[0].GroupKey, groups[1].GroupKey)

		flushes, notifications := recorder.snapshot()
		timesByRepeat := map[time.Duration][]time.Duration{}
		for _, f := range flushes {
			timesByRepeat[f.repeatInterval] = append(timesByRepeat[f.repeatInterval], f.elapsed)
		}
		require.Equal(t, map[time.Duration][]time.Duration{
			4 * time.Second:  {2 * time.Second, 5 * time.Second, 8 * time.Second},
			10 * time.Second: {time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 5 * time.Second, 6 * time.Second, 7 * time.Second, 8 * time.Second},
		}, timesByRepeat)

		// The fast group notifies at 1s. The slow group's flushes at 2s and
		// 5s are deduplicated against that notification. At 8s its repeat
		// interval has elapsed, while the fast group's has not.
		const groupKey = `{}/{service="api"}:{alertname="A"}`
		require.Equal(t, []recordedNotification{
			{elapsed: time.Second, groupKey: groupKey, repeatInterval: 10 * time.Second, reason: notify.ReasonFirstNotification},
			{elapsed: 8 * time.Second, groupKey: groupKey, repeatInterval: 4 * time.Second, reason: notify.ReasonRepeatIntervalElapsed},
		}, notifications)
	})
}

// newDispatcherTestAlerts uses the current clock, including a synctest bubble's
// clock, so the alert's lifetime covers the dispatcher's notifications.
func newDispatcherTestAlerts(t *testing.T) *mem.Alerts {
	t.Helper()
	alerts, err := mem.NewAlerts(context.Background(), time.Hour, 0, nil, promslog.NewNopLogger(), eventrecorder.NopRecorder(), prometheus.NewRegistry(), nil)
	require.NoError(t, err)
	t.Cleanup(alerts.Close)

	now := time.Now()
	require.NoError(t, alerts.Put(context.Background(), alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": "A", "service": "api", "env": "prod"},
		StartsAt: now,
		EndsAt:   now.Add(time.Hour),
	}, now, false)))
	return alerts
}

func newDispatcherForTest(t *testing.T, alerts *mem.Alerts, route *Route, stage notify.Stage) *Dispatcher {
	t.Helper()
	dispatcher := NewDispatcher(alerts, route, stage, nil, nil, time.Hour, nil, promslog.NewNopLogger(), eventrecorder.NopRecorder(), nil, nil)
	t.Cleanup(dispatcher.Stop)
	return dispatcher
}

func dispatcherTestGroups(t *testing.T, dispatcher *Dispatcher) AlertGroups {
	t.Helper()
	groups, _, err := dispatcher.Groups(context.Background(),
		func(*Route) bool { return true },
		func(*alert.Alert, time.Time) (alert.AlertStatus, bool) { return alert.AlertStatus{}, true },
	)
	require.NoError(t, err)
	return groups
}

// newTestNotificationPipeline records flushes and notifier calls around the
// production deduplication, retry and notification log stages.
func newTestNotificationPipeline(t *testing.T) (notify.Stage, *notificationRecorder) {
	t.Helper()
	log, err := nflog.New(nflog.Options{Retention: time.Hour, Metrics: prometheus.NewRegistry()})
	require.NoError(t, err)

	recorder := &notificationRecorder{start: time.Now()}
	integration := notify.NewIntegration(recorder, recorder, "test", 0, "team")
	receiver := &nflogpb.Receiver{GroupName: "team", Integration: "test", Idx: 0}
	flags := featurecontrol.NoopFlags{}
	metrics := notify.NewMetrics(prometheus.NewRegistry(), flags)
	return notify.RoutingStage{"team": notify.MultiStage{
		recorder,
		notify.NewDedupStage(&integration, log, receiver),
		notify.NewRetryStage(integration, "team", metrics, eventrecorder.NopRecorder()),
		notify.NewSetNotifiesStage(log, receiver, flags),
	}}, recorder
}

type recordedNotification struct {
	elapsed        time.Duration
	groupKey       string
	repeatInterval time.Duration
	reason         notify.NotifyReason
}

type notificationRecorder struct {
	start         time.Time
	mtx           sync.Mutex
	flushes       []recordedNotification
	notifications []recordedNotification
}

func (r *notificationRecorder) record(ctx context.Context) recordedNotification {
	now, _ := notify.Now(ctx)
	key, _ := notify.GroupKey(ctx)
	repeat, _ := notify.RepeatInterval(ctx)
	reason, _ := notify.NotificationReason(ctx)
	return recordedNotification{elapsed: now.Sub(r.start), groupKey: key, repeatInterval: repeat, reason: reason}
}

// Exec records a flush before notification deduplication.
func (r *notificationRecorder) Exec(ctx context.Context, _ *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.flushes = append(r.flushes, r.record(ctx))
	return ctx, alerts, nil
}

// Notify records a notification that has passed deduplication.
func (r *notificationRecorder) Notify(ctx context.Context, _ ...*alert.Alert) notify.NotifyVerdict {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.notifications = append(r.notifications, r.record(ctx))
	return notify.Success()
}

// SendResolved allows the pipeline to notify about resolved alerts.
func (*notificationRecorder) SendResolved() bool {
	return true
}

func (r *notificationRecorder) snapshot() (flushes, notifications []recordedNotification) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return slices.Clone(r.flushes), slices.Clone(r.notifications)
}
