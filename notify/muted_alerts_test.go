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
// muted alerts.

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
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
)

// firingAlert returns a firing alert identified by the given alertname.
func firingAlert(name string) *alert.Alert {
	return alert.New(model.Alert{
		Labels:   model.LabelSet{"alertname": model.LabelValue(name)},
		StartsAt: utcNow().Add(-time.Hour),
		EndsAt:   utcNow().Add(time.Hour),
	}, time.Time{}, false)
}

// resolvedAlert returns the alert that firingAlert returns for the same name,
// resolved.
func resolvedAlert(name string) *alert.Alert {
	a := firingAlert(name)
	a.EndsAt = utcNow().Add(-time.Minute)
	return a
}

// mutedPipeline runs the part of the notification pipeline that decides
// whether a group is notified about: the mute stage that drops muted alerts,
// the dedup stage that consults the notification log, the retry stage that
// hands the receiver what is left, and the stage that writes the log entry
// back.
type mutedPipeline struct {
	t *testing.T

	// muted holds the values of the alertname label that the mute stage treats
	// as muted. Tests mutate it between flushes to mute and unmute alerts.
	muted map[model.LabelValue]struct{}

	// entry is the notification log entry the dedup stage sees. It is replaced
	// by every flush that writes to the log, as a real notification log would.
	entry *nflogpb.Entry

	// writes counts the flushes that wrote to the notification log.
	writes int

	// sequence is the notification sequence recorded by the last flush, only
	// populated with the feature enabled.
	sequence NotificationSequence

	// now is the timestamp of the flush currently in progress.
	now time.Time

	// delivered holds the alerts the integration was handed by the flush
	// currently in progress.
	delivered []*alert.Alert

	nflog *testNflog
	stage Stage
}

// newMutedPipeline returns a pipeline with the muted alerts feature disabled,
// whose notification log is empty and whose receiver sends resolved
// notifications if sendsResolved is true.
func newMutedPipeline(t *testing.T, sendsResolved bool) *mutedPipeline {
	return newMutedPipelineMuteAware(t, sendsResolved, false)
}

// newMutedPipelineMuteAware returns a pipeline wired the way PipelineBuilder
// wires it for the given setting of the muted alerts feature.
func newMutedPipelineMuteAware(t *testing.T, sendsResolved, mutedAware bool) *mutedPipeline {
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
		logFunc: func(_ *nflogpb.Receiver, _ string, firing, resolved, muted []uint64, _ *nflog.Store, _ time.Duration) error {
			p.writes++
			p.entry = &nflogpb.Entry{
				FiringAlerts:   firing,
				ResolvedAlerts: resolved,
				MutedAlerts:    muted,
				Timestamp:      timestamppb.New(p.now),
			}
			p.nflog.qerr = nil
			p.nflog.qres = []*nflogpb.Entry{p.entry}
			return nil
		},
	}

	recv := &nflogpb.Receiver{GroupName: "test"}
	metrics := NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{})

	integration := NewIntegration(notifierFunc(func(_ context.Context, alerts ...*alert.Alert) NotifyVerdict {
		p.delivered = append(p.delivered, alerts...)
		return Success()
	}), sendResolved(sendsResolved), "webhook", 0, "test")

	p.stage = newMultiStage(mutedAware,
		NewMuteStage(muter, metrics),
		NewDedupStage(&integration, p.nflog, recv, mutedAware),
		NewRetryStage(integration, "test", metrics, eventrecorder.NopRecorder()),
		NewSetNotifiesStage(p.nflog, recv, mutedAware),
	)

	return p
}

// flush runs the pipeline once, as the dispatcher does at the end of a group
// interval. It returns the alerts the receiver was shown, the reason recorded
// by the dedup stage, and whether the dedup stage ran at all. The repeat
// interval is one hour.
func (p *mutedPipeline) flush(now time.Time, alerts ...*alert.Alert) ([]*alert.Alert, NotifyReason, bool) {
	p.t.Helper()

	p.now = now
	p.delivered = nil

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "group")
	ctx = WithRepeatInterval(ctx, time.Hour)
	ctx = WithNow(ctx, now)

	ctx, _, err := p.stage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(p.t, err)

	p.sequence, _ = NotificationSequenceFor(ctx)

	// The dedup stage records its reason in the context. If it is missing, the
	// mute stage emptied the group and MultiStage short-circuited before the
	// dedup stage was reached.
	reason, ok := NotificationReason(ctx)
	return p.delivered, reason, ok
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

	mutedAlerts, ok := MutedAlerts(ctx)
	require.True(t, ok, "MutedAlerts should be in the context")
	require.Equal(t, map[uint64]*alert.Alert{hashAlert(muted): muted}, mutedAlerts)
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
			mutedAlerts, ok := MutedAlerts(ctx)
			require.True(t, ok, "MutedAlerts should be in the context")
			require.Equal(t, map[uint64]*alert.Alert{hashAlert(muted): muted}, mutedAlerts)
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

