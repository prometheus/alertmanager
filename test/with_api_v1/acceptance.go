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

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/client"
)

// AcceptanceTest provides declarative definition of given inputs and expected
// output of an Alertmanager setup.
type AcceptanceTest struct {
	*testing.T

	opts *AcceptanceOpts

	ams        []*Alertmanager
	collectors []*Collector

	actions map[float64][]func()
}

// AcceptanceOpts defines configuration paramters for an acceptance test.
type AcceptanceOpts struct {
	RoutePrefix string
	Tolerance   time.Duration
	baseTime    time.Time
}

func (opts *AcceptanceOpts) alertString(a *model.Alert) string {
	if a.EndsAt.IsZero() {
		return fmt.Sprintf("%s[%v:]", a, opts.relativeTime(a.StartsAt))
	}
	return fmt.Sprintf("%s[%v:%v]", a, opts.relativeTime(a.StartsAt), opts.relativeTime(a.EndsAt))
}

// expandTime returns the absolute time for the relative time
// calculated from the test's base time.
func (opts *AcceptanceOpts) expandTime(rel float64) time.Time {
	return opts.baseTime.Add(time.Duration(rel * float64(time.Second)))
}

// expandTime returns the relative time for the given time
// calculated from the test's base time.
func (opts *AcceptanceOpts) relativeTime(act time.Time) float64 {
	return float64(act.Sub(opts.baseTime)) / float64(time.Second)
}

// NewAcceptanceTest returns a new acceptance test with the base time
// set to the current time.
func NewAcceptanceTest(t *testing.T, opts *AcceptanceOpts) *AcceptanceTest {
	test := &AcceptanceTest{
		T:       t,
		opts:    opts,
		actions: map[float64][]func(){},
	}
	opts.baseTime = time.Now()

	return test
}

// freeAddress returns a new listen address not currently in use.
func freeAddress() string {
	// Let the OS allocate a free address, close it and hope
	// it is still free when starting Alertmanager.
	l, err := net.Listen("tcp4", "localhost:0")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			panic(err)
		}
	}()

	return l.Addr().String()
}

// Do sets the given function to be executed at the given time.
func (t *AcceptanceTest) Do(at float64, f func()) {
	t.actions[at] = append(t.actions[at], f)
}

