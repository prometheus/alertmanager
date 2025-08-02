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

package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"

	apiclient "github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/api/v2/client/general"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/cli/format"
)

const (
	// nolint:godot
	// amtool is the relative path to local amtool binary.
	amtool = "../../../amtool"
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
	RoutePrefix string
	Tolerance   time.Duration
	baseTime    time.Time
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

// AmtoolOk verifies that the "amtool" file exists in the correct location for testing,
// and is a regular file.
func AmtoolOk() (bool, error) {
	stat, err := os.Stat(amtool)
	if err != nil {
		return false, fmt.Errorf("error accessing amtool command, try 'make build' to generate the file. %w", err)
	} else if stat.IsDir() {
		return false, fmt.Errorf("file %s is a directory, expecting a binary executable file", amtool)
	}
	return true, nil
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
		defer func(am *Alertmanager) {
			am.Terminate()
			am.cleanup()
			t.Logf("stdout:\n%v", am.cmd.Stdout)
			t.Logf("stderr:\n%v", am.cmd.Stderr)
		}(am)
	}

	err := t.amc.Start()
	if err != nil {
		t.Fatal(err)
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
	cmd         *exec.Cmd
	confFile    *os.File
	dir         string

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
			return fmt.Errorf("starting alertmanager cluster: %w", err)
		}
	}

	for _, am := range amc.ams {
		err := am.WaitForCluster(len(amc.ams))
		if err != nil {
			return fmt.Errorf("waiting alertmanager cluster: %w", err)
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
		return fmt.Errorf("starting alertmanager failed: %w", err)
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
			return fmt.Errorf("starting alertmanager failed: expected HTTP status '200', got '%d'", resp.StatusCode)
		}
		_, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("starting alertmanager failed: %w", err)
		}
		return nil
	}
	return fmt.Errorf("starting alertmanager failed: timeout")
}