// TestDedup_NoResolvedNotificationForMutedAlert illustrates
// https://github.com/prometheus/alertmanager/issues/226. An alert
// that has already been notified about and is then muted never
// produces the matching resolved notification, because the mute stage
// empties the group before the dedup stage can notice that everything
// has resolved.
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

// TestDedup_MutedAlertBreaksNotificationSequence asserts the sequence
// discussed in
// https://github.com/prometheus/alertmanager/issues/5247. Alert a is
// muted for as long as it is firing, so the group is reported as
// entirely resolved while a is still firing, and a then arrives as a
// brand new group once it is unmuted.
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

// TestSetNotifies_NoWriteWhenNothingIsNotified asserts that a flush the dedup
// stage found nothing to say about leaves the notification log entry alone.
// With the feature enabled the stage is reached on such flushes, and writing
// would refresh the entry's timestamp and defer the repeat interval forever.
//
// It is the only flush that is skipped. A group going quiet is recorded even
// though nothing is delivered, and a repeat interval elapsing over a group with
// anything visible notifies and writes, as the last step shows.
func TestSetNotifies_NoWriteWhenNothingIsNotified(t *testing.T) {
	p := newMutedPipelineMuteAware(t, true, true)
	base := utcNow()

	a, b := firingAlert("a"), firingAlert("b")
	p.muted[b.Labels["alertname"]] = struct{}{}

	// The first notification for the group covers a only.
	notified, reason, _ := p.flush(base, a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a}, notified)
	require.Equal(t, 1, p.writes)
	firstWrite := p.entry.Timestamp.AsTime()

	// Nothing changed and the repeat interval has not elapsed.
	notified, reason, _ = p.flush(base.Add(10*time.Minute), a, b)
	require.Equal(t, ReasonDoNotNotify, reason)
	require.Empty(t, notified)
	require.Equal(t, 1, p.writes, "a flush that notifies nobody should not write to the notification log")
	require.Equal(t, firstWrite, p.entry.Timestamp.AsTime())

	// Measured from the last notification, the repeat interval still elapses.
	notified, reason, _ = p.flush(base.Add(65*time.Minute), a, b)
	require.Equal(t, ReasonRepeatIntervalElapsed, reason)
	require.Equal(t, []*alert.Alert{a}, notified)
	require.Equal(t, 2, p.writes)
}