// Alertmanager returns a new structure that allows starting an instance
// of Alertmanager on a random port.
func (t *AcceptanceTest) Alertmanager(conf string) *Alertmanager {
	am := &Alertmanager{
		t:    t,
		opts: t.opts,
	}

	dir, err := ioutil.TempDir("", "am_test")
	if err != nil {
		t.Fatal(err)
	}
	am.dir = dir

	cf, err := os.Create(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	am.confFile = cf
	am.UpdateConfig(conf)

	am.apiAddr = freeAddress()
	am.clusterAddr = freeAddress()

	t.Logf("AM on %s", am.apiAddr)

	c, err := api.NewClient(api.Config{
		Address: am.getURL(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	am.client = c

	t.ams = append(t.ams, am)

	return am
}

// Collector returns a new collector bound to the test instance.
func (t *AcceptanceTest) Collector(name string) *Collector {
	co := &Collector{
		t:         t.T,
		name:      name,
		opts:      t.opts,
		collected: map[float64][]model.Alerts{},
		expected:  map[Interval][]model.Alerts{},
	}
	t.collectors = append(t.collectors, co)

	return co
}

// Run starts all Alertmanagers and runs queries against them. It then checks
// whether all expected notifications have arrived at the expected receiver.
func (t *AcceptanceTest) Run() {
	errc := make(chan error)

	for _, am := range t.ams {
		am.errc = errc

		am.Start()
		defer func(am *Alertmanager) {
			am.Terminate()
			am.cleanup()
			t.Logf("stdout:\n%v", am.cmd.Stdout)
			t.Logf("stderr:\n%v", am.cmd.Stderr)
		}(am)
	}

	go t.runActions()

	var latest float64
	for _, coll := range t.collectors {
		if l := coll.latest(); l > latest {
			latest = l
		}
	}

	deadline := t.opts.expandTime(latest)

	select {
	case <-time.After(time.Until(deadline)):
		// continue
	case err := <-errc:
		t.Error(err)
	}

	for _, coll := range t.collectors {
		report := coll.check()
		t.Log(report)
	}
}

// runActions performs the stored actions at the defined times.
func (t *AcceptanceTest) runActions() {
	var wg sync.WaitGroup

	for at, fs := range t.actions {
		ts := t.opts.expandTime(at)
		wg.Add(len(fs))

		for _, f := range fs {
			go func(f func()) {
				time.Sleep(time.Until(ts))
				f()
				wg.Done()
			}(f)
		}
	}

	wg.Wait()
}

type buffer struct {
	b   bytes.Buffer
	mtx sync.Mutex
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.b.Write(p)
}

func (b *buffer) String() string {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.b.String()
}

// Alertmanager encapsulates an Alertmanager process and allows
// declaring alerts being pushed to it at fixed points in time.
type Alertmanager struct {
	t    *AcceptanceTest
	opts *AcceptanceOpts

	apiAddr     string
	clusterAddr string
	client      api.Client
	cmd         *exec.Cmd
	confFile    *os.File
	dir         string

	errc chan<- error
}

// Start the alertmanager and wait until it is ready to receive.
func (am *Alertmanager) Start() {
	args := []string{
		"--config.file", am.confFile.Name(),
		"--log.level", "debug",
		"--web.listen-address", am.apiAddr,
		"--storage.path", am.dir,
		"--cluster.listen-address", am.clusterAddr,
		"--cluster.settle-timeout", "0s",
	}
	if am.opts.RoutePrefix != "" {
		args = append(args, "--web.route-prefix", am.opts.RoutePrefix)
	}
	cmd := exec.Command("../../../alertmanager", args...)

	if am.cmd == nil {
		var outb, errb buffer
		cmd.Stdout = &outb
		cmd.Stderr = &errb
	} else {
		cmd.Stdout = am.cmd.Stdout
		cmd.Stderr = am.cmd.Stderr
	}
	am.cmd = cmd

	if err := am.cmd.Start(); err != nil {
		am.t.Fatalf("Starting alertmanager failed: %s", err)
	}

	go func() {
		if err := am.cmd.Wait(); err != nil {
			am.errc <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 10; i++ {
		resp, err := http.Get(am.getURL("/"))
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			am.t.Fatalf("Starting alertmanager failed: expected HTTP status '200', got '%d'", resp.StatusCode)
		}
		_, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			am.t.Fatalf("Starting alertmanager failed: %s", err)
		}
		resp.Body.Close()
		return
	}
	am.t.Fatalf("Starting alertmanager failed: timeout")
}

// Terminate kills the underlying Alertmanager process and remove intermediate
// data.
func (am *Alertmanager) Terminate() {
	if err := syscall.Kill(am.cmd.Process.Pid, syscall.SIGTERM); err != nil {
		am.t.Fatalf("error sending SIGTERM to Alertmanager process: %v", err)
	}
}

// Reload sends the reloading signal to the Alertmanager process.
func (am *Alertmanager) Reload() {
	if err := syscall.Kill(am.cmd.Process.Pid, syscall.SIGHUP); err != nil {
		am.t.Fatalf("error sending SIGHUP to Alertmanager process: %v", err)
	}
}

func (am *Alertmanager) cleanup() {
	if err := os.RemoveAll(am.confFile.Name()); err != nil {
		am.t.Errorf("error removing test config file %q: %v", am.confFile.Name(), err)
	}
}

// Push declares alerts that are to be pushed to the Alertmanager
// server at a relative point in time.
func (am *Alertmanager) Push(at float64, alerts ...*TestAlert) {
	var cas []client.Alert
	for i := range alerts {
		a := alerts[i].nativeAlert(am.opts)
		al := client.Alert{
			Labels:       client.LabelSet{},
			Annotations:  client.LabelSet{},
			StartsAt:     a.StartsAt,
			EndsAt:       a.EndsAt,
			GeneratorURL: a.GeneratorURL,
		}
		for n, v := range a.Labels {
			al.Labels[client.LabelName(n)] = client.LabelValue(v)
		}
		for n, v := range a.Annotations {
			al.Annotations[client.LabelName(n)] = client.LabelValue(v)
		}
		cas = append(cas, al)
	}

	alertAPI := client.NewAlertAPI(am.client)

	am.t.Do(at, func() {
		if err := alertAPI.Push(context.Background(), cas...); err != nil {
			am.t.Errorf("Error pushing %v: %s", cas, err)
		}
	})
}

// SetSilence updates or creates the given Silence.
func (am *Alertmanager) SetSilence(at float64, sil *TestSilence) {
	am.t.Do(at, func() {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(sil.nativeSilence(am.opts)); err != nil {
			am.t.Errorf("Error setting silence %v: %s", sil, err)
			return
		}

		resp, err := http.Post(am.getURL("/api/v1/silences"), "application/json", &buf)
		if err != nil {
			am.t.Errorf("Error setting silence %v: %s", sil, err)
			return
		}
		defer resp.Body.Close()

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}

		var v struct {
			Status string `json:"status"`
			Data   struct {
				SilenceID string `json:"silenceId"`
			} `json:"data"`
		}
		if err := json.Unmarshal(b, &v); err != nil || resp.StatusCode/100 != 2 {
			am.t.Errorf("error setting silence %v: %s", sil, err)
			return
		}
		sil.SetID(v.Data.SilenceID)
	})
}

// DelSilence deletes the silence with the sid at the given time.
func (am *Alertmanager) DelSilence(at float64, sil *TestSilence) {
	am.t.Do(at, func() {
		req, err := http.NewRequest("DELETE", am.getURL(fmt.Sprintf("/api/v1/silence/%s", sil.ID())), nil)
		if err != nil {
			am.t.Errorf("Error deleting silence %v: %s", sil, err)
			return
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode/100 != 2 {
			am.t.Errorf("Error deleting silence %v: %s", sil, err)
			return
		}
	})
}

// UpdateConfig rewrites the configuration file for the Alertmanager. It does not
// initiate config reloading.
func (am *Alertmanager) UpdateConfig(conf string) {
	if _, err := am.confFile.WriteString(conf); err != nil {
		am.t.Fatal(err)
		return
	}
	if err := am.confFile.Sync(); err != nil {
		am.t.Fatal(err)
		return
	}
}

func (am *Alertmanager) getURL(path string) string {
	return fmt.Sprintf("http://%s%s%s", am.apiAddr, am.opts.RoutePrefix, path)
}
