// Copyright 2016 Prometheus Team
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
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
)

var (
	t0 = time.Now()
	t1 = t0.Add(100 * time.Millisecond)

	alert1 = &types.Alert{
		Alert: model.Alert{
			Labels:       model.LabelSet{"bar": "foo"},
			Annotations:  model.LabelSet{"foo": "bar"},
			StartsAt:     t0,
			EndsAt:       t1,
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: t0,
		Timeout:   false,
	}

	alert2 = &types.Alert{
		Alert: model.Alert{
			Labels:       model.LabelSet{"bar": "foo2"},
			Annotations:  model.LabelSet{"foo": "bar2"},
			StartsAt:     t0,
			EndsAt:       t1,
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: t0,
		Timeout:   false,
	}

	alert3 = &types.Alert{
		Alert: model.Alert{
			Labels:       model.LabelSet{"bar": "foo3"},
			Annotations:  model.LabelSet{"foo": "bar3"},
			StartsAt:     t0,
			EndsAt:       t1,
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: t0,
		Timeout:   false,
	}
)

func init() {
	pretty.CompareConfig.IncludeUnexported = true
}

// TestAlertsSubscribePutStarvation tests starvation of `iterator.Close` and
// `alerts.Put`. Both `Subscribe` and `Put` use the Alerts.mtx lock. `Subscribe`
// needs it to subscribe and more importantly unsubscribe `Alerts.listeners`. `Put`
// uses the lock to add additional alerts and iterate the `Alerts.listeners` map.
// If the channel of a listener is at its limit, `alerts.Lock` is blocked, whereby
// a listener can not unsubscribe as the lock is hold by `alerts.Lock`.
func TestAlertsSubscribePutStarvation(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 30*time.Minute, noopCallback{}, promslog.NewNopLogger(), prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	iterator := alerts.Subscribe("test")

	alertsToInsert := []*types.Alert{}
	// Exhaust alert channel
	for i := 0; i < alertChannelLength+1; i++ {
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

	putIsDone := make(chan struct{})
	putsErr := make(chan error, 1)
	go func() {
		if err := alerts.Put(alertsToInsert...); err != nil {
			putsErr <- err
			return
		}

		putIsDone <- struct{}{}
	}()

	// Increase probability that `iterator.Close` is called after `alerts.Put`.
	time.Sleep(100 * time.Millisecond)
	iterator.Close()

	select {
	case <-putsErr:
		t.Fatal(err)
	case <-putIsDone:
		// continue
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected `alerts.Put` and `iterator.Close` not to starve each other")
	}
}

func TestDeadLock(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(5 * time.Second)

	marker := types.NewMarker(prometheus.NewRegistry())
	// Run gc every 5 milliseconds to increase the possibility of a deadlock with Subscribe()
	alerts, err := NewAlerts(context.Background(), marker, 5*time.Millisecond, noopCallback{}, promslog.NewNopLogger(), prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	alertsToInsert := []*types.Alert{}
	for i := 0; i < 200+1; i++ {
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

	if err := alerts.Put(alertsToInsert...); err != nil {
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
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 30*time.Minute, noopCallback{}, promslog.NewNopLogger(), prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	insert := []*types.Alert{alert1, alert2, alert3}

	if err := alerts.Put(insert...); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	for i, a := range insert {
		res, err := alerts.Get(a.Fingerprint())
		if err != nil {
			t.Fatalf("retrieval error: %s", err)
		}
		if !alertsEqual(res, a) {
			t.Errorf("Unexpected alert: %d", i)
			t.Fatal(pretty.Compare(res, a))
		}
	}
}

func TestAlertsSubscribe(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	alerts, err := NewAlerts(ctx, marker, 30*time.Minute, noopCallback{}, promslog.NewNopLogger(), prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	// Add alert1 to validate if pending alerts will be sent.
	if err := alerts.Put(alert1); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	expectedAlerts := map[model.Fingerprint]*types.Alert{
		alert1.Fingerprint(): alert1,
		alert2.Fingerprint(): alert2,
		alert3.Fingerprint(): alert3,
	}

	// Start many consumers and make sure that each receives all the subsequent alerts.
	var (
		nb     = 100
		fatalc = make(chan string, nb)
		wg     sync.WaitGroup
	)
	wg.Add(nb)
	for i := 0; i < nb; i++ {
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
					expected := expectedAlerts[got.Fingerprint()]
					if !alertsEqual(got, expected) {
						fatalc <- fmt.Sprintf("Unexpected alert (iterator %d)\n%s", i, pretty.Compare(got, expected))
						return
					}
					received[got.Fingerprint()] = struct{}{}
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
	if err := alerts.Put(alert2); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}
	if err := alerts.Put(alert3); err != nil {
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
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 30*time.Minute, noopCallback{}, promslog.NewNopLogger(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := alerts.Put(alert1, alert2); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	expectedAlerts := map[model.Fingerprint]*types.Alert{
		alert1.Fingerprint(): alert1,
		alert2.Fingerprint(): alert2,
	}
	iterator := alerts.GetPending()
	for actual := range iterator.Next() {
		expected := expectedAlerts[actual.Fingerprint()]
		if !alertsEqual(actual, expected) {
			t.Errorf("Unexpected alert")
			t.Fatal(pretty.Compare(actual, expected))
		}
	}

	if err := alerts.Put(alert3); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	expectedAlerts = map[model.Fingerprint]*types.Alert{
		alert1.Fingerprint(): alert1,
		alert2.Fingerprint(): alert2,
		alert3.Fingerprint(): alert3,
	}
	iterator = alerts.GetPending()
	for actual := range iterator.Next() {
		expected := expectedAlerts[actual.Fingerprint()]
		if !alertsEqual(actual, expected) {
			t.Errorf("Unexpected alert")
			t.Fatal(pretty.Compare(actual, expected))
		}
	}
}

func TestAlertsGC(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 200*time.Millisecond, noopCallback{}, promslog.NewNopLogger(), nil)
	if err != nil {
		t.Fatal(err)
	}

	insert := []*types.Alert{alert1, alert2, alert3}

	if err := alerts.Put(insert...); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	for _, a := range insert {
		marker.SetActiveOrSilenced(a.Fingerprint(), 0, nil, nil)
		marker.SetInhibited(a.Fingerprint())
		if !marker.Active(a.Fingerprint()) {
			t.Errorf("error setting status: %v", a)
		}
	}

	time.Sleep(300 * time.Millisecond)

	for i, a := range insert {
		_, err := alerts.Get(a.Fingerprint())
		require.Error(t, err)
		require.Equal(t, store.ErrNotFound, err, "alert %d didn't get GC'd: %v", i, err)

		s := marker.Status(a.Fingerprint())
		if s.State != types.AlertStateUnprocessed {
			t.Errorf("marker %d didn't get GC'd: %v", i, s)
		}
	}
}

func TestAlertsStoreCallback(t *testing.T) {
	cb := &limitCountCallback{limit: 3}

	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 200*time.Millisecond, cb, promslog.NewNopLogger(), nil)
	if err != nil {
		t.Fatal(err)
	}

	err = alerts.Put(alert1, alert2, alert3)
	if err != nil {
		t.Fatal(err)
	}
	if num := cb.alerts.Load(); num != 3 {
		t.Fatalf("unexpected number of alerts in the store, expected %v, got %v", 3, num)
	}

	alert1Mod := *alert1
	alert1Mod.Annotations = model.LabelSet{"foo": "bar", "new": "test"} // Update annotations for alert1

	alert4 := &types.Alert{
		Alert: model.Alert{
			Labels:       model.LabelSet{"bar4": "foo4"},
			Annotations:  model.LabelSet{"foo4": "bar4"},
			StartsAt:     t0,
			EndsAt:       t1,
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: t0,
		Timeout:   false,
	}

	err = alerts.Put(&alert1Mod, alert4)
	// Verify that we failed to put new alert into store (not reported via error, only checked using Load)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	if num := cb.alerts.Load(); num != 3 {
		t.Fatalf("unexpected number of alerts in the store, expected %v, got %v", 3, num)
	}

	// But we still managed to update alert1, since callback doesn't report error when updating existing alert.
	a, err := alerts.Get(alert1.Fingerprint())
	if err != nil {
		t.Fatal(err)
	}
	if !alertsEqual(a, &alert1Mod) {
		t.Errorf("Unexpected alert")
		t.Fatal(pretty.Compare(a, &alert1Mod))
	}

	// Now wait until existing alerts are GC-ed, and make sure that callback was called.
	time.Sleep(300 * time.Millisecond)

	if num := cb.alerts.Load(); num != 0 {
		t.Fatalf("unexpected number of alerts in the store, expected %v, got %v", 0, num)
	}

	err = alerts.Put(alert4)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAlerts_Count(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 200*time.Millisecond, nil, promslog.NewNopLogger(), nil)
	require.NoError(t, err)

	states := []types.AlertState{types.AlertStateActive, types.AlertStateSuppressed, types.AlertStateUnprocessed}

	countByState := func(st types.AlertState) int {
		return alerts.count(st)
	}
	countTotal := func() int {
		var count int
		for _, st := range states {
			count += countByState(st)
		}
		return count
	}

	// First, there shouldn't be any alerts.
	require.Equal(t, 0, countTotal())

	// When you insert a new alert that will eventually be active, it should be unprocessed first.
	now := time.Now()
	a1 := &types.Alert{
		Alert: model.Alert{
			Labels:       model.LabelSet{"bar": "foo"},
			Annotations:  model.LabelSet{"foo": "bar"},
			StartsAt:     now,
			EndsAt:       now.Add(400 * time.Millisecond),
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: now,
		Timeout:   false,
	}

	alerts.Put(a1)
	require.Equal(t, 1, countByState(types.AlertStateUnprocessed))
	require.Equal(t, 1, countTotal())
	require.Eventually(t, func() bool {
		// When the alert will eventually expire and is considered resolved - it won't count.
		return countTotal() == 0
	}, 600*time.Millisecond, 100*time.Millisecond)

	now = time.Now()
	a2 := &types.Alert{
		Alert: model.Alert{
			Labels:       model.LabelSet{"bar": "foo"},
			Annotations:  model.LabelSet{"foo": "bar"},
			StartsAt:     now,
			EndsAt:       now.Add(400 * time.Millisecond),
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: now,
		Timeout:   false,
	}

	// When insert an alert, and then silence it. It shows up with the correct filter.
	alerts.Put(a2)
	marker.SetActiveOrSilenced(a2.Fingerprint(), 1, []string{"1"}, nil)
	require.Equal(t, 1, countByState(types.AlertStateSuppressed))
	require.Equal(t, 1, countTotal())

	require.Eventually(t, func() bool {
		// When the alert will eventually expire and is considered resolved - it won't count.
		return countTotal() == 0
	}, 600*time.Millisecond, 100*time.Millisecond)
}

func alertsEqual(a1, a2 *types.Alert) bool {
	if a1 == nil || a2 == nil {
		return false
	}
	if !reflect.DeepEqual(a1.Labels, a2.Labels) {
		return false
	}
	if !reflect.DeepEqual(a1.Annotations, a2.Annotations) {
		return false
	}
	if a1.GeneratorURL != a2.GeneratorURL {
		return false
	}
	if !a1.StartsAt.Equal(a2.StartsAt) {
		return false
	}
	if !a1.EndsAt.Equal(a2.EndsAt) {
		return false
	}
	if !a1.UpdatedAt.Equal(a2.UpdatedAt) {
		return false
	}
	return a1.Timeout == a2.Timeout
}

type limitCountCallback struct {
	alerts atomic.Int32
	limit  int
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
		l.alerts.Inc()
	}
}

func (l *limitCountCallback) PostDelete(_ *types.Alert) {
	l.alerts.Dec()
}

func TestAlertsConcurrently(t *testing.T) {
	callback := &limitCountCallback{limit: 100}
	a, err := NewAlerts(context.Background(), types.NewMarker(prometheus.NewRegistry()), time.Millisecond, callback, promslog.NewNopLogger(), nil)
	require.NoError(t, err)

	stopc := make(chan struct{})
	failc := make(chan struct{})
	go func() {
		time.Sleep(2 * time.Second)
		close(stopc)
	}()
	expire := 10 * time.Millisecond
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

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
				err := a.Put(&types.Alert{
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
		}()
	}
	wg.Wait()
	select {
	case <-failc:
		t.Fatalf("unexpected error happened")
	default:
	}

	time.Sleep(expire)
	require.Eventually(t, func() bool {
		// When the alert will eventually expire and is considered resolved - it won't count.
		return a.count(types.AlertStateActive) == 0
	}, 2*expire, expire)
	require.Equal(t, int32(0), callback.alerts.Load())
}

func TestSubscriberChannelMetrics(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())
	reg := prometheus.NewRegistry()
	alerts, err := NewAlerts(context.Background(), marker, 30*time.Minute, noopCallback{}, promslog.NewNopLogger(), reg)
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

	err = alerts.Put(alertsToSend...)
	require.NoError(t, err)

	// Verify the counter incremented for each successful write
	require.Eventually(t, func() bool {
		writeCount := getCounterValue("alertmanager_alerts_subscriber_channel_writes_total", "subscriber", subscriberName)
		return writeCount == float64(len(alertsToSend))
	}, 1*time.Second, 10*time.Millisecond, "subscriberChannelWrites should equal the number of alerts sent")
}

// TestAlertMergeResolution tests the specific case of resolving alerts via API
// where endsAt is set without normal time overlap.
func TestAlertMergeResolution(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 30*time.Minute, noopCallback{}, promslog.NewNopLogger(), prometheus.NewRegistry())
	require.NoError(t, err)

	now := time.Now()
	baseLabels := model.LabelSet{"alertname": "test", "instance": "localhost:9090"}

	// Test case 1: Resolving an active alert (old alert has no EndsAt)
	t.Run("resolve_active_alert", func(t *testing.T) {
		activeAlert := &types.Alert{
			Alert: model.Alert{
				Labels:       baseLabels,
				Annotations:  model.LabelSet{"summary": "Active alert"},
				StartsAt:     now.Add(-1 * time.Hour),
				EndsAt:       time.Time{}, // Zero time = active
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now.Add(-30 * time.Minute),
			Timeout:   false,
		}

		resolveAlert := &types.Alert{
			Alert: model.Alert{
				Labels:       baseLabels,
				Annotations:  model.LabelSet{"summary": "Resolved alert"},
				StartsAt:     now.Add(-1 * time.Hour), // Same StartsAt
				EndsAt:       now.Add(-10 * time.Minute), // EndsAt in the past
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now,
			Timeout:   false,
		}

		// Insert active alert first
		err := alerts.Put(activeAlert)
		require.NoError(t, err)

		// Verify it's stored
		stored, err := alerts.Get(activeAlert.Fingerprint())
		require.NoError(t, err)
		require.True(t, stored.EndsAt.IsZero(), "Active alert should have zero EndsAt")

		// Now resolve it
		err = alerts.Put(resolveAlert)
		require.NoError(t, err)

		// Verify it was merged and resolved
		updated, err := alerts.Get(activeAlert.Fingerprint())
		require.NoError(t, err)
		require.False(t, updated.EndsAt.IsZero(), "Alert should now have EndsAt set")
		require.True(t, updated.EndsAt.Equal(resolveAlert.EndsAt), "EndsAt should match resolve alert")
	})

	// Test case 2: Re-resolving an already resolved alert to an earlier time
	t.Run("re_resolve_earlier", func(t *testing.T) {
		resolvedAlert := &types.Alert{
			Alert: model.Alert{
				Labels:       model.LabelSet{"alertname": "re_resolve_test"},
				Annotations:  model.LabelSet{"summary": "Originally resolved"},
				StartsAt:     now.Add(-2 * time.Hour),
				EndsAt:       now.Add(-30 * time.Minute), // Already resolved
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now.Add(-1 * time.Hour),
			Timeout:   false,
		}

		reResolveAlert := &types.Alert{
			Alert: model.Alert{
				Labels:       model.LabelSet{"alertname": "re_resolve_test"},
				Annotations:  model.LabelSet{"summary": "Re-resolved earlier"},
				StartsAt:     now.Add(-2 * time.Hour), // Same StartsAt
				EndsAt:       now.Add(-1 * time.Hour), // Earlier resolution
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now,
			Timeout:   false,
		}

		// Insert resolved alert first
		err := alerts.Put(resolvedAlert)
		require.NoError(t, err)

		// Verify it's stored with original EndsAt
		stored, err := alerts.Get(resolvedAlert.Fingerprint())
		require.NoError(t, err)
		require.True(t, stored.EndsAt.Equal(resolvedAlert.EndsAt), "Should have original EndsAt")

		// Now re-resolve it earlier
		err = alerts.Put(reResolveAlert)
		require.NoError(t, err)

		// Verify it was updated to the earlier resolution time
		updated, err := alerts.Get(resolvedAlert.Fingerprint())
		require.NoError(t, err)
		require.True(t, updated.EndsAt.Equal(reResolveAlert.EndsAt), "Should have earlier EndsAt")
	})

	// Test case 3: Reject merge when StartsAt doesn't match (different alert instance)
	t.Run("reject_different_starts_at", func(t *testing.T) {
		originalAlert := &types.Alert{
			Alert: model.Alert{
				Labels:       model.LabelSet{"alertname": "starts_at_test"},
				Annotations:  model.LabelSet{"summary": "Original alert"},
				StartsAt:     now.Add(-1 * time.Hour),
				EndsAt:       time.Time{}, // Active
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now.Add(-30 * time.Minute),
			Timeout:   false,
		}

		differentStartsAlert := &types.Alert{
			Alert: model.Alert{
				Labels:       model.LabelSet{"alertname": "starts_at_test"}, // Same labels
				Annotations:  model.LabelSet{"summary": "Different start time"},
				StartsAt:     now.Add(-2 * time.Hour), // Different StartsAt!
				EndsAt:       now.Add(-10 * time.Minute), // Trying to resolve
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now,
			Timeout:   false,
		}

		// Insert original alert
		err := alerts.Put(originalAlert)
		require.NoError(t, err)

		// Try to "resolve" with different StartsAt
		err = alerts.Put(differentStartsAlert)
		require.NoError(t, err)

		// Verify it was NOT merged - should be stored as separate alert
		allAlerts := alerts.GetAll()
		require.Len(t, allAlerts, 2, "Should have 2 separate alerts")

		// Original should still be active
		stored, err := alerts.Get(originalAlert.Fingerprint())
		require.NoError(t, err)
		require.True(t, stored.EndsAt.IsZero(), "Original alert should still be active")

		// New alert should exist separately
		stored2, err := alerts.Get(differentStartsAlert.Fingerprint())
		require.NoError(t, err)
		require.True(t, stored2.EndsAt.Equal(differentStartsAlert.EndsAt), "New alert should have its own EndsAt")
	})

	// Test case 4: Reject merge when trying to extend active alert (security check)
	t.Run("reject_extend_active_alert", func(t *testing.T) {
		activeAlert := &types.Alert{
			Alert: model.Alert{
				Labels:       model.LabelSet{"alertname": "extend_test"},
				Annotations:  model.LabelSet{"summary": "Active alert"},
				StartsAt:     now.Add(-1 * time.Hour),
				EndsAt:       time.Time{}, // Active
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now.Add(-30 * time.Minute),
			Timeout:   false,
		}

		extendAlert := &types.Alert{
			Alert: model.Alert{
				Labels:       model.LabelSet{"alertname": "extend_test"}, // Same labels
				Annotations:  model.LabelSet{"summary": "Trying to extend"},
				StartsAt:     now.Add(-1 * time.Hour), // Same StartsAt
				EndsAt:       now.Add(1 * time.Hour),   // Future time - trying to extend!
				GeneratorURL: "http://example.com/prometheus",
			},
			UpdatedAt: now,
			Timeout:   false,
		}

		// Insert active alert
		err := alerts.Put(activeAlert)
		require.NoError(t, err)

		// Try to extend it (this should NOT merge)
		err = alerts.Put(extendAlert)
		require.NoError(t, err)

		// Verify it was NOT merged - should be stored as separate alert
		allAlerts := alerts.GetAll()
		require.Len(t, allAlerts, 2, "Should have 2 separate alerts")

		// Original should still be active
		stored, err := alerts.Get(activeAlert.Fingerprint())
		require.NoError(t, err)
		require.True(t, stored.EndsAt.IsZero(), "Original alert should still be active")
	})
}
