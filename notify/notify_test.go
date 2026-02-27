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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
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

	logFunc func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, receiverData *nflog.Store, expiry time.Duration) error
}

func (l *testNflog) Query(p ...nflog.QueryParam) ([]*nflogpb.Entry, error) {
	return l.qres, l.qerr
}

func (l *testNflog) Log(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, receiverData *nflog.Store, expiry time.Duration) error {
	return l.logFunc(r, gkey, firingAlerts, resolvedAlerts, receiverData, expiry)
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
				Timestamp:    &timestamppb.Timestamp{},
			},
			firingAlerts: alertHashSet(1, 2, 3),
			res:          true,
		}, {
			// Identical sets of alerts shouldn't update before repeat_interval.
			entry: &nflogpb.Entry{
				FiringAlerts: []uint64{1, 2, 3},
				Timestamp:    timestamppb.New(now.Add(-9 * time.Minute)),
			},
			repeat:       10 * time.Minute,
			firingAlerts: alertHashSet(1, 2, 3),
			res:          false,
		}, {
			// Identical sets of alerts should update after repeat_interval.
			entry: &nflogpb.Entry{
				FiringAlerts: []uint64{1, 2, 3},
				Timestamp:    timestamppb.New(now.Add(-11 * time.Minute)),
			},
			repeat:       10 * time.Minute,
			firingAlerts: alertHashSet(1, 2, 3),
			res:          true,
		}, {
			// Different sets of resolved alerts without firing alerts shouldn't update after repeat_interval.
			entry: &nflogpb.Entry{
				ResolvedAlerts: []uint64{1, 2, 3},
				Timestamp:      timestamppb.New(now.Add(-11 * time.Minute)),
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
				Timestamp:      timestamppb.New(now.Add(-9 * time.Minute)),
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
				Timestamp:      timestamppb.New(now.Add(-9 * time.Minute)),
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
				Timestamp:      timestamppb.New(now.Add(-9 * time.Minute)),
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
				Timestamp:      timestamppb.New(now.Add(-9 * time.Minute)),
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
		res := s.needsUpdate(c.entry, c.firingAlerts, c.resolvedAlerts, c.repeat).shouldNotify()
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
	reason, ok := NotificationReason(ctx)
	require.True(t, ok, "NotificationReason should be in context")
	require.Equal(t, ReasonFirstNotification, reason, "should be first notification")

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
				Timestamp:    timestamppb.New(now),
			},
		},
	}
	ctx, res, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Nil(t, res, "unexpected alerts returned")
	reason, ok = NotificationReason(ctx)
	require.True(t, ok, "NotificationReason should be in context")
	require.Equal(t, ReasonDoNotNotify, reason, "should not notify when nothing changed")

	// Must return no error and all input alerts on changes.
	i = 0
	s.nflog = &testNflog{
		qerr: nil,
		qres: []*nflogpb.Entry{
			{
				FiringAlerts: []uint64{1, 2, 3, 4},
				Timestamp:    timestamppb.New(now),
			},
		},
	}
	ctx, res, err = s.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)
	require.Equal(t, alerts, res, "unexpected alerts returned")
	reason, ok = NotificationReason(ctx)
	require.True(t, ok, "NotificationReason should be in context")
	require.Equal(t, ReasonNewAlertsInGroup, reason, "should notify when alerts change")
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

	tnflog.logFunc = func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, receiverData *nflog.Store, expiry time.Duration) error {
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

	tnflog.logFunc = func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, receiverData *nflog.Store, expiry time.Duration) error {
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

func TestReceiverData_PreservationWhenNotifierDoesNotUpdate(t *testing.T) {
	var storedData *nflog.Store
	callCount := 0

	tnflog := &testNflog{
		logFunc: func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, receiverData *nflog.Store, expiry time.Duration) error {
			storedData = receiverData
			return nil
		},
	}

	tnflog.qres = []*nflogpb.Entry{}

	recv := &nflogpb.Receiver{GroupName: "test"}
	dedupStage := NewDedupStage(sendResolved(true), tnflog, recv)

	notifier := notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
		callCount++

		if callCount == 1 {
			// First call - store some data
			if store, ok := NflogStore(ctx); ok {
				store.SetStr("threadTs", "1234.5678")
			}
			return false, nil
		}
		// Second call - notifier doesn't update ReceiverData
		// Does NOT call StoreStr - just returns success
		return false, nil
	})

	integration := NewIntegration(notifier, sendResolved(true), "test", 0, "test-receiver")
	retryStage := NewRetryStage(integration, "test", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}))
	setNotifiesStage := NewSetNotifiesStage(tnflog, recv)

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "testkey")
	ctx = WithRepeatInterval(ctx, time.Hour)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "test"},
			},
		},
	}

	// First notification
	ctx, _, err := dedupStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	ctx, _, err = retryStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	_, _, err = setNotifiesStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	// Verify first notification stored data
	require.NotNil(t, storedData)
	threadTs, found := storedData.GetStr("threadTs")
	require.True(t, found, "threadTs should be in stored data")
	require.Equal(t, "1234.5678", threadTs)

	firstReceiverData := map[string]*nflogpb.ReceiverDataValue{
		"threadTs": {
			Value: &nflogpb.ReceiverDataValue_StrVal{StrVal: "1234.5678"},
		},
	}

	// Second notification - load previous state
	tnflog.qres = []*nflogpb.Entry{
		{
			Receiver:       recv,
			GroupKey:       []byte("testkey"),
			FiringAlerts:   []uint64{1},
			ResolvedAlerts: []uint64{},
			ReceiverData:   firstReceiverData,
		},
	}

	ctx = context.Background()
	ctx = WithGroupKey(ctx, "testkey")
	ctx = WithRepeatInterval(ctx, time.Hour)

	ctx, _, err = dedupStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	ctx, _, err = retryStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	_, _, err = setNotifiesStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	if storedData == nil {
		t.Error("ReceiverData was lost! Second notification has nil data")
	} else {
		if threadTs, exists := storedData.GetStr("threadTs"); !exists {
			t.Error("ReceiverData 'threadTs' was lost! Second notification doesn't have it")
		} else {
			t.Logf("threadTs value: %s", threadTs)
		}
	}
}