// TestSetNotifies_FullyMutedGroupIsRecorded asserts that a group going fully
// muted is written to the notification log, and rewritten as long as it stays
// that way. Nothing is delivered -- the receiver is shown nothing until the
// per-receiver behaviour #5247 asks for exists -- but the entry has to say the
// group went quiet, and has to keep saying it: an entry expires after
// 2 * repeat_interval, and a group whose entry expired comes back as one the
// receiver has never been told about.
func TestSetNotifies_FullyMutedGroupIsRecorded(t *testing.T) {
	p := newMutedPipelineMuteAware(t, true, true)
	base := utcNow()

	a, b := firingAlert("a"), firingAlert("b")

	notified, reason, _ := p.flush(base, a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a, b}, notified)
	require.Equal(t, 1, p.writes)

	p.muted[a.Labels["alertname"]] = struct{}{}
	p.muted[b.Labels["alertname"]] = struct{}{}

	// The whole group is muted, so the receiver is shown nothing. The entry
	// records that the group went quiet, and the sequence closes as muted.
	notified, reason, _ = p.flush(base.Add(10*time.Minute), a, b)
	require.Equal(t, ReasonAllAlertsMuted, reason)
	require.Empty(t, notified)
	require.Equal(t, SequenceClosedMuted, p.sequence)
	require.Equal(t, 2, p.writes)
	require.ElementsMatch(t, []uint64{hashAlert(a), hashAlert(b)}, p.entry.FiringAlerts)
	require.ElementsMatch(t, []uint64{hashAlert(a), hashAlert(b)}, p.entry.MutedAlerts)
	closingWrite := p.entry.Timestamp.AsTime()

	// Nothing has changed since, and the entry is not due to be rewritten.
	notified, reason, _ = p.flush(base.Add(20*time.Minute), a, b)
	require.Equal(t, ReasonDoNotNotify, reason)
	require.Empty(t, notified)
	require.Equal(t, 2, p.writes)
	require.Equal(t, closingWrite, p.entry.Timestamp.AsTime())

	// The repeat interval elapses with the group still muted. Nothing is
	// delivered, but the entry is written again so that it outlives its expiry.
	notified, reason, _ = p.flush(base.Add(80*time.Minute), a, b)
	require.Equal(t, ReasonStillMuted, reason)
	require.Empty(t, notified)
	require.Equal(t, 3, p.writes)
	require.Equal(t, base.Add(80*time.Minute), p.entry.Timestamp.AsTime())
	require.ElementsMatch(t, []uint64{hashAlert(a), hashAlert(b)}, p.entry.MutedAlerts)

	// The receiver was told nothing while the group was muted, so unmuting it
	// opens a new sequence rather than continuing the closed one.
	delete(p.muted, a.Labels["alertname"])
	delete(p.muted, b.Labels["alertname"])

	notified, reason, _ = p.flush(base.Add(85*time.Minute), a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a, b}, notified)
	require.Equal(t, SequenceOpen, p.sequence)
	require.Equal(t, 4, p.writes)
	require.Empty(t, p.entry.MutedAlerts)
}

// TestDedup_StillMutedDeliversNothing no alerts should be delivered in this state.
func TestDedup_StillMutedDeliversNothing(t *testing.T) {
	p := newMutedPipelineMuteAware(t, true, true)
	base := utcNow()

	a, b := firingAlert("a"), firingAlert("b")

	delivered, reason, _ := p.flush(base, a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a, b}, delivered)

	// a is muted, and b resolves but stays in the group where the receiver can
	// still see it. Its resolution is delivered once.
	p.muted[a.Labels["alertname"]] = struct{}{}
	bResolved := resolvedAlert("b")

	delivered, reason, _ = p.flush(base.Add(time.Minute), a, bResolved)
	require.Equal(t, ReasonNewResolvedAlerts, reason)
	require.Equal(t, []*alert.Alert{bResolved}, delivered)
	require.Equal(t, 2, p.writes)

	// Nothing about the group has changed since, so the flushes that keep its
	// entry alive deliver nothing, however many repeat intervals pass.
	for i, at := range []time.Time{base.Add(2 * time.Hour), base.Add(4 * time.Hour)} {
		delivered, reason, _ = p.flush(at, a, bResolved)
		require.Equal(t, ReasonStillMuted, reason)
		require.Empty(t, delivered, "the receiver has already been shown everything it can see")
		require.Equal(t, 3+i, p.writes, "the entry is still rewritten")
		require.Equal(t, at, p.entry.Timestamp.AsTime())
	}
}

// TestDedup_MutedAlertStaysInNflog is TestDedup_MutedAlertVanishesFromNflog
// with the feature enabled. A muted alert is still firing, so it stays in the
// log, marked as muted rather than dropped.
func TestDedup_MutedAlertStaysInNflog(t *testing.T) {
	p := newMutedPipelineMuteAware(t, true, true)
	base := utcNow()

	a, b := firingAlert("a"), firingAlert("b")

	notified, reason, _ := p.flush(base, a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a, b}, notified)
	require.Equal(t, []uint64{hashAlert(a), hashAlert(b)}, p.entry.FiringAlerts)
	require.Empty(t, p.entry.MutedAlerts)
	require.Equal(t, SequenceOpen, p.sequence)

	// Mute a. Both are still firing, so the next notification, once the repeat
	// interval elapses, covers b only.
	p.muted[a.Labels["alertname"]] = struct{}{}

	notified, reason, _ = p.flush(base.Add(2*time.Hour), a, b)
	require.Equal(t, ReasonRepeatIntervalElapsed, reason)
	require.Equal(t, []*alert.Alert{b}, notified)

	// The log knows a is firing and knows the receiver was not shown it.
	require.Equal(t, []uint64{hashAlert(b), hashAlert(a)}, p.entry.FiringAlerts)
	require.Equal(t, []uint64{hashAlert(a)}, p.entry.MutedAlerts)
	require.Equal(t, SequenceOpen, p.sequence)
}

