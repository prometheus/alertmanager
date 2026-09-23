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

package mem

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
)

// testAlerts holds three alerts that start "now" and end 100ms later. It is
// built inside each test rather than at package init so that tests running in
// a synctest bubble see timestamps from the bubble's clock.
type testAlerts struct {
	t0, t1                 time.Time
	alert1, alert2, alert3 *types.Alert
}

func newTestAlerts() testAlerts {
	t0 := time.Now()
	t1 := t0.Add(100 * time.Millisecond)
	newAlert := func(labels, annotations model.LabelSet) *types.Alert {
		return &types.Alert{
			Alert: model.Alert{
				Labels:       labels,
				Annotations:  annotations,
				StartsAt:     t0,
				EndsAt:       t1,
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: t0,
			Timeout:   false,
		}
	}
	return testAlerts{
		t0:     t0,
		t1:     t1,
		alert1: newAlert(model.LabelSet{"bar": "foo"}, model.LabelSet{"foo": "bar"}),
		alert2: newAlert(model.LabelSet{"bar": "foo2"}, model.LabelSet{"foo": "bar2"}),
		alert3: newAlert(model.LabelSet{"bar": "foo3"}, model.LabelSet{"foo": "bar3"}),
	}
}

// TestAlertsSubscribePutStarvation tests starvation of `iterator.Close` and
// `alerts.Put`. Both `Subscribe` and `Put` use the Alerts.mtx lock. `Subscribe`
// needs it to subscribe and more importantly unsubscribe `Alerts.listeners`. `Put`
// uses the lock to add additional alerts and iterate the `Alerts.listeners` map.
// If the channel of a listener is at its limit, `alerts.Lock` is blocked, whereby
// a listener can not unsubscribe as the lock is hold by `alerts.Lock`.
func TestAlertsSubscribePutStarvation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newTestAlerts()
		alerts, err := NewAlerts(t.Context(), 30*time.Minute, 0, noopCallback{}, promslog.NewNopLogger(), eventrecorder.NopRecorder(), prometheus.NewRegistry(), nil)
		require.NoError(t, err)

		iterator := alerts.Subscribe("test")

		alertsToInsert := []*types.Alert{}
		// Exhaust alert channel
		for i := range alertChannelLength + 1 {
			alertsToInsert = append(alertsToInsert, &types.Alert{
				Alert: model.Alert{
					// Make sure the fingerprints differ
					Labels:       model.LabelSet{"iteration": model.LabelValue(strconv.Itoa(i))},
					Annotations:  model.LabelSet{"foo": "bar"},
					StartsAt:     f.t0,
					EndsAt:       f.t1,
					GeneratorURL: "http://example.com/prometheus",
				},
				UpdatedAt: f.t0,
				Timeout:   false,
			})
		}

		putDone := make(chan error, 1)
		go func() {
			putDone <- alerts.Put(context.Background(), alertsToInsert...)
		}()

		// Wait until Put is blocked on the full subscriber channel, then close
		// the iterator: neither side may starve the other.
		synctest.Wait()
		iterator.Close()

		select {
		case err := <-putDone:
			require.NoError(t, err)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected `alerts.Put` and `iterator.Close` not to starve each other")
		}
	})
}

func TestDeadLock(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(5 * time.Second)

	// Run gc every 5 milliseconds to increase the possibility of a deadlock with Subscribe()
	alerts, err := NewAlerts(context.Background(), 5*time.Millisecond, 0, noopCallback{}, promslog.NewNopLogger(), eventrecorder.NopRecorder(), prometheus.NewRegistry(), nil)
	if err != nil {
		t.Fatal(err)
	}
	alertsToInsert := []*types.Alert{}
	for i := range 200 + 1 {
		alertsToInsert = append(alertsToInsert, &types.Alert{
			Alert: model.Alert{
				// Make sure the fingerprints differ
				Labels:       model.LabelSet{"iteration": model.LabelValue(strconv.Itoa(i))},
				Annotations:  model.LabelSet{"foo": "bar"},
				StartsAt:     t0,
				EndsAt:       t1,
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: t0,
			Timeout:   false,
		})
	}

	if err := alerts.Put(context.Background(), alertsToInsert...); err != nil {
		t.Fatal("Unable to add alerts")
	}
	done := make(chan bool)

	// call subscribe repeatedly in a goroutine to increase
	// the possibility of a deadlock occurring
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		stopAfter := time.After(1 * time.Second)
		for {
			select {
			case <-tick.C:
				alerts.Subscribe("test")
			case <-stopAfter:
				done <- true
				break
			}
		}
	}()

	select {
	case <-done:
		// no deadlock
		alerts.Close()
	case <-time.After(10 * time.Second):
		t.Error("Deadlock detected")
	}
}

