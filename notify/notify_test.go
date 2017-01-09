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

func (s failStage) Exec(ctx context.Context, as ...*types.Alert) (context.Context, []*types.Alert, error) {
	return ctx, nil, fmt.Errorf("some error")
}

type testNflog struct {
	qres []*nflogpb.Entry
	qerr error

	logActiveFunc   func(r *nflogpb.Receiver, gkey, hash []byte) error
	logResolvedFunc func(r *nflogpb.Receiver, gkey, hash []byte) error
}

func (l *testNflog) Query(p ...nflog.QueryParam) ([]*nflogpb.Entry, error) {
	return l.qres, l.qerr
}

func (l *testNflog) LogActive(r *nflogpb.Receiver, gkey, hash []byte) error {
	return l.logActiveFunc(r, gkey, hash)
}

func (l *testNflog) LogResolved(r *nflogpb.Receiver, gkey, hash []byte) error {
	return l.logResolvedFunc(r, gkey, hash)
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

func TestDedupStageNeedsUpdate(t *testing.T) {
	now := utcNow()

	cases := []struct {
		entry    *nflogpb.Entry
		hash     []byte
		resolved bool
		repeat   time.Duration

		res    bool
		resErr bool
	}{
		{
			entry: nil,
			res:   true,
		}, {
			entry: &nflogpb.Entry{GroupHash: []byte{1, 2, 3}},
			hash:  []byte{2, 3, 4},
			res:   true,
		}, {
			entry: &nflogpb.Entry{
				GroupHash: []byte{1, 2, 3},
				Timestamp: nil, // parsing will error
			},
			hash:   []byte{1, 2, 3},
			resErr: true,
		}, {
			entry: &nflogpb.Entry{
				GroupHash: []byte{1, 2, 3},
				Timestamp: mustTimestampProto(now.Add(-9 * time.Minute)),
			},
			repeat: 10 * time.Minute,
			hash:   []byte{1, 2, 3},
			res:    false,
		}, {
			entry: &nflogpb.Entry{
				GroupHash: []byte{1, 2, 3},
				Timestamp: mustTimestampProto(now.Add(-11 * time.Minute)),
			},
			repeat: 10 * time.Minute,
			hash:   []byte{1, 2, 3},
			res:    true,
		},
	}
	for i, c := range cases {
		t.Log("case", i)

		s := &DedupStage{
			now: func() time.Time { return now },
		}
		ok, err := s.needsUpdate(c.entry, c.hash, c.resolved, c.repeat)
		if c.resErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, c.res, ok)
	}
}

