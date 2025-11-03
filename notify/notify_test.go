// Copyright 2015 Prometheus Team
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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/timeinterval"
	"github.com/prometheus/alertmanager/types"
)

type sendResolved bool

func (s sendResolved) SendResolved() bool {
	return bool(s)
}

type notifierFunc func(ctx context.Context, alerts ...*types.Alert) (bool, error)

func (f notifierFunc) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	return f(ctx, alerts...)
}

type failStage struct{}

func (s failStage) Exec(ctx context.Context, l *slog.Logger, as ...*types.Alert) (context.Context, []*types.Alert, error) {
	return ctx, nil, fmt.Errorf("some error")
}

type testNflog struct {
	qres []*nflogpb.Entry
	qerr error

	logFunc func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, expiry time.Duration) error
}

func (l *testNflog) Query(p ...nflog.QueryParam) ([]*nflogpb.Entry, error) {
	return l.qres, l.qerr
}

func (l *testNflog) Log(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, expiry time.Duration) error {
	return l.logFunc(r, gkey, firingAlerts, resolvedAlerts, expiry)
}

func (l *testNflog) GC() (int, error) {
	return 0, nil
}

func (l *testNflog) Snapshot(w io.Writer) (int, error) {
	return 0, nil
}

func alertHashSet(hashes ...uint64) map[uint64]struct{} {
	res := map[uint64]struct{}{}

	for _, h := range hashes {
		res[h] = struct{}{}
	}

	return res
}

func TestDedupStageNeedsUpdate(t *testing.T) {
	now := utcNow()

	cases := []struct {
		entry          *nflogpb.Entry
		firingAlerts   map[uint64]struct{}
		resolvedAlerts map[uint64]struct{}
		repeat         time.Duration
		resolve        bool

		res bool
	}{
		{
			// No matching nflog entry should update.
			entry:        nil,
			firingAlerts: alertHashSet(2, 3, 4),
			res:          true,
		}, {
			// No matching nflog entry shouldn't update if no alert fires.
			entry:          nil,
			resolvedAlerts: alertHashSet(2, 3, 4),
			res:            false,
		}, {
			// Different sets of firing alerts should update.
			entry:        &nflogpb.Entry{FiringAlerts: []uint64{1, 2, 3}},
			firingAlerts: alertHashSet(2, 3, 4),
			res:          true,
		}, {
			// Zero timestamp in the nflog entry should always update.
			entry: &nflogpb.Entry{
				FiringAlerts: []uint64{1, 2, 3},
				Timestamp:    time.Time{},
			},
			firingAlerts: alertHashSet(1, 2, 3),
			res:          true,
		}, {
			// Identical sets of alerts shouldn't update before repeat_interval.
			entry: &nflogpb.Entry{
				FiringAlerts: []uint64{1, 2, 3},
				Timestamp:    now.Add(-9 * time.Minute),
			},
			repeat:       10 * time.Minute,
			firingAlerts: alertHashSet(1, 2, 3),
			res:          false,
		}, {
			// Identical sets of alerts should update after repeat_interval.
			entry: &nflogpb.Entry{
				FiringAlerts: []uint64{1, 2, 3},
				Timestamp:    now.Add(-11 * time.Minute),
			},
			repeat:       10 * time.Minute,
			firingAlerts: alertHashSet(1, 2, 3),
			res:          true,
		}, {
			// Different sets of resolved alerts without firing alerts shouldn't update after repeat_interval.
			entry: &nflogpb.Entry{
				ResolvedAlerts: []uint64{1, 2, 3},
				Timestamp:      now.Add(-11 * time.Minute),
			},
			repeat:         10 * time.Minute,
			resolvedAlerts: alertHashSet(3, 4, 5),
			resolve:        true,
			res:            false,
		}, {
			// Different sets of resolved alerts shouldn't update when resolve is false.
			entry: &nflogpb.Entry{
				FiringAlerts:   []uint64{1, 2},
				ResolvedAlerts: []uint64{3},
				Timestamp:      now.Add(-9 * time.Minute),
			},
			repeat:         10 * time.Minute,
			firingAlerts:   alertHashSet(1),
			resolvedAlerts: alertHashSet(2, 3),
			resolve:        false,
			res:            false,
		}, {
			// Different sets of resolved alerts should update when resolve is true.
			entry: &nflogpb.Entry{
				FiringAlerts:   []uint64{1, 2},
				ResolvedAlerts: []uint64{3},
				Timestamp:      now.Add(-9 * time.Minute),
			},
			repeat:         10 * time.Minute,
			firingAlerts:   alertHashSet(1),
			resolvedAlerts: alertHashSet(2, 3),
			resolve:        true,
			res:            true,
		}, {
			// Empty set of firing alerts should update when resolve is false.
			entry: &nflogpb.Entry{
				FiringAlerts:   []uint64{1, 2},
				ResolvedAlerts: []uint64{3},
				Timestamp:      now.Add(-9 * time.Minute),
			},
			repeat:         10 * time.Minute,
			firingAlerts:   alertHashSet(),
			resolvedAlerts: alertHashSet(1, 2, 3),
			resolve:        false,
			res:            true,
		}, {
			// Empty set of firing alerts should update when resolve is true.
			entry: &nflogpb.Entry{
				FiringAlerts:   []uint64{1, 2},
				ResolvedAlerts: []uint64{3},
				Timestamp:      now.Add(-9 * time.Minute),
			},
			repeat:         10 * time.Minute,
			firingAlerts:   alertHashSet(),
			resolvedAlerts: alertHashSet(1, 2, 3),
			resolve:        true,
			res:            true,
		},
	}
	for i, c := range cases {
		t.Log("case", i)

		s := &DedupStage{
			now: func() time.Time { return now },
			rs:  sendResolved(c.resolve),
		}
		res := s.needsUpdate(c.entry, c.firingAlerts, c.resolvedAlerts, c.repeat)
		require.Equal(t, c.res, res)
	}
}