func TestDedupStageExtractsReceiverData_DataPresent(t *testing.T) {
	receiverData := map[string]*nflogpb.ReceiverDataValue{
		"threadTs": {
			Value: &nflogpb.ReceiverDataValue_StrVal{StrVal: "1234.5678"},
		},
		"counter": {
			Value: &nflogpb.ReceiverDataValue_IntVal{IntVal: 42},
		},
	}

	entry := &nflogpb.Entry{
		Receiver:     &nflogpb.Receiver{GroupName: "test"},
		GroupKey:     []byte("key"),
		FiringAlerts: []uint64{1, 2, 3},
		ReceiverData: receiverData,
	}

	tnflog := &testNflog{
		qres: []*nflogpb.Entry{entry},
	}

	stage := NewDedupStage(sendResolved(false), tnflog, &nflogpb.Receiver{GroupName: "test"})

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "key")
	ctx = WithRepeatInterval(ctx, time.Hour)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "test"},
			},
		},
	}

	resCtx, _, err := stage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	store, ok := NflogStore(resCtx)
	require.True(t, ok, "NflogStore should be in context")
	require.NotNil(t, store)

	threadTs, found := store.GetStr("threadTs")
	require.True(t, found)
	require.Equal(t, "1234.5678", threadTs)

	counter, found := store.GetInt("counter")
	require.True(t, found)
	require.Equal(t, int64(42), counter)
}

func TestDedupStageExtractsReceiverData_NilReceiverData(t *testing.T) {
	entry := &nflogpb.Entry{
		Receiver:     &nflogpb.Receiver{GroupName: "test"},
		GroupKey:     []byte("key"),
		FiringAlerts: []uint64{1, 2, 3},
		ReceiverData: nil,
	}

	tnflog := &testNflog{
		qres: []*nflogpb.Entry{entry},
	}

	stage := NewDedupStage(sendResolved(false), tnflog, &nflogpb.Receiver{GroupName: "test"})

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "key")
	ctx = WithRepeatInterval(ctx, time.Hour)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "test"},
			},
		},
	}

	resCtx, _, err := stage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	store, ok := NflogStore(resCtx)
	require.True(t, ok, "NflogStore should be in context even when ReceiverData is nil")
	require.NotNil(t, store)
}