// TestDedup_MutedAlertResolvesTheSequence is
// TestDedup_NoResolvedNotificationForMutedAlert with the feature enabled. The
// dedup stage now sees that the group resolved and closes the sequence, where
// before the mute stage emptied the group first.
//
// Nothing is delivered yet: everything in the group is muted, so there is
// nothing to send until the per-receiver behaviour in
// https://github.com/prometheus/alertmanager/issues/5247 exists. What changed
// is that the log records the resolution instead of holding an alert that is
// no longer firing.
func TestDedup_MutedAlertResolvesTheSequence(t *testing.T) {
	p := newMutedPipelineMuteAware(t, true, true)
	base := utcNow()
	a, aResolved := firingAlert("a"), resolvedAlert("a")

	notified, reason, _ := p.flush(base, a)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a}, notified)

	p.muted[a.Labels["alertname"]] = struct{}{}

	notified, reason, dedupRan := p.flush(base.Add(time.Minute), aResolved)
	require.True(t, dedupRan, "the dedup stage should be reached even though the group was emptied")
	require.Equal(t, ReasonAllAlertsResolved, reason)
	require.Empty(t, notified)
	require.Equal(t, SequenceClosedResolved, p.sequence)

	require.Empty(t, p.entry.FiringAlerts)
	require.Equal(t, []uint64{hashAlert(a)}, p.entry.ResolvedAlerts)
	require.Equal(t, []uint64{hashAlert(a)}, p.entry.MutedAlerts)
}

// TestDedup_MutedAlertKeepsTheSequenceCoherent is
// TestDedup_MutedAlertBreaksNotificationSequence with the feature enabled: the
// sequence from https://github.com/prometheus/alertmanager/issues/5247. The
// group is no longer reported as resolved while a is firing. It goes quiet
// instead, as a group every one of whose alerts is muted, and the log says so.
//
// Unmuting a then opens a new sequence rather than continuing the closed one,
// which is what #5247 asks for: muting every alert in a group closes the
// sequence because none of them is active any more. Nothing is delivered on the
// close until the per-receiver behaviour that issue describes exists.
func TestDedup_MutedAlertKeepsTheSequenceCoherent(t *testing.T) {
	p := newMutedPipelineMuteAware(t, false, true)
	base := utcNow()

	a, b := firingAlert("a"), firingAlert("b")
	p.muted[a.Labels["alertname"]] = struct{}{}

	// a is muted, so the first notification covers b only, but the log records
	// that a is firing.
	notified, reason, _ := p.flush(base, a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{b}, notified)
	require.Equal(t, []uint64{hashAlert(b), hashAlert(a)}, p.entry.FiringAlerts)
	require.Equal(t, []uint64{hashAlert(a)}, p.entry.MutedAlerts)

	// b resolves while a is still firing. This receiver sends no resolved
	// notifications, so nothing is left to show and the sequence closes as
	// muted rather than the group being reported as resolved.
	bResolved := resolvedAlert("b")
	notified, reason, _ = p.flush(base.Add(time.Minute), a, bResolved)
	require.Equal(t, ReasonAllAlertsMuted, reason)
	require.Empty(t, notified)
	require.Equal(t, SequenceClosedMuted, p.sequence)

	// The entry records the group as it now stands: a firing and muted, b
	// resolved. Nothing in it was shown to the receiver.
	require.Equal(t, []uint64{hashAlert(a)}, p.entry.FiringAlerts)
	require.Equal(t, []uint64{hashAlert(b)}, p.entry.ResolvedAlerts)
	require.Equal(t, []uint64{hashAlert(a)}, p.entry.MutedAlerts)

	// Unmuting a opens a new sequence, because the one it was part of closed
	// while it was muted.
	delete(p.muted, a.Labels["alertname"])

	notified, reason, _ = p.flush(base.Add(2*time.Minute), a)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a}, notified)
	require.Equal(t, SequenceOpen, p.sequence)
	require.Equal(t, []uint64{hashAlert(a)}, p.entry.FiringAlerts)
	require.Empty(t, p.entry.MutedAlerts)
}

