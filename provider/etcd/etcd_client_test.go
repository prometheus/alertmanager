// Copyright 2019 Prometheus Team
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

package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"go.etcd.io/etcd/integration"

	"github.com/go-kit/kit/log"
	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
)

var (
	verbose = true

	fakeAlertCounter = 0

	etcdPrefix      = "am/test/alerts-"
	alertGcInterval = 200 * time.Millisecond

	etcdLogger log.Logger
)

func init() {
	if verbose {
		w := log.NewSyncWriter(os.Stderr)
		etcdLogger = log.NewJSONLogger(w)
	} else {
		etcdLogger = log.NewNopLogger()
	}
}

func TestEtcdWriteReadDeleteAlert(t *testing.T) {
	clus := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	defer clus.Terminate(t)
	etcdEndpoints := []string{clus.Members[0].GRPCAddr()}

	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, alertGcInterval, etcdLogger,
		etcdEndpoints, etcdPrefix)
	if err != nil {
		t.Fatal(err)
	}

	// write and read back
	a1 := fakeAlert()
	if err := alerts.EtcdClient.Put(a1); err != nil {
		t.Errorf("etcdPut failed: %s", err)
	}
	a2, err := alerts.EtcdClient.Get(a1.Fingerprint())
	if err != nil {
		t.Errorf("etcdGet failed: %s", err)
	}
	if !alertsEqual(a1, a2) {
		t.Errorf("Unexpected alert: %s", pretty.Compare(a1, a2))
	}

	// delete and read back
	err = alerts.EtcdClient.Del(a1.Fingerprint())
	if err != nil {
		t.Errorf("etcdDel failed: %s", err)
	}
	_, err = alerts.EtcdClient.Get(a1.Fingerprint())
	if err == nil {
		t.Errorf("etcdGet SHOULD HAVE failed")
	}
}

func TestEtcdMarshalUnmarshalAlert(t *testing.T) {
	var str1, str2 string
	var err error
	var a1, a2 *types.Alert

	a1 = fakeAlert()
	if str1, err = MarshalAlert(a1); err != nil {
		t.Errorf("marshal alert failed: %s", err)
	}
	if a2, err = UnmarshalAlert(str1); err != nil {
		t.Errorf("unmarshal alert failed: %s", err)
	}
	if str2, err = MarshalAlert(a2); err != nil {
		t.Errorf("re-marshal alert failed: %s", err)
	}
	if str1 != str2 {
		t.Error("alert string comparison failed")
	}
	if !alertsEqual(a1, a2) {
		t.Errorf("Unexpected alert: %s", pretty.Compare(a1, a2))
	}
}

func TestEtcdRunWatch(t *testing.T) {
	clus := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	defer clus.Terminate(t)
	etcdEndpoints := []string{clus.Members[0].GRPCAddr()}

	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, alertGcInterval, etcdLogger,
		etcdEndpoints, etcdPrefix)
	if err != nil {
		t.Fatal(err)
	}

	alerts.EtcdClient.RunWatch(context.Background())
	iterator := alerts.Subscribe()
	time.Sleep(100 * time.Millisecond) // wait for subscribe

	// send all of the alerts
	alertsToSend := []*types.Alert{fakeAlert(), fakeAlert(), fakeAlert()}
	for _, a := range alertsToSend {
		if err := alerts.EtcdClient.Put(a); err != nil {
			t.Errorf("etcdPut failed: %s", err)
		}
	}

	// read the alerts back in order
	index := 0
	for alert := range iterator.Next() {
		if !alertsEqual(alert, alertsToSend[index]) {
			t.Errorf("Unexpected alert: %s", pretty.Compare(alert, alertsToSend[index]))
		}
		index += 1
		if index == len(alertsToSend) {
			break
		}
	}
}