func TestDedupStage(t *testing.T) {
	i := 0
	now := utcNow()
	s := &DedupStage{
		hash: func(a *types.Alert) uint64 {
			res := uint64(i)
			i++
			return res
		},
		now: func() time.Time {
			return now
		},
		rs: sendResolved(false),
	}

	ctx := context.Background()

	_, _, err := s.Exec(ctx, promslog.NewNopLogger())
	require.EqualError(t, err, "group key missing")

	ctx = WithGroupKey(ctx, "1")

	_, _, err = s.Exec(ctx, promslog.NewNopLogger())
	require.EqualError(t, err, "repeat interval missing")

	ctx = WithRepeatInterval(ctx, time.Hour)

	alerts := []*types.Alert{{}, {}, {}}

	// Must catch notification log query errors.
	s.nflog = &testNflog{
		qerr: errors.New("bad things"),
	}
	ctx, _, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.EqualError(t, err, "bad things")

	// ... but skip ErrNotFound.
	s.nflog = &testNflog{
		qerr: nflog.ErrNotFound,
	}
	ctx, res, err := s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err, "unexpected error on not found log entry")
	require.Equal(t, alerts, res, "input alerts differ from result alerts")

	s.nflog = &testNflog{
		qerr: nil,
		qres: []*nflogpb.Entry{
			{FiringAlerts: []uint64{0, 1, 2}},
			{FiringAlerts: []uint64{1, 2, 3}},
		},
	}
	ctx, _, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.Contains(t, err.Error(), "result size")

	// Must return no error and no alerts no need to update.
	i = 0
	s.nflog = &testNflog{
		qerr: nflog.ErrNotFound,
		qres: []*nflogpb.Entry{
			{
				FiringAlerts: []uint64{0, 1, 2},
				Timestamp:    now,
			},
		},
	}
	ctx, res, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Nil(t, res, "unexpected alerts returned")

	// Must return no error and all input alerts on changes.
	i = 0
	s.nflog = &testNflog{
		qerr: nil,
		qres: []*nflogpb.Entry{
			{
				FiringAlerts: []uint64{1, 2, 3, 4},
				Timestamp:    now,
			},
		},
	}
	_, res, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res, "unexpected alerts returned")
}