// TestDedup_UnmutedAlertContinuesTheSequence is the other half of
// TestDedup_MutedAlertKeepsTheSequenceCoherent: while anything in the group is
// still visible the sequence stays open, so an alert coming out of a mute
// continues it instead of arriving as a group the receiver has never seen.
func TestDedup_UnmutedAlertContinuesTheSequence(t *testing.T) {
	p := newMutedPipelineMuteAware(t, true, true)
	base := utcNow()

	a, b := firingAlert("a"), firingAlert("b")
	p.muted[b.Labels["alertname"]] = struct{}{}

	notified, reason, _ := p.flush(base, a, b)
	require.Equal(t, ReasonFirstNotification, reason)
	require.Equal(t, []*alert.Alert{a}, notified)
	require.Equal(t, []uint64{hashAlert(a), hashAlert(b)}, p.entry.FiringAlerts)
	require.Equal(t, []uint64{hashAlert(b)}, p.entry.MutedAlerts)
	require.Equal(t, SequenceOpen, p.sequence)

	delete(p.muted, b.Labels["alertname"])

	notified, reason, _ = p.flush(base.Add(time.Minute), a, b)
	require.Equal(t, ReasonAlertsUnmuted, reason)
	require.Equal(t, []*alert.Alert{a, b}, notified)
	require.Equal(t, SequenceOpen, p.sequence)
	require.Empty(t, p.entry.MutedAlerts)
}

// mutedGroupState builds a group state from the hashes in each of its parts. A
// muted alert belongs to firing or resolved as well as to muted.
func mutedGroupState(firing, resolved, muted []uint64) groupState {
	s := groupState{
		firing:      firing,
		resolved:    resolved,
		firingSet:   alertHashSet(firing...),
		resolvedSet: alertHashSet(resolved...),
		mutedSet:    alertHashSet(muted...),
	}
	return s
}