// WaitForCluster waits for the Alertmanager instance to join a cluster with the
// given size.
func (am *Alertmanager) WaitForCluster(size int) error {
	params := general.NewGetStatusParams()
	params.WithContext(context.Background())
	var status general.GetStatusOK

	// Poll for 2s
	for i := 0; i < 20; i++ {
		status, err := am.clientV2.General.GetStatus(params)
		if err != nil {
			return err
		}

		if len(status.Payload.Cluster.Peers) == size {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf(
		"failed to wait for Alertmanager instance %q to join cluster: expected %v peers, but got %v",
		am.clusterAddr,
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
	if err := syscall.Kill(am.cmd.Process.Pid, syscall.SIGTERM); err != nil {
		am.t.Fatalf("Error sending SIGTERM to Alertmanager process: %v", err)
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
	if err := syscall.Kill(am.cmd.Process.Pid, syscall.SIGHUP); err != nil {
		am.t.Fatalf("Error sending SIGHUP to Alertmanager process: %v", err)
	}
}

func (am *Alertmanager) cleanup() {
	am.t.Helper()
	if err := os.RemoveAll(am.confFile.Name()); err != nil {
		am.t.Errorf("Error removing test config file %q: %v", am.confFile.Name(), err)
	}
}

// Version runs the 'amtool' command with the --version option and checks
// for appropriate output.
func Version() (string, error) {
	cmd := exec.Command(amtool, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	versionRE := regexp.MustCompile(`^amtool, version (\d+\.\d+\.\d+) *`)
	matched := versionRE.FindStringSubmatch(string(out))
	if len(matched) != 2 {
		return "", errors.New("Unable to match version info regex: " + string(out))
	}
	return matched[1], nil
}

// AddAlertsAt declares alerts that are to be added to the Alertmanager
// server at a relative point in time.
func (am *Alertmanager) AddAlertsAt(omitEquals bool, at float64, alerts ...*TestAlert) {
	am.t.Do(at, func() {
		am.AddAlerts(omitEquals, alerts...)
	})
}

// AddAlerts declares alerts that are to be added to the Alertmanager server.
// The omitEquals option omits alertname= from the command line args passed to
// amtool and instead uses the alertname value as the first argument to the command.
// For example `amtool alert add foo` instead of `amtool alert add alertname=foo`.
// This has been added to allow certain tests to test adding alerts both with and
// without alertname=. All other tests that use AddAlerts as a fixture can set this
// to false.
func (am *Alertmanager) AddAlerts(omitEquals bool, alerts ...*TestAlert) {
	for _, alert := range alerts {
		out, err := am.addAlertCommand(omitEquals, alert)
		if err != nil {
			am.t.Errorf("Error adding alert: %v\nOutput: %s", err, string(out))
		}
	}
}

func (am *Alertmanager) addAlertCommand(omitEquals bool, alert *TestAlert) ([]byte, error) {
	amURLFlag := "--alertmanager.url=" + am.getURL("/")
	args := []string{amURLFlag, "alert", "add"}
	// Make a copy of the labels
	labels := make(models.LabelSet, len(alert.labels))
	for k, v := range alert.labels {
		labels[k] = v
	}
	if omitEquals {
		// If alertname is present and omitEquals is true then the command should
		// be `amtool alert add foo ...` and not `amtool alert add alertname=foo ...`.
		if alertname, ok := labels["alertname"]; ok {
			args = append(args, alertname)
			delete(labels, "alertname")
		}
	}
	for k, v := range labels {
		args = append(args, k+"="+v)
	}
	startsAt := strfmt.DateTime(am.opts.expandTime(alert.startsAt))
	args = append(args, "--start="+startsAt.String())
	if alert.endsAt > alert.startsAt {
		endsAt := strfmt.DateTime(am.opts.expandTime(alert.endsAt))
		args = append(args, "--end="+endsAt.String())
	}
	cmd := exec.Command(amtool, args...)
	return cmd.CombinedOutput()
}

// QueryAlerts uses the amtool cli to query alerts.
func (am *Alertmanager) QueryAlerts(match ...string) ([]TestAlert, error) {
	amURLFlag := "--alertmanager.url=" + am.getURL("/")
	args := append([]string{amURLFlag, "alert", "query"}, match...)
	cmd := exec.Command(amtool, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	return parseAlertQueryResponse(output)
}

func parseAlertQueryResponse(data []byte) ([]TestAlert, error) {
	alerts := []TestAlert{}
	lines := strings.Split(string(data), "\n")
	header, lines := lines[0], lines[1:len(lines)-1]
	startTimePos := strings.Index(header, "Starts At")
	if startTimePos == -1 {
		return alerts, errors.New("Invalid header: " + header)
	}
	summPos := strings.Index(header, "Summary")
	if summPos == -1 {
		return alerts, errors.New("Invalid header: " + header)
	}
	for _, line := range lines {
		alertName := strings.TrimSpace(line[0:startTimePos])
		startTime := strings.TrimSpace(line[startTimePos:summPos])
		startsAt, err := time.Parse(format.DefaultDateFormat, startTime)
		if err != nil {
			return alerts, err
		}
		summary := strings.TrimSpace(line[summPos:])
		alert := TestAlert{
			labels:   models.LabelSet{"alertname": alertName},
			startsAt: float64(startsAt.Unix()),
			summary:  summary,
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

// SetSilence updates or creates the given Silence.
func (amc *AlertmanagerCluster) SetSilence(at float64, sil *TestSilence) {
	for _, am := range amc.ams {
		am.SetSilence(at, sil)
	}
}

// SetSilence updates or creates the given Silence.
func (am *Alertmanager) SetSilence(at float64, sil *TestSilence) {
	out, err := am.addSilenceCommand(sil)
	if err != nil {
		am.t.Errorf("Unable to set silence %v %v", err, string(out))
	}
}

// addSilenceCommand adds a silence using the 'amtool silence add' command.
func (am *Alertmanager) addSilenceCommand(sil *TestSilence) ([]byte, error) {
	amURLFlag := "--alertmanager.url=" + am.getURL("/")
	args := []string{amURLFlag, "silence", "add"}
	if sil.comment != "" {
		args = append(args, "--comment="+sil.comment)
	}
	args = append(args, sil.match...)
	cmd := exec.Command(amtool, args...)
	return cmd.CombinedOutput()
}

// QuerySilence queries the current silences using the 'amtool silence query' command.
func (am *Alertmanager) QuerySilence(match ...string) ([]TestSilence, error) {
	amURLFlag := "--alertmanager.url=" + am.getURL("/")
	args := append([]string{amURLFlag, "silence", "query"}, match...)
	cmd := exec.Command(amtool, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		am.t.Error("Silence query command failed: ", err)
	}
	return parseSilenceQueryResponse(out)
}

var silenceHeaderFields = []string{"ID", "Matchers", "Ends At", "Created By", "Comment"}

func parseSilenceQueryResponse(data []byte) ([]TestSilence, error) {
	sils := []TestSilence{}
	lines := strings.Split(string(data), "\n")
	header, lines := lines[0], lines[1:len(lines)-1]
	matchersPos := strings.Index(header, silenceHeaderFields[1])
	if matchersPos == -1 {
		return sils, errors.New("Invalid header: " + header)
	}
	endsAtPos := strings.Index(header, silenceHeaderFields[2])
	if endsAtPos == -1 {
		return sils, errors.New("Invalid header: " + header)
	}
	createdByPos := strings.Index(header, silenceHeaderFields[3])
	if createdByPos == -1 {
		return sils, errors.New("Invalid header: " + header)
	}
	commentPos := strings.Index(header, silenceHeaderFields[4])
	if commentPos == -1 {
		return sils, errors.New("Invalid header: " + header)
	}
	for _, line := range lines {
		id := strings.TrimSpace(line[0:matchersPos])
		matchers := strings.TrimSpace(line[matchersPos:endsAtPos])
		endsAtString := strings.TrimSpace(line[endsAtPos:createdByPos])
		endsAt, err := time.Parse(format.DefaultDateFormat, endsAtString)
		if err != nil {
			return sils, err
		}
		createdBy := strings.TrimSpace(line[createdByPos:commentPos])
		comment := strings.TrimSpace(line[commentPos:])
		silence := TestSilence{
			id:        id,
			endsAt:    float64(endsAt.Unix()),
			match:     strings.Split(matchers, " "),
			createdBy: createdBy,
			comment:   comment,
		}
		sils = append(sils, silence)
	}
	return sils, nil
}

// DelSilence deletes the silence with the sid at the given time.
func (amc *AlertmanagerCluster) DelSilence(at float64, sil *TestSilence) {
	for _, am := range amc.ams {
		am.DelSilence(at, sil)
	}
}

// DelSilence deletes the silence with the sid at the given time.
func (am *Alertmanager) DelSilence(at float64, sil *TestSilence) {
	output, err := am.expireSilenceCommand(sil)
	if err != nil {
		am.t.Errorf("Error expiring silence %v: %s", string(output), err)
		return
	}
}

// expireSilenceCommand expires a silence using the 'amtool silence expire' command.
func (am *Alertmanager) expireSilenceCommand(sil *TestSilence) ([]byte, error) {
	amURLFlag := "--alertmanager.url=" + am.getURL("/")
	args := []string{amURLFlag, "silence", "expire", sil.ID()}
	cmd := exec.Command(amtool, args...)
	return cmd.CombinedOutput()
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
		return
	}
	if err := am.confFile.Sync(); err != nil {
		am.t.Fatal(err)
		return
	}
}

func (am *Alertmanager) ShowRoute() ([]byte, error) {
	return am.showRouteCommand()
}

func (am *Alertmanager) showRouteCommand() ([]byte, error) {
	amURLFlag := "--alertmanager.url=" + am.getURL("/")
	args := []string{amURLFlag, "config", "routes", "show"}
	cmd := exec.Command(amtool, args...)
	return cmd.CombinedOutput()
}

func (am *Alertmanager) TestRoute(labels ...string) ([]byte, error) {
	return am.testRouteCommand(labels...)
}

func (am *Alertmanager) testRouteCommand(labels ...string) ([]byte, error) {
	amURLFlag := "--alertmanager.url=" + am.getURL("/")
	args := append([]string{amURLFlag, "config", "routes", "test"}, labels...)
	cmd := exec.Command(amtool, args...)
	return cmd.CombinedOutput()
}

func (am *Alertmanager) getURL(path string) string {
	return fmt.Sprintf("http://%s%s%s", am.apiAddr, am.opts.RoutePrefix, path)
}