func TestMultiStage(t *testing.T) {
	var (
		alerts1 = []*types.Alert{{}}
		alerts2 = []*types.Alert{{}, {}}
		alerts3 = []*types.Alert{{}, {}, {}}
	)

	stage := MultiStage{
		StageFunc(func(ctx context.Context, l *slog.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
			if !reflect.DeepEqual(alerts, alerts1) {
				t.Fatal("Input not equal to input of MultiStage")
			}
			//nolint:staticcheck // Ignore SA1029
			ctx = context.WithValue(ctx, "key", "value")
			return ctx, alerts2, nil
		}),
		StageFunc(func(ctx context.Context, l *slog.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
			if !reflect.DeepEqual(alerts, alerts2) {
				t.Fatal("Input not equal to output of previous stage")
			}
			v, ok := ctx.Value("key").(string)
			if !ok || v != "value" {
				t.Fatalf("Expected value %q for key %q but got %q", "value", "key", v)
			}
			return ctx, alerts3, nil
		}),
	}

	_, alerts, err := stage.Exec(context.Background(), promslog.NewNopLogger(), alerts1...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	if !reflect.DeepEqual(alerts, alerts3) {
		t.Fatal("Output of MultiStage is not equal to the output of the last stage")
	}
}

func TestMultiStageFailure(t *testing.T) {
	var (
		ctx   = context.Background()
		s1    = failStage{}
		stage = MultiStage{s1}
	)

	_, _, err := stage.Exec(ctx, promslog.NewNopLogger(), nil)
	if err.Error() != "some error" {
		t.Fatal("Errors were not propagated correctly by MultiStage")
	}
}

func TestRoutingStage(t *testing.T) {
	var (
		alerts1 = []*types.Alert{{}}
		alerts2 = []*types.Alert{{}, {}}
	)

	stage := RoutingStage{
		"name": StageFunc(func(ctx context.Context, l *slog.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
			if !reflect.DeepEqual(alerts, alerts1) {
				t.Fatal("Input not equal to input of RoutingStage")
			}
			return ctx, alerts2, nil
		}),
		"not": failStage{},
	}

	ctx := WithReceiverName(context.Background(), "name")

	_, alerts, err := stage.Exec(ctx, promslog.NewNopLogger(), alerts1...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	if !reflect.DeepEqual(alerts, alerts2) {
		t.Fatal("Output of RoutingStage is not equal to the output of the inner stage")
	}
}

func TestRetryStageWithError(t *testing.T) {
	fail, retry := true, true
	sent := []*types.Alert{}
	i := Integration{
		notifier: notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
			if fail {
				fail = false
				return retry, errors.New("fail to deliver notification")
			}
			sent = append(sent, alerts...)
			return false, nil
		}),
		rs: sendResolved(false),
	}
	r := NewRetryStage(i, "", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}))

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				EndsAt: time.Now().Add(time.Hour),
			},
		},
	}

	ctx := context.Background()
	ctx = WithFiringAlerts(ctx, []uint64{0})

	// Notify with a recoverable error should retry and succeed.
	resctx, res, err := r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.Equal(t, alerts, sent)
	require.NotNil(t, resctx)

	// Notify with an unrecoverable error should fail.
	sent = sent[:0]
	fail = true
	retry = false
	resctx, _, err = r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.Error(t, err)
	require.NotNil(t, resctx)
}

