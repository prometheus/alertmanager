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

package testutils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	apiclient "github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/api/v2/client/general"
	"github.com/prometheus/alertmanager/api/v2/models"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

// AcceptanceOpts defines configuration parameters for an acceptance test.
type AcceptanceOpts struct {
	FeatureFlags []string
	RoutePrefix  string
	Tolerance    time.Duration
	baseTime     time.Time
}

// AlertString formats an alert for display with relative times.
func (opts *AcceptanceOpts) AlertString(a *models.GettableAlert) string {
	if a.EndsAt == nil || time.Time(*a.EndsAt).IsZero() {
		return fmt.Sprintf("%v[%v:]", a, opts.RelativeTime(time.Time(*a.StartsAt)))
	}
	return fmt.Sprintf("%v[%v:%v]", a, opts.RelativeTime(time.Time(*a.StartsAt)), opts.RelativeTime(time.Time(*a.EndsAt)))
}

// ExpandTime returns the absolute time for the relative time
// calculated from the test's base time.
func (opts *AcceptanceOpts) ExpandTime(rel float64) time.Time {
	return opts.baseTime.Add(time.Duration(rel * float64(time.Second)))
}

// RelativeTime returns the relative time for the given time
// calculated from the test's base time.
func (opts *AcceptanceOpts) RelativeTime(act time.Time) float64 {
	return float64(act.Sub(opts.baseTime)) / float64(time.Second)
}

// SetBaseTime sets the base time for relative time calculations.
func (opts *AcceptanceOpts) SetBaseTime(t time.Time) {
	opts.baseTime = t
}

// AcceptanceTest provides declarative definition of given inputs and expected
// output of an Alertmanager setup.
type AcceptanceTest struct {
	*testing.T

	opts *AcceptanceOpts

	amc        *AlertmanagerCluster
	collectors []*Collector

	actions map[float64][]func()
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

// Do sets the given function to be executed at the given time.
func (t *AcceptanceTest) Do(at float64, f func()) {
	t.actions[at] = append(t.actions[at], f)
}

// AlertmanagerCluster returns a new AlertmanagerCluster that allows starting a
// cluster of Alertmanager instances on random ports.
func (t *AcceptanceTest) AlertmanagerCluster(conf string, size int) *AlertmanagerCluster {
	amc := AlertmanagerCluster{}

	for range size {
		am := &Alertmanager{
			T:    t,
			Opts: t.opts,
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

		// apiAddr and clusterAddr will be discovered during Start()
		// clientV2 will be created during Start() with the discovered address

		amc.ams = append(amc.ams, am)
	}

	t.amc = &amc

	return &amc
}

// Collector returns a new collector bound to the test instance.
func (t *AcceptanceTest) Collector(name string) *Collector {
	co := NewCollector(t.T, name, t.opts)
	t.collectors = append(t.collectors, co)

	return co
}

// Run starts all Alertmanagers and runs queries against them. It then checks
// whether all expected notifications have arrived at the expected receiver.
func (t *AcceptanceTest) Run(additionalArgs ...string) {
	errc := make(chan error)

	for _, am := range t.amc.ams {
		am.errc = errc
		t.Cleanup(am.Terminate)
		t.Cleanup(am.cleanup)
	}

	err := t.amc.Start(additionalArgs...)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}

	// Set the reference time right before running the test actions to avoid
	// test failures due to slow setup of the test environment.
	t.opts.SetBaseTime(time.Now())

	go t.runActions()

	var latest float64
	for _, coll := range t.collectors {
		if l := coll.Latest(); l > latest {
			latest = l
		}
	}

	deadline := t.opts.ExpandTime(latest)

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
		ts := t.opts.ExpandTime(at)
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
	T    *AcceptanceTest
	Opts *AcceptanceOpts

	apiAddr     string // the API address of this instance, discovered after start
	clusterAddr string // the cluster address can be the address of any peer

	clientV2 *apiclient.AlertmanagerAPI
	confFile *os.File
	dir      string

	cmd  *exec.Cmd
	errc chan<- error
}

// ClusterAddr returns an address for the cluster.
func (am *Alertmanager) ClusterAddr() string {
	return am.clusterAddr
}

// APIAddr returns the API address for the instance.
func (am *Alertmanager) APIAddr() string {
	return am.apiAddr
}

// AlertmanagerCluster represents a group of Alertmanager instances
// acting as a cluster.
type AlertmanagerCluster struct {
	ams []*Alertmanager
}

// Start the Alertmanager cluster and wait until it is ready to receive.
func (amc *AlertmanagerCluster) Start(additionalArgs ...string) error {
	args := make([]string, 0, len(additionalArgs)+1)
	args = append(args, additionalArgs...)
	clusterAdded := false

	for i, am := range amc.ams {
		am.T.Logf("Starting cluster member %d/%d", i+1, len(amc.ams))

		// Start this instance (it will discover its own ports)
		if err := am.Start(args); err != nil {
			return fmt.Errorf("starting cluster member %d: %w", i, err)
		}

		// From the second instance onwards, append the cluster.peer argument
		// so the subsequent ones join up.
		if !clusterAdded {
			args = append(args, "--cluster.peer="+am.ClusterAddr())
			clusterAdded = true
		}
	}

	// Wait for cluster to converge
	for _, am := range amc.ams {
		if err := am.WaitForCluster(len(amc.ams)); err != nil {
			return fmt.Errorf("failed to wait for Alertmanager instance %q to join cluster: %w", am.APIAddr(), err)
		}
	}

	return nil
}

// Members returns the underlying slice of cluster members.
func (amc *AlertmanagerCluster) Members() []*Alertmanager {
	return amc.ams
}

// discoverWebAddress parses stderr for "Listening on" log message and updates am.apiAddr.
func (am *Alertmanager) discoverWebAddress(timeout time.Duration) error {
	am.T.Helper()
	deadline := time.Now().Add(timeout)
	stderrBuf, ok := am.cmd.Stderr.(*buffer)
	if !ok {
		return fmt.Errorf("stderr is not a buffer")
	}

	// Compile regex once outside the loop
	re := regexp.MustCompile(`address=([^\s]+)`)
	lastPos := 0

	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		stderr := stderrBuf.String()
		// Only process new content since last check
		if len(stderr) <= lastPos {
			continue
		}
		newContent := stderr[lastPos:]
		lastPos = len(stderr)

		// Look for: msg="Listening on" address=127.0.0.1:PORT
		for line := range strings.SplitSeq(newContent, "\n") {
			if !strings.Contains(line, "Listening on") {
				continue
			}
			// Extract address using regex: address=IP:PORT
			matches := re.FindStringSubmatch(line)
			if len(matches) == 2 {
				am.apiAddr = matches[1]
				am.T.Logf("Discovered web address: %s", am.apiAddr)
				return nil
			}
		}
	}
	return fmt.Errorf("timeout waiting for web address in logs")
}

