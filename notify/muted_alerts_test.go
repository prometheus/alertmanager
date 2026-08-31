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

// The tests in this file characterize how the notification pipeline treats
// muted alerts by default. The mute stages remove muted alerts from the
// pipeline before the dedup stage runs, so muted alerts never reach the
// notification log, and the group's notification history has no way to
// represent them. Enabling the muted-alerts-in-nflog feature changes that; the
// tests for it live alongside the stages they exercise.
//
// The semantics that follow from that are discussed in
// https://github.com/prometheus/alertmanager/issues/5247, and one of them is
// the bug reported in https://github.com/prometheus/alertmanager/issues/226.
// The expectations below record what the pipeline does, including where that
// is the bug.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
)

// firingAlert returns a firing alert identified by the given alertname.
func firingAlert(name string) *alert.Alert {
	return &alert.Alert{
		Alert: model.Alert{
			Labels:   model.LabelSet{"alertname": model.LabelValue(name)},
			StartsAt: utcNow().Add(-time.Hour),
			EndsAt:   utcNow().Add(time.Hour),
		},
	}
}

// resolvedAlert returns the alert that firingAlert returns for the same name,
// resolved. Both share the same label set and therefore the same hash.
func resolvedAlert(name string) *alert.Alert {
	a := firingAlert(name)
	a.EndsAt = utcNow().Add(-time.Minute)
	return a
}

// mutedPipeline runs the part of the notification pipeline that decides
// whether a group is notified about: the mute stage that drops muted alerts,
// the dedup stage that consults the notification log, and the stage that
// writes the log entry back. The retry stage is deliberately left out so that
// the tests observe the notification decision rather than the delivery.
type mutedPipeline struct {
	t *testing.T

	// muted holds the values of the alertname label that the mute stage treats
	// as muted. Tests mutate it between flushes to mute and unmute alerts.
	muted map[model.LabelValue]struct{}

	// entry is the notification log entry the dedup stage sees. It is replaced
	// by every flush that writes to the log, as a real notification log would.
	entry *nflogpb.Entry

	// now is the timestamp of the flush currently in progress.
	now time.Time

	nflog *testNflog
	stage MultiStage
}

// newMutedPipeline returns a pipeline whose notification log is empty and
// whose receiver sends resolved notifications if sendsResolved is true.
func newMutedPipeline(t *testing.T, sendsResolved bool) *mutedPipeline {
	p := &mutedPipeline{
		t:     t,
		muted: map[model.LabelValue]struct{}{},
	}

	muter := MuteFunc(func(_ context.Context, lset model.LabelSet) bool {
		_, ok := p.muted[lset["alertname"]]
		return ok
	})

	p.nflog = &testNflog{
		qerr: nflog.ErrNotFound,
		logFunc: func(_ *nflogpb.Receiver, _ string, firing, resolved, _ []uint64, _ *nflog.Store, _ time.Duration) error {
			p.entry = &nflogpb.Entry{
				FiringAlerts:   firing,
				ResolvedAlerts: resolved,
				Timestamp:      timestamppb.New(p.now),
			}
			p.nflog.qerr = nil
			p.nflog.qres = []*nflogpb.Entry{p.entry}
			return nil
		},
	}

	recv := &nflogpb.Receiver{GroupName: "test"}
	metrics := NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{})

	p.stage = MultiStage{
		NewMuteStage(muter, metrics),
		NewDedupStage(sendResolved(sendsResolved), p.nflog, recv),
		NewSetNotifiesStage(p.nflog, recv, featurecontrol.NoopFlags{}),
	}

	return p
}

// flush runs the pipeline once, as the dispatcher does at the end of a group
// interval. It returns the alerts the receiver would have been notified about,
// the reason recorded by the dedup stage, and whether the dedup stage ran at
// all. The repeat interval is one hour.
func (p *mutedPipeline) flush(now time.Time, alerts ...*alert.Alert) ([]*alert.Alert, NotifyReason, bool) {
	p.t.Helper()

	p.now = now

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "group")
	ctx = WithRepeatInterval(ctx, time.Hour)
	ctx = WithNow(ctx, now)

	ctx, notified, err := p.stage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(p.t, err)

	// The dedup stage records its reason in the context. If it is missing, the
	// mute stage emptied the group and MultiStage short-circuited before the
	// dedup stage was reached.
	reason, ok := NotificationReason(ctx)
	return notified, reason, ok
}