func TestAlertsPut(t *testing.T) {
	f := newTestAlerts()
	alerts, err := NewAlerts(context.Background(), 30*time.Minute, 0, noopCallback{}, promslog.NewNopLogger(), eventrecorder.NopRecorder(), prometheus.NewRegistry(), nil)
	if err != nil {
		t.Fatal(err)
	}

	insert := []*types.Alert{f.alert1, f.alert2, f.alert3}

	if err := alerts.Put(context.Background(), insert...); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	for i, a := range insert {
		res, err := alerts.Get(a.Fingerprint())
		if err != nil {
			t.Fatalf("retrieval error: %s", err)
		}
		require.NoError(t, alertDiff(a, res), "unexpected alert: %d", i)
	}
}

func TestAlertsSubscribe(t *testing.T) {
	ctx := t.Context()
	f := newTestAlerts()
	alerts, err := NewAlerts(ctx, 30*time.Minute, 0, noopCallback{}, promslog.NewNopLogger(), eventrecorder.NopRecorder(), prometheus.NewRegistry(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Add alert1 to validate if pending alerts will be sent.
	if err := alerts.Put(ctx, f.alert1); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	expectedAlerts := map[model.Fingerprint]*types.Alert{
		f.alert1.Fingerprint(): f.alert1,
		f.alert2.Fingerprint(): f.alert2,
		f.alert3.Fingerprint(): f.alert3,
	}

	// Start many consumers and make sure that each receives all the subsequent alerts.
	var (
		nb     = 100
		fatalc = make(chan string, nb)
		wg     sync.WaitGroup
	)
	wg.Add(nb)
	for i := range nb {
		go func(i int) {
			defer wg.Done()

			it := alerts.Subscribe("test")
			defer it.Close()

			received := make(map[model.Fingerprint]struct{})
			for {
				select {
				case got, ok := <-it.Next():
					if !ok {
						fatalc <- fmt.Sprintf("Iterator %d closed", i)
						return
					}
					if it.Err() != nil {
						fatalc <- fmt.Sprintf("Iterator %d: %v", i, it.Err())
						return
					}
					expected := expectedAlerts[got.Data.Fingerprint()]
					if err := alertDiff(got.Data, expected); err != nil {
						fatalc <- fmt.Sprintf("Unexpected alert (iterator %d)\n%s", i, err.Error())
						return
					}
					received[got.Data.Fingerprint()] = struct{}{}
					if len(received) == len(expectedAlerts) {
						return
					}
				case <-time.After(5 * time.Second):
					fatalc <- fmt.Sprintf("Unexpected number of alerts for iterator %d, got: %d, expected: %d", i, len(received), len(expectedAlerts))
					return
				}
			}
		}(i)
	}

	// Add more alerts that should be received by the subscribers.
	if err := alerts.Put(ctx, f.alert2); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}
	if err := alerts.Put(ctx, f.alert3); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	wg.Wait()
	close(fatalc)
	fatal, ok := <-fatalc
	if ok {
		t.Fatal(fatal)
	}
}

func TestAlertsGetPending(t *testing.T) {
	f := newTestAlerts()
	alerts, err := NewAlerts(context.Background(), 30*time.Minute, 0, noopCallback{}, promslog.NewNopLogger(), eventrecorder.NopRecorder(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := alerts.Put(ctx, f.alert1, f.alert2); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	expectedAlerts := map[model.Fingerprint]*types.Alert{
		f.alert1.Fingerprint(): f.alert1,
		f.alert2.Fingerprint(): f.alert2,
	}
	iterator := alerts.GetPending()
	for actual := range iterator.Next() {
		expected := expectedAlerts[actual.Data.Fingerprint()]
		require.NoError(t, alertDiff(actual.Data, expected))
	}

	if err := alerts.Put(ctx, f.alert3); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	expectedAlerts = map[model.Fingerprint]*types.Alert{
		f.alert1.Fingerprint(): f.alert1,
		f.alert2.Fingerprint(): f.alert2,
		f.alert3.Fingerprint(): f.alert3,
	}
	iterator = alerts.GetPending()
	for actual := range iterator.Next() {
		expected := expectedAlerts[actual.Data.Fingerprint()]
		require.NoError(t, alertDiff(actual.Data, expected))
	}
}

func TestAlertsGC(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newTestAlerts()
		alerts, err := NewAlerts(t.Context(), 200*time.Millisecond, 0, noopCallback{}, promslog.NewNopLogger(), eventrecorder.NopRecorder(), nil, nil)
		require.NoError(t, err)

		insert := []*types.Alert{f.alert1, f.alert2, f.alert3}
		require.NoError(t, alerts.Put(context.Background(), insert...))

		// The alerts end after 100ms and the GC runs every 200ms, so one GC
		// round has removed them by now (time is virtual in the bubble).
		time.Sleep(300 * time.Millisecond)
		synctest.Wait()

		for i, a := range insert {
			_, err := alerts.Get(a.Fingerprint())
			require.ErrorIs(t, err, store.ErrNotFound, "alert %d didn't get GC'd", i)
		}
	})
}

func TestAlertsStoreCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cb := &limitCountCallback{limit: 3}
		f := newTestAlerts()

		alerts, err := NewAlerts(t.Context(), 200*time.Millisecond, 0, cb, promslog.NewNopLogger(), eventrecorder.NopRecorder(), nil, nil)
		require.NoError(t, err)

		ctx := context.Background()
		require.NoError(t, alerts.Put(ctx, f.alert1, f.alert2, f.alert3))
		require.Equal(t, int32(3), cb.alerts.Load(), "unexpected number of alerts in the store")

		alert1Mod := *f.alert1
		alert1Mod.Annotations = model.LabelSet{"foo": "bar", "new": "test"} // Update annotations for alert1

		alert4 := &types.Alert{
			Alert: model.Alert{
				Labels:       model.LabelSet{"bar4": "foo4"},
				Annotations:  model.LabelSet{"foo4": "bar4"},
				StartsAt:     f.t0,
				EndsAt:       f.t1,
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: f.t0,
			Timeout:   false,
		}

		// The new alert is rejected by the callback, which is not reported as
		// an error but only visible through the callback's count.
		require.NoError(t, alerts.Put(ctx, &alert1Mod, alert4))
		require.Equal(t, int32(3), cb.alerts.Load(), "unexpected number of alerts in the store")

		// But we still managed to update alert1, since callback doesn't report error when updating existing alert.
		a, err := alerts.Get(f.alert1.Fingerprint())
		require.NoError(t, err)
		require.NoError(t, alertDiff(a, &alert1Mod))

		// Now wait until existing alerts are GC-ed, and make sure that callback was called.
		time.Sleep(300 * time.Millisecond)
		synctest.Wait()
		require.Equal(t, int32(0), cb.alerts.Load(), "unexpected number of alerts in the store")

		require.NoError(t, alerts.Put(ctx, alert4))
	})
}