// discoverClusterAddress queries /api/v2/status for cluster address and updates am.clusterAddr.
func (am *Alertmanager) discoverClusterAddress(timeout time.Duration) error {
	am.T.Helper()
	deadline := time.Now().Add(timeout)
	params := general.NewGetStatusParams()
	params.WithContext(context.Background())

	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		status, err := am.clientV2.General.GetStatus(params)
		if err != nil || status.Payload == nil || status.Payload.Cluster == nil {
			continue
		}
		if len(status.Payload.Cluster.Peers) == 0 {
			continue
		}
		peer := status.Payload.Cluster.Peers[0]
		if peer != nil && peer.Address != nil {
			am.clusterAddr = *peer.Address
			am.T.Logf("Discovered cluster address: %s", am.clusterAddr)
			return nil
		}
	}
	return fmt.Errorf("timeout waiting for cluster address from API")
}

// Start the alertmanager and wait until it is ready to receive.
func (am *Alertmanager) Start(additionalArg []string) error {
	am.T.Helper()
	args := []string{
		"--config.file", am.confFile.Name(),
		"--log.level", "debug",
		"--web.listen-address", "127.0.0.1:0",
		"--storage.path", am.dir,
		"--cluster.listen-address", "127.0.0.1:0",
		"--cluster.settle-timeout", "0s",
	}
	if len(am.Opts.FeatureFlags) > 0 {
		args = append(args, "--enable-feature", strings.Join(am.Opts.FeatureFlags, ","))
	}
	if am.Opts.RoutePrefix != "" {
		args = append(args, "--web.route-prefix", am.Opts.RoutePrefix)
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

	// Discover web address from logs
	if err := am.discoverWebAddress(5 * time.Second); err != nil {
		return fmt.Errorf("failed to discover web address: %w", err)
	}

	// Update API client with discovered address
	transport := httptransport.New(am.apiAddr, am.Opts.RoutePrefix+"/api/v2/", nil)
	am.clientV2 = apiclient.New(transport, strfmt.Default)

	// Discover cluster address from API (also serves as readiness check)
	if err := am.discoverClusterAddress(5 * time.Second); err != nil {
		return fmt.Errorf("failed to discover cluster address: %w", err)
	}

	am.T.Logf("Alertmanager started - web: %s, cluster: %s", am.apiAddr, am.clusterAddr)
	return nil
}

// WaitForCluster waits for the Alertmanager instance to join a cluster with the
// given size.
func (am *Alertmanager) WaitForCluster(size int) error {
	params := general.NewGetStatusParams()
	params.WithContext(context.Background())
	var status *general.GetStatusOK

	// Poll for 2s
	for range 20 {
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
	am.T.Helper()
	if am.cmd != nil && am.cmd.Process != nil {
		if err := syscall.Kill(am.cmd.Process.Pid, syscall.SIGTERM); err != nil {
			am.T.Logf("Error sending SIGTERM to Alertmanager process: %v", err)
		}
		am.T.Logf("stdout:\n%v", am.cmd.Stdout)
		am.T.Logf("stderr:\n%v", am.cmd.Stderr)
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
	am.T.Helper()
	if am.cmd.Process != nil {
		if err := syscall.Kill(am.cmd.Process.Pid, syscall.SIGHUP); err != nil {
			am.T.Fatalf("Error sending SIGHUP to Alertmanager process: %v", err)
		}
	}
}

func (am *Alertmanager) cleanup() {
	am.T.Helper()
	if err := os.RemoveAll(am.confFile.Name()); err != nil {
		am.T.Errorf("Error removing test config file %q: %v", am.confFile.Name(), err)
	}
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
		am.T.Fatal(err)
	}
	if err := am.confFile.Sync(); err != nil {
		am.T.Fatal(err)
	}
}

// Client returns a client to interact with the API v2 endpoint.
func (am *Alertmanager) Client() *apiclient.AlertmanagerAPI {
	if am.clientV2 == nil {
		panic("Client not available. Start() was not called or failed.")
	}
	return am.clientV2
}