func TestDedupStage(t *testing.T) {
	s := &DedupStage{
		hash:     func([]*types.Alert) []byte { return []byte{1, 2, 3} },
		resolved: func([]*types.Alert) bool { return false },
	}

	ctx := context.Background()

	_, _, err := s.Exec(ctx)
	require.EqualError(t, err, "group key missing")

	ctx = WithGroupKey(ctx, 1)

	_, _, err = s.Exec(ctx)
	require.EqualError(t, err, "repeat interval missing")

	ctx = WithRepeatInterval(ctx, time.Hour)

	alerts := []*types.Alert{{}, {}, {}}

	// Must catch notification log query errors.
	s.nflog = &testNflog{
		qerr: errors.New("bad things"),
	}
	ctx, res, err := s.Exec(ctx, alerts...)
	require.EqualError(t, err, "bad things")

	// ... but skip ErrNotFound.
	s.nflog = &testNflog{
		qerr: nflog.ErrNotFound,
	}
	ctx, res, err = s.Exec(ctx, alerts...)
	require.NoError(t, err, "unexpected error on not found log entry")
	require.Equal(t, alerts, res, "input alerts differ from result alerts")
	// The hash must be added to the context.
	hash, ok := NotificationHash(ctx)
	require.True(t, ok, "notification has missing in context")
	require.Equal(t, []byte{1, 2, 3}, hash, "notification hash does not match")

	s.nflog = &testNflog{
		qerr: nil,
		qres: []*nflogpb.Entry{
			{GroupHash: []byte{1, 2, 3}},
			{GroupHash: []byte{2, 3, 4}},
		},
	}
	ctx, res, err = s.Exec(ctx, alerts...)
	require.Contains(t, err.Error(), "result size")

	// Must return no error and no alerts no need to update.
	s.nflog = &testNflog{
		qerr: nflog.ErrNotFound,
	}
	s.resolved = func([]*types.Alert) bool { return true }
	ctx, res, err = s.Exec(ctx, alerts...)
	require.NoError(t, err)
	require.Nil(t, res, "unexpected alerts returned")

	// Must return no error and all input alerts on changes.
	s.nflog = &testNflog{
		qerr: nil,
		qres: []*nflogpb.Entry{
			{GroupHash: []byte{1, 2, 3, 4}},
		},
	}
	ctx, res, err = s.Exec(ctx, alerts...)
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
		StageFunc(func(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
			if !reflect.DeepEqual(alerts, alerts1) {
				t.Fatal("Input not equal to input of MultiStage")
			}
			ctx = context.WithValue(ctx, "key", "value")
			return ctx, alerts2, nil
		}),
		StageFunc(func(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
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

	_, alerts, err := stage.Exec(context.Background(), alerts1...)
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

	_, _, err := stage.Exec(ctx, nil)
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
		"name": StageFunc(func(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
			if !reflect.DeepEqual(alerts, alerts1) {
				t.Fatal("Input not equal to input of RoutingStage")
			}
			return ctx, alerts2, nil
		}),
		"not": failStage{},
	}

	ctx := WithReceiverName(context.Background(), "name")

	_, alerts, err := stage.Exec(ctx, alerts1...)
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

	resctx, res, err := s.Exec(ctx, alerts...)
	require.EqualError(t, err, "notification hash missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithNotificationHash(ctx, []byte{1, 2, 3})

	_, res, err = s.Exec(ctx, alerts...)
	require.EqualError(t, err, "group key missing")
	require.Nil(t, res)
	require.NotNil(t, resctx)

	ctx = WithGroupKey(ctx, 1)

	s.resolved = func([]*types.Alert) bool { return false }
	tnflog.logActiveFunc = func(r *nflogpb.Receiver, gkey, hash []byte) error {
		require.Equal(t, s.recv, r)
		require.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0, 1}, gkey)
		require.Equal(t, []byte{1, 2, 3}, hash)
		return nil
	}
	tnflog.logResolvedFunc = func(r *nflogpb.Receiver, gkey, hash []byte) error {
		t.Fatalf("LogResolved called unexpectedly")
		return nil
	}
	resctx, res, err = s.Exec(ctx, alerts...)
	require.Nil(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)

	s.resolved = func([]*types.Alert) bool { return true }
	tnflog.logActiveFunc = func(r *nflogpb.Receiver, gkey, hash []byte) error {
		t.Fatalf("LogActive called unexpectedly")
		return nil
	}
	tnflog.logResolvedFunc = func(r *nflogpb.Receiver, gkey, hash []byte) error {
		require.Equal(t, s.recv, r)
		require.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0, 1}, gkey)
		require.Equal(t, []byte{1, 2, 3}, hash)
		return nil
	}
	resctx, res, err = s.Exec(ctx, alerts...)
	require.Nil(t, err)
	require.Equal(t, alerts, res)
	require.NotNil(t, resctx)
}

func TestSilenceStage(t *testing.T) {
	silences, err := silence.New(silence.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := silences.Create(&silencepb.Silence{
		EndsAt:   mustTimestampProto(utcNow().Add(time.Hour)),
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

	// Set the second alert als previously silenced. It is expected to have
	// the WasSilenced flag set to true afterwards.
	marker.SetSilenced(inAlerts[1].Fingerprint(), "123")

	_, alerts, err := silencer.Exec(nil, inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	var got []model.LabelSet
	for i, a := range alerts {
		got = append(got, a.Labels)
		if a.WasSilenced != (i == 1) {
			t.Errorf("Expected WasSilenced to be %v for %d, was %v", i == 1, i, a.WasSilenced)
		}
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

	marker := types.NewMarker()
	inhibitor := NewInhibitStage(muter, marker)

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

	// Set the second alert as previously inhibited. It is expected to have
	// the WasInhibited flag set to true afterwards.
	marker.SetInhibited(inAlerts[1].Fingerprint(), true)

	_, alerts, err := inhibitor.Exec(nil, inAlerts...)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

	var got []model.LabelSet
	for i, a := range alerts {
		got = append(got, a.Labels)
		if a.WasInhibited != (i == 1) {
			t.Errorf("Expected WasInhibited to be %v for %d, was %v", i == 1, i, a.WasInhibited)
		}
	}

	if !reflect.DeepEqual(got, out) {
		t.Fatalf("Muting failed, expected: %v\ngot %v", out, got)
	}
}