func alertDiff(left, right *types.Alert) error {
	if left == nil || right == nil {
		return errors.New("should not be nil")
	}
	comparisons := []struct {
		name     string
		isEqual  bool
		expected any
		got      any
	}{
		{"Labels", reflect.DeepEqual(right.Labels, left.Labels), right.Labels, left.Labels},
		{"Annotations", reflect.DeepEqual(right.Annotations, left.Annotations), right.Annotations, left.Annotations},
		{"StartsAt", right.StartsAt.Equal(left.StartsAt), right.StartsAt, left.StartsAt},
		{"EndsAt", right.EndsAt.Equal(left.EndsAt), right.EndsAt, left.EndsAt},
		{"UpdatedAt", right.UpdatedAt.Equal(left.UpdatedAt), right.UpdatedAt, left.UpdatedAt},
		{"GeneratorURL", right.GeneratorURL == left.GeneratorURL, right.GeneratorURL, left.GeneratorURL},
		{"Timeout", right.Timeout == left.Timeout, right.Timeout, left.Timeout},
	}
	var errs []error
	for _, comp := range comparisons {
		if !comp.isEqual {
			errs = append(errs, fmt.Errorf("field `%s` mismatch.\n Expected: %v\n Got: %v", comp.name, comp.expected, comp.got))
		}
	}
	return errors.Join(errs...)
}

type limitCountCallback struct {
	alerts  atomic.Int32
	gcCount atomic.Int32
	limit   int
}

var errTooManyAlerts = fmt.Errorf("too many alerts")

func (l *limitCountCallback) PreStore(_ *types.Alert, existing bool) error {
	if existing {
		return nil
	}

	if int(l.alerts.Load())+1 > l.limit {
		return errTooManyAlerts
	}

	return nil
}

func (l *limitCountCallback) PostStore(_ *types.Alert, existing bool) {
	if !existing {
		l.alerts.Add(1)
		l.gcCount.Add(1)
	}
}

