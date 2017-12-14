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
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

type notifierConfigFunc func() bool

func (f notifierConfigFunc) SendResolved() bool {
	return f()
}

type notifierFunc func(ctx context.Context, alerts ...*types.Alert) (bool, error)

func (f notifierFunc) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	return f(ctx, alerts...)
}

type failStage struct{}

func (s failStage) Exec(ctx context.Context, l log.Logger, as ...*types.Alert) (context.Context, []*types.Alert, error) {
	return ctx, nil, fmt.Errorf("some error")
}

type testNflog struct {
	qres []*nflogpb.Entry
	qerr error

	logFunc func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64) error
}

func (l *testNflog) Query(p ...nflog.QueryParam) ([]*nflogpb.Entry, error) {
	return l.qres, l.qerr
}

func (l *testNflog) Log(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64) error {
	return l.logFunc(r, gkey, firingAlerts, resolvedAlerts)
}

func (l *testNflog) GC() (int, error) {
	return 0, nil
}

func (l *testNflog) Snapshot(w io.Writer) (int, error) {
	return 0, nil
}

func mustTimestampProto(ts time.Time) *timestamp.Timestamp {
	tspb, err := ptypes.TimestampProto(ts)
	if err != nil {
		panic(err)
	}
	return tspb
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
		entry        *nflogpb.Entry
		firingAlerts map[uint64]struct{}
		repeat       time.Duration

		res    bool
		resErr bool
	}{
		{
			entry:        nil,
			firingAlerts: alertHashSet(2, 3, 4),
			res:          true,
		}, {
			entry:        &nflogpb.Entry{FiringAlerts: []uint64{1, 2, 3}},
			firingAlerts: alertHashSet(2, 3, 4),
			res:          true,
		}, {
			entry: &nflogpb.Entry{
				FiringAlerts: []uint64{1, 2, 3},
				Timestamp:    time.Time{}, // zero timestamp should always update
			},
			firingAlerts: alertHashSet(1, 2, 3),
			res:          true,
		}, {
			entry: &nflogpb.Entry{
				FiringAlerts: []uint64{1, 2, 3},
				Timestamp:    now.Add(-9 * time.Minute),
			},
			repeat:       10 * time.Minute,
			firingAlerts: alertHashSet(1, 2, 3),
			res:          false,
		}, {
			entry: &nflogpb.Entry{
				FiringAlerts: []uint64{1, 2, 3},
				Timestamp:    now.Add(-11 * time.Minute),
			},
			repeat:       10 * time.Minute,
			firingAlerts: alertHashSet(1, 2, 3),
			res:          true,
		},
	}
	for i, c := range cases {
		t.Log("case", i)

		s := &DedupStage{
			now: func() time.Time { return now },
		}
		ok, err := s.needsUpdate(c.entry, c.firingAlerts, nil, c.repeat)
		if c.resErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, c.res, ok)
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
	}

	ctx := context.Background()

	_, _, err := s.Exec(ctx, log.NewNopLogger())
	require.EqualError(t, err, "group key missing")

	ctx = WithGroupKey(ctx, "1")

	_, _, err = s.Exec(ctx, log.NewNopLogger())
	require.EqualError(t, err, "repeat interval missing")

	ctx = WithRepeatInterval(ctx, time.Hour)

	alerts := []*types.Alert{{}, {}, {}}

	// Must catch notification log query errors.
	s.nflog = &testNflog{
		qerr: errors.New("bad things"),
	}
	ctx, res, err := s.Exec(ctx, log.NewNopLogger(), alerts...)
	require.EqualError(t, err, "bad things")

	// ... but skip ErrNotFound.
	s.nflog = &testNflog{
		qerr: nflog.ErrNotFound,
	}
	ctx, res, err = s.Exec(ctx, log.NewNopLogger(), alerts...)
	require.NoError(t, err, "unexpected error on not found log entry")
	require.Equal(t, alerts, res, "input alerts differ from result alerts")

	s.nflog = &testNflog{
		qerr: nil,
		qres: []*nflogpb.Entry{
			{FiringAlerts: []uint64{0, 1, 2}},
			{FiringAlerts: []uint64{1, 2, 3}},
		},
	}
	ctx, res, err = s.Exec(ctx, log.NewNopLogger(), alerts...)
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
	ctx, res, err = s.Exec(ctx, log.NewNopLogger(), alerts...)
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
	ctx, res, err = s.Exec(ctx, log.NewNopLogger(), alerts...)
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
		StageFunc(func(ctx context.Context, l log.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
			if !reflect.DeepEqual(alerts, alerts1) {
				t.Fatal("Input not equal to input of MultiStage")
			}
			ctx = context.WithValue(ctx, "key", "value")
			return ctx, alerts2, nil
		}),
		StageFunc(func(ctx context.Context, l log.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
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

	_, alerts, err := stage.Exec(context.Background(), log.NewNopLogger(), alerts1...)
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

	_, _, err := stage.Exec(ctx, log.NewNopLogger(), nil)
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
		"name": StageFunc(func(ctx context.Context, l log.Logger, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
			if !reflect.DeepEqual(alerts, alerts1) {
				t.Fatal("Input not equal to input of RoutingStage")
			}
			return ctx, alerts2, nil
		}),
		"not": failStage{},
	}

	ctx := WithReceiverName(context.Background(), "name")

	_, alerts, err := stage.Exec(ctx, log.NewNopLogger(), alerts1...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	if !reflect.DeepEqual(alerts, alerts2) {
		t.Fatal("Output of RoutingStage is not equal to the output of the inner stage")
	}
}

func TestIntegrationNoResolved(t *testing.T) {
	res := []*types.Alert{}
	r := notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
		res = append(res, alerts...)

		return false, nil
	})
	i := Integration{
		notifier: r,
		conf:     notifierConfigFunc(func() bool { return false }),
	}

	alerts := []*types.Alert{
		&types.Alert{
			Alert: model.Alert{
				EndsAt: time.Now().Add(-time.Hour),
			},
		},
		&types.Alert{
			Alert: model.Alert{
				EndsAt: time.Now().Add(time.Hour),
			},
		},
	}

	i.Notify(nil, alerts...)

	require.Equal(t, len(res), 1)
}