func TestEtcdRunLoadAllAlerts(t *testing.T) {
	clus := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	defer clus.Terminate(t)
	etcdEndpoints := []string{clus.Members[0].GRPCAddr()}

	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, alertGcInterval, etcdLogger,
		etcdEndpoints, etcdPrefix)
	if err != nil {
		t.Fatal(err)
	}

	// put some alerts into etcd first
	alertsToSend := []*types.Alert{fakeAlert(), fakeAlert(), fakeAlert()}
	for _, a := range alertsToSend {
		if err := alerts.EtcdClient.Put(a); err != nil {
			t.Errorf("etcdPut failed: %s", err)
		}
	}

	iterator := alerts.Subscribe()
	time.Sleep(100 * time.Millisecond) // wait for subscribe

	// instruct AM to read back all alerts from etcd
	alerts.EtcdClient.RunLoadAllAlerts(context.Background())

	// read the alerts back.  ordering is not guaranteed
	expectedAlerts := map[model.Fingerprint]*types.Alert{}
	for _, a := range alertsToSend {
		expectedAlerts[a.Fingerprint()] = a
	}

	for actual := range iterator.Next() {
		expected := expectedAlerts[actual.Fingerprint()]
		if !alertsEqual(actual, expected) {
			t.Errorf("Unexpected alert: %s", pretty.Compare(actual, expected))
		}
		delete(expectedAlerts, actual.Fingerprint())
		if len(expectedAlerts) == 0 {
			break
		}
	}
}

func TestEtcdGC(t *testing.T) {
	clus := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	defer clus.Terminate(t)
	etcdEndpoints := []string{clus.Members[0].GRPCAddr()}

	marker := types.NewMarker(prometheus.NewRegistry())
	alerts, err := NewAlerts(context.Background(), marker, alertGcInterval, etcdLogger,
		etcdEndpoints, etcdPrefix)
	if err != nil {
		t.Fatal(err)
	}

	testDuration := 3000 * time.Millisecond
	startsAt := time.Now()
	endsAt := t0.Add(testDuration)

	// write to alert store
	a1 := fakeAlertWithTime(startsAt, endsAt)
	if err := alerts.Put(a1); err != nil {
		t.Errorf("alertPut failed: %s", err)
	}

	time.Sleep(testDuration / 2)

	// ensure write-through to etcd
	a2, err := alerts.EtcdClient.Get(a1.Fingerprint())
	if err != nil {
		t.Errorf("etcdGet failed: %s", err)
	}
	if !alertsEqual(a1, a2) {
		t.Errorf("Unexpected alert: %s", pretty.Compare(a1, a2))
	}

	time.Sleep(testDuration/2 + alertGcInterval*2)

	// ensure expiration in etcd
	_, err = alerts.EtcdClient.Get(a1.Fingerprint())
	if err == nil {
		t.Errorf("etcdGet SHOULD HAVE failed")
	}
}

func fakeAlert() *types.Alert {
	startsAt := time.Now()
	endsAt := t0.Add(10 * time.Second)
	return fakeAlertWithTime(startsAt, endsAt)
}

func fakeAlertWithTime(startsAt time.Time, endsAt time.Time) *types.Alert {
	fakeAlertCounter += 1

	labelSetJSON := fmt.Sprintf(`{ "labelSet": {
		"foo%d": "bar%d",
                "time": "%s"
	}}`, fakeAlertCounter, fakeAlertCounter, time.Now().String())

	type testConfig struct {
		LabelSet model.LabelSet `yaml:"labelSet,omitempty"`
	}

	var c testConfig
	err := json.Unmarshal([]byte(labelSetJSON), &c)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	a := &types.Alert{
		Alert: model.Alert{
			Labels:       c.LabelSet,
			Annotations:  model.LabelSet{"foo": "bar"},
			StartsAt:     startsAt,
			EndsAt:       endsAt,
			GeneratorURL: "http://example.com/prometheus",
		},
		UpdatedAt: startsAt,
		Timeout:   false,
	}
	return a
}
