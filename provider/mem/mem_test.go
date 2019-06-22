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
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"sync"

	"github.com/go-kit/kit/log"
	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
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
	alerts, err := NewAlerts(context.Background(), marker, 30*time.Minute, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}

	iterator := alerts.Subscribe()

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

func TestAlertsPut(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 30*time.Minute, log.NewNopLogger())
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
			t.Fatalf(pretty.Compare(res, a))
		}
	}
}

func TestAlertsSubscribe(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	alerts, err := NewAlerts(ctx, marker, 30*time.Minute, log.NewNopLogger())
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

			it := alerts.Subscribe()
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
		t.Fatalf(fatal)
	}
}

func TestAlertsGetPending(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 30*time.Minute, log.NewNopLogger())
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
			t.Fatalf(pretty.Compare(actual, expected))
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
			t.Fatalf(pretty.Compare(actual, expected))
		}
	}
}

func TestAlertsGC(t *testing.T) {
	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, 200*time.Millisecond, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}

	insert := []*types.Alert{alert1, alert2, alert3}

	if err := alerts.Put(insert...); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	for _, a := range insert {
		marker.SetActive(a.Fingerprint())
		if !marker.Active(a.Fingerprint()) {
			t.Errorf("error setting status: %v", a)
		}
	}

	time.Sleep(300 * time.Millisecond)

	for i, a := range insert {
		_, err := alerts.Get(a.Fingerprint())
		require.Error(t, err)
		require.Equal(t, store.ErrNotFound, err, fmt.Sprintf("alert %d didn't get GC'd: %v", i, err))

		s := marker.Status(a.Fingerprint())
		if s.State != types.AlertStateUnprocessed {
			t.Errorf("marker %d didn't get GC'd: %v", i, s)
		}
	}
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