// TestMuteStage_RecordsMutedAlerts asserts that the mute stage records the
// hashes of the alerts it drops in the context, keyed by the same hash the
// dedup stage uses. Only the event recorder reads them.
func TestMuteStage_RecordsMutedAlerts(t *testing.T) {
	muter := MuteFunc(func(_ context.Context, lset model.LabelSet) bool {
		return lset["alertname"] == "muted"
	})
	stage := NewMuteStage(muter, NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}))

	muted, active := firingAlert("muted"), firingAlert("active")

	ctx, alerts, err := stage.Exec(context.Background(), promslog.NewNopLogger(), muted, active)
	require.NoError(t, err)
	require.Equal(t, []*alert.Alert{active}, alerts)

	mutedHashes, ok := MutedAlerts(ctx)
	require.True(t, ok, "MutedAlerts should be in the context")
	require.Equal(t, alertHashSet(hashAlert(muted)), mutedHashes)
}

// stubTimeMuter is a TimeMuter with a fixed answer.
type stubTimeMuter struct {
	mutes bool
	names []string
}

// Mutes implements the TimeMuter interface.
func (m stubTimeMuter) Mutes(_ []string, _ time.Time) (bool, []string, error) {
	return m.mutes, m.names, nil
}

// TestTimeStagesRecordMutedAlerts asserts that the time interval stages record
// the alerts they mute in the context, in the same way MuteStage does for
// silences and inhibitions.
func TestTimeStagesRecordMutedAlerts(t *testing.T) {
	tests := []struct {
		name  string
		muter TimeMuter
		stage func(TimeMuter, marker.GroupMarker, *Metrics) Stage
	}{{
		// The mute stage mutes when the time is inside a mute time interval.
		name:  "TimeMuteStage",
		muter: stubTimeMuter{mutes: true, names: []string{"evenings"}},
		stage: func(m TimeMuter, gm marker.GroupMarker, metrics *Metrics) Stage {
			return NewTimeMuteStage(m, gm, metrics)
		},
	}, {
		// The active stage mutes when the time is outside every active time
		// interval.
		name:  "TimeActiveStage",
		muter: stubTimeMuter{mutes: false},
		stage: func(m TimeMuter, gm marker.GroupMarker, metrics *Metrics) Stage {
			return NewTimeActiveStage(m, gm, metrics)
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metrics := NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{})
			st := test.stage(test.muter, marker.NewGroupMarker(), metrics)

			ctx := context.Background()
			ctx = WithNow(ctx, utcNow())
			ctx = WithGroupKey(ctx, "group1")
			ctx = WithRouteID(ctx, "route1")
			ctx = WithMuteTimeIntervals(ctx, []string{"evenings"})
			ctx = WithActiveTimeIntervals(ctx, []string{"weekdays"})

			muted := firingAlert("test")

			ctx, active, err := st.Exec(ctx, promslog.NewNopLogger(), muted)
			require.NoError(t, err)

			// The alert is dropped from the pipeline and recorded as muted.
			require.Empty(t, active)
			mutedHashes, ok := MutedAlerts(ctx)
			require.True(t, ok, "MutedAlerts should be in the context")
			require.Equal(t, alertHashSet(hashAlert(muted)), mutedHashes)
		})
	}
}

// TestMultiStage_ShortCircuitsWhenAllAlertsMuted asserts that once the mute
// stage has removed every alert from the group, MultiStage stops and the
// stages after it, including the dedup stage, never run. "All alerts are
// muted" is therefore invisible to the notification log.
func TestMultiStage_ShortCircuitsWhenAllAlertsMuted(t *testing.T) {
	muter := MuteFunc(func(context.Context, model.LabelSet) bool { return true })

	var reached bool
	stage := MultiStage{
		NewMuteStage(muter, NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{})),
		StageFunc(func(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
			reached = true
			return ctx, alerts, nil
		}),
	}

	_, alerts, err := stage.Exec(context.Background(), promslog.NewNopLogger(), firingAlert("test"))
	require.NoError(t, err)
	require.Empty(t, alerts)
	require.False(t, reached, "stages after the mute stage should not run when every alert is muted")
}