// TestDedup_NeedsUpdateMuteAware walks every transition of the mute aware dedup
// rules. Hashes 1, 2 and 3 stand for three alerts in one group.
func TestDedup_NeedsUpdateMuteAware(t *testing.T) {
	now := utcNow()
	repeat := time.Hour

	// entry writes within the repeat interval, so it does not elapse unless a
	// case asks for it.
	entry := func(firing, resolved, muted []uint64) *nflogpb.Entry {
		return &nflogpb.Entry{
			FiringAlerts:   firing,
			ResolvedAlerts: resolved,
			MutedAlerts:    muted,
			Timestamp:      timestamppb.New(now.Add(-repeat / 2)),
		}
	}

	tests := []struct {
		name          string
		entry         *nflogpb.Entry
		firing        []uint64
		resolved      []uint64
		muted         []uint64
		sendsResolved bool
		want          NotifyReason
	}{{
		name:   "first notification",
		firing: []uint64{1},
		want:   ReasonFirstNotification,
	}, {
		name:   "first notification is held back while everything is muted",
		firing: []uint64{1},
		muted:  []uint64{1},
		want:   ReasonDoNotNotify,
	}, {
		name:     "nothing to notify about when a group only ever resolved",
		resolved: []uint64{1},
		want:     ReasonDoNotNotify,
	}, {
		// The receiver was shown the group, so the group closes, even though
		// the alert that resolved it is muted and nothing can be delivered.
		name:     "muted alert resolves the group the receiver was told about",
		entry:    entry([]uint64{1}, nil, nil),
		resolved: []uint64{1},
		muted:    []uint64{1},
		want:     ReasonAllAlertsResolved,
	}, {
		// Alert 1 resolving can be shown, so the group still closes.
		name:          "group resolves with one of its alerts muted",
		entry:         entry([]uint64{1, 2}, nil, nil),
		resolved:      []uint64{1, 2},
		muted:         []uint64{2},
		sendsResolved: true,
		want:          ReasonAllAlertsResolved,
	}, {
		name:     "group resolves after the receiver was shown it",
		entry:    entry([]uint64{1, 2}, nil, nil),
		resolved: []uint64{1, 2},
		want:     ReasonAllAlertsResolved,
	}, {
		// Everything the entry recorded as firing was muted, so the receiver
		// does not know the group ever fired.
		name:     "group resolves without the receiver ever being shown it",
		entry:    entry([]uint64{1}, nil, []uint64{1}),
		resolved: []uint64{1},
		muted:    []uint64{1},
		want:     ReasonDoNotNotify,
	}, {
		// The group is not over: alert 2 is still firing, just muted.
		name:     "muted firing alert keeps the group from resolving",
		entry:    entry([]uint64{1, 2}, nil, nil),
		firing:   []uint64{2},
		resolved: []uint64{1},
		muted:    []uint64{2},
		want:     ReasonAllAlertsMuted,
	}, {
		name:   "alert added to the group",
		entry:  entry([]uint64{1}, nil, nil),
		firing: []uint64{1, 2},
		want:   ReasonNewAlertsInGroup,
	}, {
		// Alert 2 was already recorded as firing, so its mute ended.
		name:   "muted alert becomes visible",
		entry:  entry([]uint64{1, 2}, nil, []uint64{2}),
		firing: []uint64{1, 2},
		want:   ReasonAlertsUnmuted,
	}, {
		// Both at once. The alert the receiver has never heard of wins.
		name:   "alert added to the group while another is unmuted",
		entry:  entry([]uint64{1, 2}, nil, []uint64{2}),
		firing: []uint64{1, 2, 3},
		want:   ReasonNewAlertsInGroup,
	}, {
		// Nothing was shown last time, so this opens a new sequence.
		name:   "sequence reopens after everything was muted",
		entry:  entry([]uint64{1}, nil, []uint64{1}),
		firing: []uint64{1},
		want:   ReasonFirstNotification,
	}, {
		name:          "alert resolves in a group that is still firing",
		entry:         entry([]uint64{1, 2}, nil, nil),
		firing:        []uint64{1},
		resolved:      []uint64{2},
		sendsResolved: true,
		want:          ReasonNewResolvedAlerts,
	}, {
		// Alert 2 was never shown, so its resolution is not news.
		name:          "muted alert resolves in a group that is still firing",
		entry:         entry([]uint64{1, 2}, nil, []uint64{2}),
		firing:        []uint64{1},
		resolved:      []uint64{2},
		muted:         []uint64{2},
		sendsResolved: true,
		want:          ReasonDoNotNotify,
	}, {
		name:   "every visible alert is muted",
		entry:  entry([]uint64{1, 2}, nil, nil),
		firing: []uint64{1, 2},
		muted:  []uint64{1, 2},
		want:   ReasonAllAlertsMuted,
	}, {
		// Closing writes nothing, so the next flush sees the same entry.
		name:   "every visible alert stays muted",
		entry:  entry([]uint64{1, 2}, nil, []uint64{1}),
		firing: []uint64{1, 2},
		muted:  []uint64{1, 2},
		want:   ReasonAllAlertsMuted,
	}, {
		// Nothing was ever shown, so there is no sequence to close.
		name:   "muted group the receiver was never shown",
		entry:  entry([]uint64{1}, nil, []uint64{1}),
		firing: []uint64{1},
		muted:  []uint64{1},
		want:   ReasonDoNotNotify,
	}, {
		name:   "nothing changed",
		entry:  entry([]uint64{1}, nil, nil),
		firing: []uint64{1},
		want:   ReasonDoNotNotify,
	}, {
		name: "repeat interval elapsed",
		entry: &nflogpb.Entry{
			FiringAlerts: []uint64{1},
			Timestamp:    timestamppb.New(now.Add(-2 * repeat)),
		},
		firing: []uint64{1},
		want:   ReasonRepeatIntervalElapsed,
	}, {
		// There is nothing to repeat while none of the group can be shown, but
		// the entry recording that has to outlive its expiry.
		name: "fully muted group is recorded again once the repeat interval elapses",
		entry: &nflogpb.Entry{
			FiringAlerts: []uint64{1},
			MutedAlerts:  []uint64{1},
			Timestamp:    timestamppb.New(now.Add(-2 * repeat)),
		},
		firing: []uint64{1},
		muted:  []uint64{1},
		want:   ReasonStillMuted,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &DedupStage{
				rs:         sendResolved(test.sendsResolved),
				mutedAware: true,
				now:        func() time.Time { return now },
			}
			state := mutedGroupState(test.firing, test.resolved, test.muted)
			require.Equal(t, test.want, s.needsUpdateMuteAware(test.entry, state, repeat, now))
		})
	}
}