func TestIntegrationSendResolved(t *testing.T) {
	res := []*types.Alert{}
	r := notifierFunc(func(ctx context.Context, alerts ...*types.Alert) (bool, error) {
		res = append(res, alerts...)

		return false, nil
	})
	i := Integration{
		notifier: r,
		conf:     notifierConfigFunc(func() bool { return true }),
	}

	alerts := []*types.Alert{
		&types.Alert{
			Alert: model.Alert{
				EndsAt: time.Now().Add(-time.Hour),
			},
		},
	}

	i.Notify(nil, alerts...)

	require.Equal(t, len(res), 1)
	require.Equal(t, res, alerts)
}

func TestSetNotifiesStage(t *testing.T) {
	tnflog := &testNflog{}
	s := &SetNotifiesStage{
		recv:  &nflogpb.Receiver{GroupName: "test"},
		nflog: tnflog,
	}
	alerts := []*types.Alert{{}, {}, {}}
	ctx := context.Background()

	resctx, res, err := s.Exec(ctx, log.NewNopLogger(), alerts...)
	require.EqualError(t, err, "group key missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithGroupKey(ctx, "1")

	resctx, res, err = s.Exec(ctx, log.NewNopLogger(), alerts...)
	require.EqualError(t, err, "firing alerts missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithFiringAlerts(ctx, []uint64{0, 1, 2})

	resctx, res, err = s.Exec(ctx, log.NewNopLogger(), alerts...)
	require.EqualError(t, err, "resolved alerts missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithResolvedAlerts(ctx, []uint64{})

	tnflog.logFunc = func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64) error {
		require.Equal(t, s.recv, r)
		require.Equal(t, "1", gkey)
		require.Equal(t, []uint64{0, 1, 2}, firingAlerts)
		require.Equal(t, []uint64{}, resolvedAlerts)
		return nil
	}
	resctx, res, err = s.Exec(ctx, log.NewNopLogger(), alerts...)
	require.Nil(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)

	ctx = WithFiringAlerts(ctx, []uint64{})
	ctx = WithResolvedAlerts(ctx, []uint64{0, 1, 2})

	tnflog.logFunc = func(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64) error {
		require.Equal(t, s.recv, r)
		require.Equal(t, "1", gkey)
		require.Equal(t, []uint64{}, firingAlerts)
		require.Equal(t, []uint64{0, 1, 2}, resolvedAlerts)
		return nil
	}
	resctx, res, err = s.Exec(ctx, log.NewNopLogger(), alerts...)
	require.Nil(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)
}

func TestSilenceStage(t *testing.T) {
	silences, err := silence.New(silence.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := silences.Set(&silencepb.Silence{
		EndsAt:   utcNow().Add(time.Hour),
		Matchers: []*silencepb.Matcher{{Name: "mute", Pattern: "me"}},
	}); err != nil {
		t.Fatal(err)
	}

	marker := types.NewMarker()
	silencer := NewSilenceStage(silences, marker)

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

	// Set the second alert as previously silenced. It is expected to have
	// the WasSilenced flag set to true afterwards.
	marker.SetSilenced(inAlerts[1].Fingerprint(), "123")

	_, alerts, err := silencer.Exec(nil, log.NewNopLogger(), inAlerts...)
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
}

func TestInhibitStage(t *testing.T) {
	// Mute all label sets that have a "mute" key.
	muter := types.MuteFunc(func(lset model.LabelSet) bool {
		_, ok := lset["mute"]
		return ok
	})

	inhibitor := NewInhibitStage(muter)

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

	_, alerts, err := inhibitor.Exec(nil, log.NewNopLogger(), inAlerts...)
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
}