func TestDedupStageExtractsReceiverData_NoEntry(t *testing.T) {
	tnflog := &testNflog{
		qres: []*nflogpb.Entry{},
	}

	stage := NewDedupStage(sendResolved(false), tnflog, &nflogpb.Receiver{GroupName: "test"})

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "key")
	ctx = WithRepeatInterval(ctx, time.Hour)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "test"},
			},
		},
	}

	resCtx, _, err := stage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	store, ok := NflogStore(resCtx)
	require.True(t, ok, "NflogStore should be in context even when no entry exists")
	require.NotNil(t, store)
}

func TestNflogStore_NoLeakBetweenNotificationSequences(t *testing.T) {
	var storedData *nflog.Store
	callCount := 0
	var capturedStoreValues []map[string]string

	tnflog := &testNflog{
		logFunc: func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, receiverData *nflog.Store, expiry time.Duration) error {
			storedData = receiverData
			return nil
		},
	}

	recv := &nflogpb.Receiver{GroupName: "test"}
	dedupStage := NewDedupStage(sendResolved(true), tnflog, recv)

	notifier := notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
		callCount++
		store, ok := NflogStore(ctx)
		require.True(t, ok, "Store should be available in context")

		storeSnapshot := make(map[string]string)
		if val, found := store.GetStr("session_data"); found {
			storeSnapshot["session_data"] = val
		}
		capturedStoreValues = append(capturedStoreValues, storeSnapshot)

		store.SetStr("session_data", fmt.Sprintf("session_%d", callCount))
		return false, nil
	})

	integration := NewIntegration(notifier, sendResolved(true), "test", 0, "test-receiver")
	retryStage := NewRetryStage(integration, "test", NewMetrics(prometheus.NewRegistry(), featurecontrol.NoopFlags{}))
	setNotifiesStage := NewSetNotifiesStage(tnflog, recv)

	alerts := []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{"alertname": "test"},
				EndsAt: time.Now().Add(time.Hour),
			},
		},
	}

	// Scenario 1: First notification ever (no previous nflog entry)
	tnflog.qres = []*nflogpb.Entry{}

	ctx := context.Background()
	ctx = WithGroupKey(ctx, "testkey")
	ctx = WithRepeatInterval(ctx, time.Hour)

	ctx, _, err := dedupStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	ctx, _, err = retryStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	_, _, err = setNotifiesStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	require.Equal(t, 1, callCount)
	require.Empty(t, capturedStoreValues[0], "First notification should see empty Store")

	require.NotNil(t, storedData)
	sessionData, found := storedData.GetStr("session_data")
	require.True(t, found)
	require.Equal(t, "session_1", sessionData)

	// Scenario 2: Alert resolves, then fires again (new firing sequence)
	firstSessionData := map[string]*nflogpb.ReceiverDataValue{
		"session_data": {
			Value: &nflogpb.ReceiverDataValue_StrVal{StrVal: "session_1"},
		},
	}

	tnflog.qres = []*nflogpb.Entry{
		{
			Receiver:       recv,
			GroupKey:       []byte("testkey"),
			FiringAlerts:   []uint64{},
			ResolvedAlerts: []uint64{1},
			ReceiverData:   firstSessionData,
		},
	}

	ctx = context.Background()
	ctx = WithGroupKey(ctx, "testkey")
	ctx = WithRepeatInterval(ctx, time.Hour)

	ctx, _, err = dedupStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	ctx, _, err = retryStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	_, _, err = setNotifiesStage.Exec(ctx, promslog.NewNopLogger(), alerts...)
	require.NoError(t, err)

	require.Equal(t, 2, callCount)
	require.Len(t, capturedStoreValues, 2)
	require.Empty(t, capturedStoreValues[1], "New firing sequence should see empty Store (no leak from resolved entry)")

	require.NotNil(t, storedData)
	sessionData, found = storedData.GetStr("session_data")
	require.True(t, found)
	require.Equal(t, "session_2", sessionData)
}

func BenchmarkHashAlert(b *testing.B) {
	alert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{"foo": "the_first_value", "bar": "the_second_value", "another": "value"},
		},
	}
	for b.Loop() {
		hashAlert(alert)
	}
}
