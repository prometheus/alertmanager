// Copyright 2016 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/lic:wenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mem

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"sync"

	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
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

func TestAlertsPut(t *testing.T) {
	marker := types.NewMarker()
	alerts, err := NewAlerts(marker, 30*time.Minute)
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
	marker := types.NewMarker()
	alerts, err := NewAlerts(marker, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// add alert1 to validate if pending alerts will be send
	if err := alerts.Put(alert1); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	fatalc := make(chan string, 2)

	iterator1 := alerts.Subscribe()
	iterator2 := alerts.Subscribe()

	go func() {
		defer wg.Done()
		expectedAlerts := map[model.Fingerprint]*types.Alert{
			alert1.Fingerprint(): alert1,
			alert2.Fingerprint(): alert2,
			alert3.Fingerprint(): alert3,
		}

		for i := 0; i < 3; i++ {
			actual := <-iterator1.Next()
			expected := expectedAlerts[actual.Fingerprint()]
			if !alertsEqual(actual, expected) {
				fatalc <- fmt.Sprintf("Unexpected alert (iterator1)\n%s", pretty.Compare(actual, expected))
				return
			}

			delete(expectedAlerts, actual.Fingerprint())
		}

		if len(expectedAlerts) != 0 {
			fatalc <- fmt.Sprintf("Unexpected number of alerts (iterator1): %d", len(expectedAlerts))
		}
	}()

	go func() {
		defer wg.Done()
		expectedAlerts := map[model.Fingerprint]*types.Alert{
			alert1.Fingerprint(): alert1,
			alert2.Fingerprint(): alert2,
			alert3.Fingerprint(): alert3,
		}

		for i := 0; i < 3; i++ {
			actual := <-iterator2.Next()
			expected := expectedAlerts[actual.Fingerprint()]
			if !alertsEqual(actual, expected) {
				t.Errorf("Unexpected alert")
				fatalc <- fmt.Sprintf("Unexpected alert (iterator2)\n%s", pretty.Compare(actual, expected))
			}

			delete(expectedAlerts, actual.Fingerprint())
		}

		if len(expectedAlerts) != 0 {
			fatalc <- fmt.Sprintf("Unexpected number of alerts (iterator2): %d", len(expectedAlerts))
		}
	}()

	go func() {
		wg.Wait()
		close(fatalc)
	}()

	if err := alerts.Put(alert2); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	if err := alerts.Put(alert3); err != nil {
		t.Fatalf("Insert failed: %s", err)
	}

	fatal, ok := <-fatalc
	if ok {
		t.Fatalf(fatal)
	}

	iterator1.Close()
	iterator2.Close()
}

func TestAlertsGetPending(t *testing.T) {
	marker := types.NewMarker()
	alerts, err := NewAlerts(marker, 30*time.Minute)
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
	marker := types.NewMarker()
	alerts, err := NewAlerts(marker, 200*time.Millisecond)
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
		if err != provider.ErrNotFound {
			t.Errorf("alert %d didn't get GC'd", i)
		}

		s := marker.Status(a.Fingerprint())
		if s.State != types.AlertStateUnprocessed {
			t.Errorf("marker %d didn't get GC'd: %v", i, s)
		}
	}
}

func alertsEqual(a1, a2 *types.Alert) bool {
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