func TestRetryStageWithErrorCode(t *testing.T) {
	testcases := map[string]struct {
		isNewErrorWithReason bool
		reason               Reason
		reasonlabel          string
		expectedCount        int
	}{
		"for clientError":     {isNewErrorWithReason: true, reason: ClientErrorReason, reasonlabel: ClientErrorReason.String(), expectedCount: 1},
		"for serverError":     {isNewErrorWithReason: true, reason: ServerErrorReason, reasonlabel: ServerErrorReason.String(), expectedCount: 1},
		"for unexpected code": {isNewErrorWithReason: false, reason: DefaultReason, reasonlabel: DefaultReason.String(), expectedCount: 1},
	}
	for _, testData := range testcases {
		retry := false
		testData := testData
		i := Integration{
			name: "test",
			notifier: notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
				if !testData.isNewErrorWithReason {
					return retry, errors.New("fail to deliver notification")
				}
				return retry, NewErrorWithReason(testData.reason, errors.New("fail to deliver notification"))
			}),
			rs: sendResolved(false),
		}
		r := NewRetryStage(i, "", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}))

		alerts := []*types.Alert{
			{
				Alert: model.Alert{
					EndsAt: time.Now().Add(time.Hour),
				},
			},
		}

		ctx := context.Background()
		ctx = WithFiringAlerts(ctx, []uint64{0})

		// Notify with a non-recoverable error.
		resctx, _, err := r.Exec(ctx, promslog.NewNopLogger(), alerts...)
		counter := r.metrics.numTotalFailedNotifications

		require.Equal(t, testData.expectedCount, int(prom_testutil.ToFloat64(counter.WithLabelValues(r.integration.Name(), testData.reasonlabel))))

		require.Error(t, err)
		require.NotNil(t, resctx)
	}
}

func TestRetryStageWithContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	i := Integration{
		name: "test",
		notifier: notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
			cancel()
			return true, errors.New("request failed: context canceled")
		}),
		rs: sendResolved(false),
	}
	r := NewRetryStage(i, "", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}))

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				EndsAt: time.Now().Add(time.Hour),
			},
		},
	}

	ctx = WithFiringAlerts(ctx, []uint64{0})

	// Notify with a non-recoverable error.
	resctx, _, err := r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	counter := r.metrics.numTotalFailedNotifications

	require.Equal(t, 1, int(prom_testutil.ToFloat64(counter.WithLabelValues(r.integration.Name(), ContextCanceledReason.String()))))
	require.Contains(t, err.Error(), "notify retry canceled after 1 attempts: context canceled")

	require.Error(t, err)
	require.NotNil(t, resctx)
}

func TestRetryStageNoResolved(t *testing.T) {
	sent := []*types.Alert{}
	i := Integration{
		notifier: notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
			sent = append(sent, alerts...)
			return false, nil
		}),
		rs: sendResolved(false),
	}
	r := NewRetryStage(i, "", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}))

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				EndsAt: time.Now().Add(-time.Hour),
			},
		},
		{
			Alert: model.Alert{
				EndsAt: time.Now().Add(time.Hour),
			},
		},
	}

	ctx := context.Background()

	resctx, res, err := r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.EqualError(t, err, "firing alerts missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithFiringAlerts(ctx, []uint64{0})

	resctx, res, err = r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.Equal(t, []*types.Alert{alerts[1]}, sent)
	require.NotNil(t, resctx)

	// All alerts are resolved.
	sent = sent[:0]
	ctx = WithFiringAlerts(ctx, []uint64{})
	alerts[1].EndsAt = time.Now().Add(-time.Hour)

	resctx, res, err = r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.Equal(t, []*types.Alert{}, sent)
	require.NotNil(t, resctx)
}

func TestRetryStageSendResolved(t *testing.T) {
	sent := []*types.Alert{}
	i := Integration{
		notifier: notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
			sent = append(sent, alerts...)
			return false, nil
		}),
		rs: sendResolved(true),
	}
	r := NewRetryStage(i, "", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}))

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				EndsAt: time.Now().Add(-time.Hour),
			},
		},
		{
			Alert: model.Alert{
				EndsAt: time.Now().Add(time.Hour),
			},
		},
	}

	ctx := context.Background()
	ctx = WithFiringAlerts(ctx, []uint64{0})

	resctx, res, err := r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.Equal(t, alerts, sent)
	require.NotNil(t, resctx)

	// All alerts are resolved.
	sent = sent[:0]
	ctx = WithFiringAlerts(ctx, []uint64{})
	alerts[1].EndsAt = time.Now().Add(-time.Hour)

	resctx, res, err = r.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.Equal(t, alerts, sent)
	require.NotNil(t, resctx)
}

