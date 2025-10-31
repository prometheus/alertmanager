// Copyright 2018 Prometheus Team
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
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	apiclient "github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/api/v2/client/alert"
	"github.com/prometheus/alertmanager/api/v2/client/general"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

// AcceptanceTest provides declarative definition of given inputs and expected
// output of an Alertmanager setup.
type AcceptanceTest struct {
	*testing.T

	opts *AcceptanceOpts

	amc        *AlertmanagerCluster
	collectors []*Collector

	actions map[float64][]func()
}

// AcceptanceOpts defines configuration parameters for an acceptance test.
type AcceptanceOpts struct {
	FeatureFlags []string
	RoutePrefix  string
	Tolerance    time.Duration
	baseTime     time.Time
}

func (opts *AcceptanceOpts) alertString(a *models.GettableAlert) string {
	if a.EndsAt == nil || time.Time(*a.EndsAt).IsZero() {
		return fmt.Sprintf("%v[%v:]", a, opts.relativeTime(time.Time(*a.StartsAt)))
	}
	return fmt.Sprintf("%v[%v:%v]", a, opts.relativeTime(time.Time(*a.StartsAt)), opts.relativeTime(time.Time(*a.EndsAt)))
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

// AlertmanagerCluster returns a new AlertmanagerCluster that allows starting a
// cluster of Alertmanager instances on random ports.
func (t *AcceptanceTest) AlertmanagerCluster(conf string, size int) *AlertmanagerCluster {
	amc := AlertmanagerCluster{}

	for i := 0; i < size; i++ {
		am := &Alertmanager{
			t:    t,
			opts: t.opts,
		}

		dir, err := os.MkdirTemp("", "am_test")
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

		transport := httptransport.New(am.apiAddr, t.opts.RoutePrefix+"/api/v2/", nil)
		am.clientV2 = apiclient.New(transport, strfmt.Default)

		amc.ams = append(amc.ams, am)
	}

	t.amc = &amc

	return &amc
}

// Collector returns a new collector bound to the test instance.
func (t *AcceptanceTest) Collector(name string) *Collector {
	co := &Collector{
		t:         t.T,
		name:      name,
		opts:      t.opts,
		collected: map[float64][]models.GettableAlerts{},
		expected:  map[Interval][]models.GettableAlerts{},
	}
	t.collectors = append(t.collectors, co)

	return co
}

// Run starts all Alertmanagers and runs queries against them. It then checks
// whether all expected notifications have arrived at the expected receiver.
func (t *AcceptanceTest) Run() {
	errc := make(chan error)

	for _, am := range t.amc.ams {
		am.errc = errc
		t.Cleanup(am.Terminate)
		t.Cleanup(am.cleanup)
	}

	err := t.amc.Start()
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}

	// Set the reference time right before running the test actions to avoid
	// test failures due to slow setup of the test environment.
	t.opts.baseTime = time.Now()

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
	clientV2    *apiclient.AlertmanagerAPI
	confFile    *os.File
	dir         string

	cmd  *exec.Cmd
	errc chan<- error
}

// AlertmanagerCluster represents a group of Alertmanager instances
// acting as a cluster.
type AlertmanagerCluster struct {
	ams []*Alertmanager
}

// Start the Alertmanager cluster and wait until it is ready to receive.
func (amc *AlertmanagerCluster) Start() error {
	var peerFlags []string
	for _, am := range amc.ams {
		peerFlags = append(peerFlags, "--cluster.peer="+am.clusterAddr)
	}

	for _, am := range amc.ams {
		err := am.Start(peerFlags)
		if err != nil {
			return fmt.Errorf("failed to start alertmanager cluster: %w", err)
		}
	}

	for _, am := range amc.ams {
		err := am.WaitForCluster(len(amc.ams))
		if err != nil {
			return fmt.Errorf("failed to wait for Alertmanager instance %q to join cluster: %w", am.clusterAddr, err)
		}
	}

	return nil
}

// Members returns the underlying slice of cluster members.
func (amc *AlertmanagerCluster) Members() []*Alertmanager {
	return amc.ams
}