func (l *limitCountCallback) PostDelete(_ *types.Alert) {
	l.alerts.Add(-1)
}

func (l *limitCountCallback) PostGC(fingerprints model.Fingerprints) {
	l.gcCount.Add(-int32(fingerprints.Len()))
}

func TestAlertsConcurrently(t *testing.T) {
	callback := &limitCountCallback{limit: 100}
	a, err := NewAlerts(context.Background(), time.Millisecond, 0, callback, promslog.NewNopLogger(), eventrecorder.NopRecorder(), nil, nil)
	require.NoError(t, err)

	stopc := make(chan struct{})
	failc := make(chan struct{})
	go func() {
		time.Sleep(2 * time.Second)
		close(stopc)
	}()
	expire := 10 * time.Millisecond
	wg := sync.WaitGroup{}
	for range 100 {
		wg.Go(func() {
			j := 0
			for {
				select {
				case <-failc:
					return
				case <-stopc:
					return
				default:
				}
				now := time.Now()
				err := a.Put(context.Background(), &types.Alert{
					Alert: model.Alert{
						Labels:   model.LabelSet{"bar": model.LabelValue(strconv.Itoa(j))},
						StartsAt: now,
						EndsAt:   now.Add(expire),
					},
					UpdatedAt: now,
				})
				if err != nil && !errors.Is(err, errTooManyAlerts) {
					close(failc)
					return
				}
				j++
			}
		})
	}
	wg.Wait()
	select {
	case <-failc:
		t.Fatalf("unexpected error happened")
	default:
	}

	time.Sleep(expire)
	require.Eventually(t, func() bool {
		// When the alert will eventually expire and is considered resolved - it won't be in the store.
		return len(a.alerts.List()) == 0
	}, 2*expire, expire)
	require.Equal(t, int32(0), callback.alerts.Load())
	require.Equal(t, int32(0), callback.gcCount.Load())
}

func TestSubscriberChannelMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	alerts, err := NewAlerts(context.Background(), 30*time.Minute, 0, noopCallback{}, promslog.NewNopLogger(), eventrecorder.NopRecorder(), reg, nil)
	require.NoError(t, err)

	subscriberName := "test_subscriber"

	// Subscribe to alerts
	iterator := alerts.Subscribe(subscriberName)
	defer iterator.Close()

	// Consume alerts in the background
	go func() {
		for range iterator.Next() {
			// Just drain the channel
		}
	}()

	// Helper function to get counter value
	getCounterValue := func(name, labelName, labelValue string) float64 {
		metrics, err := reg.Gather()
		require.NoError(t, err)
		for _, mf := range metrics {
			if mf.GetName() == name {
				for _, m := range mf.GetMetric() {
					for _, label := range m.GetLabel() {
						if label.GetName() == labelName && label.GetValue() == labelValue {
							return m.GetCounter().GetValue()
						}
					}
				}
			}
		}
		return 0
	}

	// Initially, the counter should be 0
	writeCount := getCounterValue("alertmanager_alerts_subscriber_channel_writes_total", "subscriber", subscriberName)
	require.Equal(t, 0.0, writeCount, "subscriberChannelWrites should start at 0")

	// Put some alerts
	now := time.Now()
	alertsToSend := []*types.Alert{
		{
			Alert: model.Alert{
				Labels:       model.LabelSet{"test": "1"},
				Annotations:  model.LabelSet{"foo": "bar"},
				StartsAt:     now,
				EndsAt:       now.Add(1 * time.Hour),
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now,
			Timeout:   false,
		},
		{
			Alert: model.Alert{
				Labels:       model.LabelSet{"test": "2"},
				Annotations:  model.LabelSet{"foo": "bar"},
				StartsAt:     now,
				EndsAt:       now.Add(1 * time.Hour),
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now,
			Timeout:   false,
		},
		{
			Alert: model.Alert{
				Labels:       model.LabelSet{"test": "3"},
				Annotations:  model.LabelSet{"foo": "bar"},
				StartsAt:     now,
				EndsAt:       now.Add(1 * time.Hour),
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now,
			Timeout:   false,
		},
	}

	err = alerts.Put(context.Background(), alertsToSend...)
	require.NoError(t, err)

	// Verify the counter incremented for each successful write
	require.Eventually(t, func() bool {
		writeCount := getCounterValue("alertmanager_alerts_subscriber_channel_writes_total", "subscriber", subscriberName)
		return writeCount == float64(len(alertsToSend))
	}, 1*time.Second, 10*time.Millisecond, "subscriberChannelWrites should equal the number of alerts sent")
}