func TestSetNotifiesStage(t *testing.T) {
	tnflog := &testNflog{}
	s := &SetNotifiesStage{
		recv:  &nflogpb.Receiver{GroupName: "test"},
		nflog: tnflog,
	}
	alerts := []*types.Alert{{}, {}, {}}
	ctx := context.Background()

	resctx, res, err := s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.EqualError(t, err, "group key missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithGroupKey(ctx, "1")

	resctx, res, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.EqualError(t, err, "firing alerts missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithFiringAlerts(ctx, []uint64{0, 1, 2})

	resctx, res, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.EqualError(t, err, "resolved alerts missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithResolvedAlerts(ctx, []uint64{})
	ctx = WithRepeatInterval(ctx, time.Hour)

	tnflog.logFunc = func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, expiry time.Duration) error {
		require.Equal(t, s.recv, r)
		require.Equal(t, "1", gkey)
		require.Equal(t, []uint64{0, 1, 2}, firingAlerts)
		require.Equal(t, []uint64{}, resolvedAlerts)
		require.Equal(t, 2*time.Hour, expiry)
		return nil
	}
	resctx, res, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)

	ctx = WithFiringAlerts(ctx, []uint64{})
	ctx = WithResolvedAlerts(ctx, []uint64{0, 1, 2})

	tnflog.logFunc = func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, expiry time.Duration) error {
		require.Equal(t, s.recv, r)
		require.Equal(t, "1", gkey)
		require.Equal(t, []uint64{}, firingAlerts)
		require.Equal(t, []uint64{0, 1, 2}, resolvedAlerts)
		require.Equal(t, 2*time.Hour, expiry)
		return nil
	}
	resctx, res, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)
}

func TestMuteStage(t *testing.T) {
	// Mute all label sets that have a "mute" key.
	muter := types.MuteFunc(func(lset model.LabelSet) bool {
		_, ok := lset["mute"]
		return ok
	})

	metrics := NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{})
	stage := NewMuteStage(muter, metrics)

	in := []model.LabelSet{
		{},
		{"test": "set"},
		{"mute": "me"},
		{"foo": "bar", "test": "set"},
		{"foo": "bar", "mute": "me"},
		{},
		{"not": "muted"},
	}
	out := []model.LabelSet{
		{},
		{"test": "set"},
		{"foo": "bar", "test": "set"},
		{},
		{"not": "muted"},
	}

	var inAlerts []*types.Alert
	for _, lset := range in {
		inAlerts = append(inAlerts, &types.Alert{
			Alert: model.Alert{Labels: lset},
		})
	}

	_, alerts, err := stage.Exec(context.Background(), promslog.NewNopLogger(), inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	var got []model.LabelSet
	for _, a := range alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, out) {
		t.Fatalf("Muting failed, expected: %v\ngot %v", out, got)
	}
	suppressed := int(prom_testutil.ToFloat64(metrics.numNotificationSuppressedTotal))
	if (len(in) - len(got)) != suppressed {
		t.Fatalf("Expected %d alerts counted in suppressed metric but got %d", (len(in) - len(got)), suppressed)
	}
}