// Start the alertmanager and wait until it is ready to receive.
func (am *Alertmanager) Start(additionalArg []string) error {
	am.t.Helper()
	args := []string{
		"--config.file", am.confFile.Name(),
		"--log.level", "debug",
		"--web.listen-address", am.apiAddr,
		"--storage.path", am.dir,
		"--cluster.listen-address", am.clusterAddr,
		"--cluster.settle-timeout", "0s",
	}
	if len(am.opts.FeatureFlags) > 0 {
		args = append(args, "--enable-feature", strings.Join(am.opts.FeatureFlags, ","))
	}
	if am.opts.RoutePrefix != "" {
		args = append(args, "--web.route-prefix", am.opts.RoutePrefix)
	}
	args = append(args, additionalArg...)

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
		return err
	}

	go func() {
		if err := am.cmd.Wait(); err != nil {
			am.errc <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)
	var lastErr error
	for i := 0; i < 10; i++ {
		_, lastErr = am.clientV2.General.GetStatus(nil)
		if lastErr == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("unable to get a successful response from the Alertmanager: %w", lastErr)
}

// WaitForCluster waits for the Alertmanager instance to join a cluster with the
// given size.
func (am *Alertmanager) WaitForCluster(size int) error {
	params := general.NewGetStatusParams()
	params.WithContext(context.Background())
	var status *general.GetStatusOK

	// Poll for 2s
	for i := 0; i < 20; i++ {
		var err error
		status, err = am.clientV2.General.GetStatus(params)
		if err != nil {
			return err
		}

		if len(status.Payload.Cluster.Peers) == size {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf(
		"expected %v peers, but got %v",
		size,
		len(status.Payload.Cluster.Peers),
	)
}

// Terminate kills the underlying Alertmanager cluster processes and removes intermediate
// data.
func (amc *AlertmanagerCluster) Terminate() {
	for _, am := range amc.ams {
		am.Terminate()
	}
}

// Terminate kills the underlying Alertmanager process and remove intermediate
// data.
func (am *Alertmanager) Terminate() {
	am.t.Helper()
	if am.cmd.Process != nil {
		if err := syscall.Kill(am.cmd.Process.Pid, syscall.SIGTERM); err != nil {
			am.t.Logf("Error sending SIGTERM to Alertmanager process: %v", err)
		}
		am.t.Logf("stdout:\n%v", am.cmd.Stdout)
		am.t.Logf("stderr:\n%v", am.cmd.Stderr)
	}
}

// Reload sends the reloading signal to the Alertmanager instances.
func (amc *AlertmanagerCluster) Reload() {
	for _, am := range amc.ams {
		am.Reload()
	}
}

// Reload sends the reloading signal to the Alertmanager process.
func (am *Alertmanager) Reload() {
	am.t.Helper()
	if am.cmd.Process != nil {
		if err := syscall.Kill(am.cmd.Process.Pid, syscall.SIGHUP); err != nil {
			am.t.Fatalf("Error sending SIGHUP to Alertmanager process: %v", err)
		}
	}
}

func (am *Alertmanager) cleanup() {
	am.t.Helper()
	if err := os.RemoveAll(am.confFile.Name()); err != nil {
		am.t.Errorf("Error removing test config file %q: %v", am.confFile.Name(), err)
	}
}

// Push declares alerts that are to be pushed to the Alertmanager
// servers at a relative point in time.
func (amc *AlertmanagerCluster) Push(at float64, alerts ...*TestAlert) {
	for _, am := range amc.ams {
		am.Push(at, alerts...)
	}
}

// Push declares alerts that are to be pushed to the Alertmanager
// server at a relative point in time.
func (am *Alertmanager) Push(at float64, alerts ...*TestAlert) {
	am.t.Do(at, func() {
		var cas models.PostableAlerts
		for i := range alerts {
			a := alerts[i].nativeAlert(am.opts)
			alert := &models.PostableAlert{
				Alert: models.Alert{
					Labels:       a.Labels,
					GeneratorURL: a.GeneratorURL,
				},
				Annotations: a.Annotations,
			}
			if a.StartsAt != nil {
				alert.StartsAt = *a.StartsAt
			}
			if a.EndsAt != nil {
				alert.EndsAt = *a.EndsAt
			}
			cas = append(cas, alert)
		}

		params := alert.PostAlertsParams{}
		params.WithContext(context.Background()).WithAlerts(cas)

		_, err := am.clientV2.Alert.PostAlerts(&params)
		if err != nil {
			am.t.Errorf("Error pushing %v: %v", cas, err)
		}
	})
}

// SetSilence updates or creates the given Silence.
func (amc *AlertmanagerCluster) SetSilence(at float64, sil *TestSilence) {
	for _, am := range amc.ams {
		am.SetSilence(at, sil)
	}
}

// SetSilence updates or creates the given Silence.
func (am *Alertmanager) SetSilence(at float64, sil *TestSilence) {
	am.t.Do(at, func() {
		resp, err := am.clientV2.Silence.PostSilences(
			silence.NewPostSilencesParams().WithSilence(
				&models.PostableSilence{
					Silence: *sil.nativeSilence(am.opts),
				},
			),
		)
		if err != nil {
			am.t.Errorf("Error setting silence %v: %s", sil, err)
			return
		}
		sil.SetID(resp.Payload.SilenceID)
	})
}

// DelSilence deletes the silence with the sid at the given time.
func (amc *AlertmanagerCluster) DelSilence(at float64, sil *TestSilence) {
	for _, am := range amc.ams {
		am.DelSilence(at, sil)
	}
}

// DelSilence deletes the silence with the sid at the given time.
func (am *Alertmanager) DelSilence(at float64, sil *TestSilence) {
	am.t.Do(at, func() {
		_, err := am.clientV2.Silence.DeleteSilence(
			silence.NewDeleteSilenceParams().WithSilenceID(strfmt.UUID(sil.ID())),
		)
		if err != nil {
			am.t.Errorf("Error deleting silence %v: %s", sil, err)
		}
	})
}

// UpdateConfig rewrites the configuration file for the Alertmanager cluster. It
// does not initiate config reloading.
func (amc *AlertmanagerCluster) UpdateConfig(conf string) {
	for _, am := range amc.ams {
		am.UpdateConfig(conf)
	}
}

// UpdateConfig rewrites the configuration file for the Alertmanager. It does not
// initiate config reloading.
func (am *Alertmanager) UpdateConfig(conf string) {
	if _, err := am.confFile.WriteString(conf); err != nil {
		am.t.Fatal(err)
	}
	if err := am.confFile.Sync(); err != nil {
		am.t.Fatal(err)
	}
}

// Client returns a client to interact with the API v2 endpoint.
func (am *Alertmanager) Client() *apiclient.AlertmanagerAPI {
	return am.clientV2
}