// TestDedup_MutedAlertVanishesFromNflog asserts that a muted alert is dropped
// from the notification log entry the next time the group is notified about,
// even though the alert is still firing. Nothing downstream can then tell it
// apart from an alert that stopped firing.
func TestDedup_MutedAlertVanishesFromNflog(t *testing.T) {
	p := newMutedPipeline(t, true)
	base := utcNow()

	a, b := firingAlert("a"), firingAlert("b")

	// Both alerts fire and the receiver is notified about both.
	notified, reason, _ := p.flush(base, a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a, b}, notified)
	require.Equal(t, []uint64{hashAlert(a), hashAlert(b)}, p.entry.FiringAlerts)

	// Mute a. Both alerts are still firing, so the group is notified about
	// again once the repeat interval has elapsed, but for b only.
	p.muted[a.Labels["alertname"]] = struct{}{}

	notified, reason, _ = p.flush(base.Add(2*time.Hour), a, b)
	require.Equal(t, ReasonRepeatIntervalElapsed, reason)
	require.Equal(t, []*alert.Alert{b}, notified)

	// a is now absent from the notification log despite still firing.
	require.Equal(t, []uint64{hashAlert(b)}, p.entry.FiringAlerts)
}

// TestDedup_NoResolvedNotificationForMutedAlert is
// https://github.com/prometheus/alertmanager/issues/226 as a unit test. An
// alert that has already been notified about and is then muted never produces
// the matching resolved notification, because the mute stage empties the group
// before the dedup stage can notice that everything has resolved.
func TestDedup_NoResolvedNotificationForMutedAlert(t *testing.T) {
	base := utcNow()
	a, aResolved := firingAlert("a"), resolvedAlert("a")

	t.Run("muted", func(t *testing.T) {
		p := newMutedPipeline(t, true)

		notified, reason, _ := p.flush(base, a)
		require.Equal(t, ReasonFirstNotification, reason)
		require.Equal(t, []*alert.Alert{a}, notified)

		p.muted[a.Labels["alertname"]] = struct{}{}

		notified, _, dedupRan := p.flush(base.Add(time.Minute), aResolved)
		require.Empty(t, notified, "no resolved notification should be sent for a muted alert")
		require.False(t, dedupRan, "the dedup stage should not be reached")

		// The notification log still records the alert as firing, so a
		// stateful receiver is left permanently out of sync.
		require.Equal(t, []uint64{hashAlert(a)}, p.entry.FiringAlerts)
		require.Empty(t, p.entry.ResolvedAlerts)
	})

	t.Run("not muted", func(t *testing.T) {
		// The same sequence without the mute sends the resolved notification.
		p := newMutedPipeline(t, true)

		_, reason, _ := p.flush(base, a)
		require.Equal(t, ReasonFirstNotification, reason)

		notified, reason, _ := p.flush(base.Add(time.Minute), aResolved)
		require.Equal(t, ReasonAllAlertsResolved, reason)
		require.Equal(t, []*alert.Alert{aResolved}, notified)

		require.Empty(t, p.entry.FiringAlerts)
		require.Equal(t, []uint64{hashAlert(a)}, p.entry.ResolvedAlerts)
	})
}

// TestDedup_MutedAlertBreaksNotificationSequence asserts the incoherent
// sequence discussed in
// https://github.com/prometheus/alertmanager/issues/5247. Alert a is muted for
// as long as it is firing, so the group is reported as entirely resolved while
// a is still firing, and a then arrives as a brand new group once it is
// unmuted.
func TestDedup_MutedAlertBreaksNotificationSequence(t *testing.T) {
	p := newMutedPipeline(t, true)
	base := utcNow()

	a, b := firingAlert("a"), firingAlert("b")
	p.muted[a.Labels["alertname"]] = struct{}{}

	// a is muted, so the first notification for the group covers b only.
	notified, reason, _ := p.flush(base, a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{b}, notified)
	require.Equal(t, []uint64{hashAlert(b)}, p.entry.FiringAlerts)

	// b resolves while a is still firing. The dedup stage has only ever seen
	// b, so it reports the whole group as resolved.
	bResolved := resolvedAlert("b")
	notified, reason, _ = p.flush(base.Add(time.Minute), a, bResolved)
	require.Equal(t, ReasonAllAlertsResolved, reason)
	require.Equal(t, []*alert.Alert{bResolved}, notified)
	require.Empty(t, p.entry.FiringAlerts)

	// Unmuting a opens a new sequence instead of continuing the one that was
	// just closed, so the receiver is notified about a as if it had only
	// started firing now.
	delete(p.muted, a.Labels["alertname"])

	notified, reason, _ = p.flush(base.Add(2*time.Minute), a)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a}, notified)
	require.Equal(t, []uint64{hashAlert(a)}, p.entry.FiringAlerts)
}