func TestMuteStageWithSilences(t *testing.T) {
	silences, err := silence.New(silence.Options{Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	sil := &silencepb.Silence{
		EndsAt:   utcNow().Add(time.Hour),
		Matchers: []*silencepb.Matcher{{Name: "mute", Pattern: "me"}},
	}
	if err = silences.Set(sil); err != nil {
		t.Fatal(err)
	}

	reg := prometheus.NewRegistry()
	marker := types.NewMarker(reg)
	silencer := silence.NewSilencer(silences, marker, promslog.NewNopLogger())
	metrics := NewMetrics(reg, featurecontrol.NoopFlags{})
	stage := NewMuteStage(silencer, metrics)

	in := []model.LabelSet{
		{},
		{"test": "set"},
		{"mute": "me"},
		{"foo": "bar", "test": "set"},
		{"foo": "bar", "mute": "me"},
		{},
		{"not": "muted"},
	}
	out := []model.LabelSet{
		{},
		{"test": "set"},
		{"foo": "bar", "test": "set"},
		{},
		{"not": "muted"},
	}

	var inAlerts []*types.Alert
	for _, lset := range in {
		inAlerts = append(inAlerts, &types.Alert{
			Alert: model.Alert{Labels: lset},
		})
	}

	// Set the second alert as previously silenced with an old version
	// number. This is expected to get unsilenced by the stage.
	marker.SetActiveOrSilenced(inAlerts[1].Fingerprint(), 0, []string{"123"}, nil)

	_, alerts, err := stage.Exec(context.Background(), promslog.NewNopLogger(), inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	var got []model.LabelSet
	for _, a := range alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, out) {
		t.Fatalf("Muting failed, expected: %v\ngot %v", out, got)
	}
	suppressedRoundOne := int(prom_testutil.ToFloat64(metrics.numNotificationSuppressedTotal))
	if (len(in) - len(got)) != suppressedRoundOne {
		t.Fatalf("Expected %d alerts counted in suppressed metric but got %d", (len(in) - len(got)), suppressedRoundOne)
	}

	// Do it again to exercise the version tracking of silences.
	_, alerts, err = stage.Exec(context.Background(), promslog.NewNopLogger(), inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	got = got[:0]
	for _, a := range alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, out) {
		t.Fatalf("Muting failed, expected: %v\ngot %v", out, got)
	}

	suppressedRoundTwo := int(prom_testutil.ToFloat64(metrics.numNotificationSuppressedTotal))
	if (len(in) - len(got) + suppressedRoundOne) != suppressedRoundTwo {
		t.Fatalf("Expected %d alerts counted in suppressed metric but got %d", (len(in) - len(got)), suppressedRoundTwo)
	}

	// Expire the silence and verify that no alerts are silenced now.
	if err := silences.Expire(sil.Id); err != nil {
		t.Fatal(err)
	}

	_, alerts, err = stage.Exec(context.Background(), promslog.NewNopLogger(), inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}
	got = got[:0]
	for _, a := range alerts {
		got = append(got, a.Labels)
	}

	if !reflect.DeepEqual(got, in) {
		t.Fatalf("Unmuting failed, expected: %v\ngot %v", in, got)
	}
	suppressedRoundThree := int(prom_testutil.ToFloat64(metrics.numNotificationSuppressedTotal))
	if (len(in) - len(got) + suppressedRoundTwo) != suppressedRoundThree {
		t.Fatalf("Expected %d alerts counted in suppressed metric but got %d", (len(in) - len(got)), suppressedRoundThree)
	}
}

func TestTimeMuteStage(t *testing.T) {
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatalf("Failed to load location Australia/Sydney: %s", err)
	}
	eveningsAndWeekends := map[string][]timeinterval.TimeInterval{
		"evenings": {{
			Times: []timeinterval.TimeRange{{
				StartMinute: 0,   // 00:00
				EndMinute:   540, // 09:00
			}, {
				StartMinute: 1020, // 17:00
				EndMinute:   1440, // 24:00
			}},
			Location: &timeinterval.Location{Location: sydney},
		}},
		"weekends": {{
			Weekdays: []timeinterval.WeekdayRange{{
				InclusiveRange: timeinterval.InclusiveRange{Begin: 6, End: 6}, // Saturday
			}, {
				InclusiveRange: timeinterval.InclusiveRange{Begin: 0, End: 0}, // Sunday
			}},
			Location: &timeinterval.Location{Location: sydney},
		}},
	}

	tests := []struct {
		name      string
		intervals map[string][]timeinterval.TimeInterval
		now       time.Time
		alerts    []*types.Alert
		mutedBy   []string
	}{{
		name:      "Should be muted outside working hours",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 1, 0, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"evenings"},
	}, {
		name:      "Should not be muted during workings hours",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 1, 9, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   nil,
	}, {
		name:      "Should be muted during weekends",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 6, 10, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"weekends"},
	}, {
		name:      "Should be muted at 12pm UTC on a weekday",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"evenings"},
	}, {
		name:      "Should be muted at 12pm UTC on a weekend",
		intervals: eveningsAndWeekends,
		now:       time.Date(2024, 1, 6, 10, 0, 0, 0, time.UTC),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"evenings", "weekends"},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			marker := types.NewMarker(r)
			metrics := NewMetrics(r, featurecontrol.NoopFlags{})
			intervener := timeinterval.NewIntervener(test.intervals)
			st := NewTimeMuteStage(intervener, marker, metrics)

			// Get the names of all time intervals for the context.
			muteTimeIntervalNames := make([]string, 0, len(test.intervals))
			for name := range test.intervals {
				muteTimeIntervalNames = append(muteTimeIntervalNames, name)
			}
			// Sort the names so we can compare mutedBy with test.mutedBy.
			sort.Strings(muteTimeIntervalNames)

			ctx := context.Background()
			ctx = WithNow(ctx, test.now)
			ctx = WithGroupKey(ctx, "group1")
			ctx = WithActiveTimeIntervals(ctx, nil)
			ctx = WithMuteTimeIntervals(ctx, muteTimeIntervalNames)
			ctx = WithRouteID(ctx, "route1")

			_, active, err := st.Exec(ctx, promslog.NewNopLogger(), test.alerts...)
			require.NoError(t, err)

			if len(test.mutedBy) == 0 {
				// All alerts should be active.
				require.Len(t, active, len(test.alerts))
				// The group should not be marked.
				mutedBy, isMuted := marker.Muted("route1", "group1")
				require.False(t, isMuted)
				require.Empty(t, mutedBy)
				// The metric for total suppressed notifications should not
				// have been incremented, which means it will not be collected.
				require.NoError(t, prom_testutil.GatherAndCompare(r, strings.NewReader(`
# HELP alertmanager_marked_alerts How many alerts by state are currently marked in the Alertmanager regardless of their expiry.
# TYPE alertmanager_marked_alerts gauge
alertmanager_marked_alerts{state="active"} 0
alertmanager_marked_alerts{state="suppressed"} 0
alertmanager_marked_alerts{state="unprocessed"} 0
`)))
			} else {
				// All alerts should be muted.
				require.Empty(t, active)
				// The group should be marked as muted.
				mutedBy, isMuted := marker.Muted("route1", "group1")
				require.True(t, isMuted)
				require.Equal(t, test.mutedBy, mutedBy)
				// Gets the metric for total suppressed notifications.
				require.NoError(t, prom_testutil.GatherAndCompare(r, strings.NewReader(fmt.Sprintf(`
# HELP alertmanager_marked_alerts How many alerts by state are currently marked in the Alertmanager regardless of their expiry.
# TYPE alertmanager_marked_alerts gauge
alertmanager_marked_alerts{state="active"} 0
alertmanager_marked_alerts{state="suppressed"} 0
alertmanager_marked_alerts{state="unprocessed"} 0
# HELP alertmanager_notifications_suppressed_total The total number of notifications suppressed for being silenced, inhibited, outside of active time intervals or within muted time intervals.
# TYPE alertmanager_notifications_suppressed_total counter
alertmanager_notifications_suppressed_total{reason="mute_time_interval"} %d
`, len(test.alerts)))))
			}
		})
	}
}

func TestTimeActiveStage(t *testing.T) {
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatalf("Failed to load location Australia/Sydney: %s", err)
	}
	weekdays := map[string][]timeinterval.TimeInterval{
		"weekdays": {{
			Weekdays: []timeinterval.WeekdayRange{{
				InclusiveRange: timeinterval.InclusiveRange{
					Begin: 1, // Monday
					End:   5, // Friday
				},
			}},
			Times: []timeinterval.TimeRange{{
				StartMinute: 540,  // 09:00
				EndMinute:   1020, // 17:00
			}},
			Location: &timeinterval.Location{Location: sydney},
		}},
	}

	tests := []struct {
		name      string
		intervals map[string][]timeinterval.TimeInterval
		now       time.Time
		alerts    []*types.Alert
		mutedBy   []string
	}{{
		name:      "Should be muted outside working hours",
		intervals: weekdays,
		now:       time.Date(2024, 1, 1, 0, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"weekdays"},
	}, {
		name:      "Should not be muted during workings hours",
		intervals: weekdays,
		now:       time.Date(2024, 1, 1, 9, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   nil,
	}, {
		name:      "Should be muted during weekends",
		intervals: weekdays,
		now:       time.Date(2024, 1, 6, 10, 0, 0, 0, sydney),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"weekdays"},
	}, {
		name:      "Should be muted at 12pm UTC",
		intervals: weekdays,
		now:       time.Date(2024, 1, 6, 10, 0, 0, 0, time.UTC),
		alerts:    []*types.Alert{{Alert: model.Alert{Labels: model.LabelSet{"foo": "bar"}}}},
		mutedBy:   []string{"weekdays"},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := prometheus.NewRegistry()
			marker := types.NewMarker(r)
			metrics := NewMetrics(r, featurecontrol.NoopFlags{})
			intervener := timeinterval.NewIntervener(test.intervals)
			st := NewTimeActiveStage(intervener, marker, metrics)

			// Get the names of all time intervals for the context.
			activeTimeIntervalNames := make([]string, 0, len(test.intervals))
			for name := range test.intervals {
				activeTimeIntervalNames = append(activeTimeIntervalNames, name)
			}
			// Sort the names so we can compare mutedBy with test.mutedBy.
			sort.Strings(activeTimeIntervalNames)

			ctx := context.Background()
			ctx = WithNow(ctx, test.now)
			ctx = WithGroupKey(ctx, "group1")
			ctx = WithActiveTimeIntervals(ctx, activeTimeIntervalNames)
			ctx = WithMuteTimeIntervals(ctx, nil)
			ctx = WithRouteID(ctx, "route1")

			_, active, err := st.Exec(ctx, promslog.NewNopLogger(), test.alerts...)
			require.NoError(t, err)

			if len(test.mutedBy) == 0 {
				// All alerts should be active.
				require.Len(t, active, len(test.alerts))
				// The group should not be marked.
				mutedBy, isMuted := marker.Muted("route1", "group1")
				require.False(t, isMuted)
				require.Empty(t, mutedBy)
				// The metric for total suppressed notifications should not
				// have been incremented, which means it will not be collected.
				require.NoError(t, prom_testutil.GatherAndCompare(r, strings.NewReader(`
# HELP alertmanager_marked_alerts How many alerts by state are currently marked in the Alertmanager regardless of their expiry.
# TYPE alertmanager_marked_alerts gauge
alertmanager_marked_alerts{state="active"} 0
alertmanager_marked_alerts{state="suppressed"} 0
alertmanager_marked_alerts{state="unprocessed"} 0
`)))
			} else {
				// All alerts should be muted.
				require.Empty(t, active)
				// The group should be marked as muted.
				mutedBy, isMuted := marker.Muted("route1", "group1")
				require.True(t, isMuted)
				require.Equal(t, test.mutedBy, mutedBy)
				// Gets the metric for total suppressed notifications.
				require.NoError(t, prom_testutil.GatherAndCompare(r, strings.NewReader(fmt.Sprintf(`
# HELP alertmanager_marked_alerts How many alerts by state are currently marked in the Alertmanager regardless of their expiry.
# TYPE alertmanager_marked_alerts gauge
alertmanager_marked_alerts{state="active"} 0
alertmanager_marked_alerts{state="suppressed"} 0
alertmanager_marked_alerts{state="unprocessed"} 0
# HELP alertmanager_notifications_suppressed_total The total number of notifications suppressed for being silenced, inhibited, outside of active time intervals or within muted time intervals.
# TYPE alertmanager_notifications_suppressed_total counter
alertmanager_notifications_suppressed_total{reason="active_time_interval"} %d
`, len(test.alerts)))))
			}
		})
	}
}

func BenchmarkHashAlert(b *testing.B) {
	alert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{"foo": "the_first_value", "bar": "the_second_value", "another": "value"},
		},
	}
	for i := 0; i < b.N; i++ {
		hashAlert(alert)
	}
}
