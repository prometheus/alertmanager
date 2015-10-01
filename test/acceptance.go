package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/api/alertmanager"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
)

type AcceptanceTest struct {
	*testing.T

	opts *AcceptanceOpts

	ams        []*Alertmanager
	collectors []*Collector
}

type AcceptanceOpts struct {
	baseTime  time.Time
	Tolerance time.Duration

	Config string
}

func (opts *AcceptanceOpts) expandTime(rel float64) time.Time {
	return opts.baseTime.Add(time.Duration(rel * float64(time.Second)))
}

func (opts *AcceptanceOpts) relativeTime(act time.Time) float64 {
	return float64(act.Sub(opts.baseTime)) / float64(time.Second)
}

func NewAcceptanceTest(t *testing.T, opts *AcceptanceOpts) *AcceptanceTest {
	test := &AcceptanceTest{
		T:    t,
		opts: opts,
	}
	opts.baseTime = time.Now()

	return test
}

func freeAddress() string {
	// Let the OS allocate a free address, close it and hope
	// it is still free when starting Alertmanager.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	l.Close()

	return l.Addr().String()
}

// Alertmanager returns a new structure that allows starting an instance
// of Alertmanager on a random port.
func (t *AcceptanceTest) Alertmanager() *Alertmanager {
	am := &Alertmanager{
		t:       t.T,
		opts:    t.opts,
		actions: map[float64][]func(){},
	}

	cf, err := ioutil.TempFile("", "am_config")
	if err != nil {
		t.Fatal(err)
	}
	am.confFile = cf

	if _, err := cf.WriteString(t.opts.Config); err != nil {
		t.Fatal(err)
	}

	am.addr = freeAddress()

	client, err := alertmanager.New(alertmanager.Config{
		Address: fmt.Sprintf("http://%s", am.addr),
	})
	if err != nil {
		t.Error(err)
	}
	am.client = client

	am.cmd = exec.Command("../../alertmanager",
		"-config.file", cf.Name(),
		"-log.level", "debug",
		"-web.listen-address", am.addr,
	)

	var outb, errb bytes.Buffer
	am.cmd.Stdout = &outb
	am.cmd.Stderr = &errb

	t.ams = append(t.ams, am)

	return am
}

func (t *AcceptanceTest) Collector(name string) *Collector {
	co := &Collector{
		t:         t.T,
		name:      name,
		opts:      t.opts,
		collected: map[float64][]model.Alerts{},
		exepected: map[Interval][]model.Alerts{},
	}
	t.collectors = append(t.collectors, co)

	return co
}

// Run starts all Alertmanagers and runs queries against them. It then checks
// whether all expected notifications have arrived at the expected destination.
func (t *AcceptanceTest) Run() {
	for _, am := range t.ams {
		am.start()
		defer am.kill()
	}

	for _, am := range t.ams {
		go am.runActions()
	}

	var latest float64
	for _, coll := range t.collectors {
		if l := coll.latest(); l > latest {
			latest = l
		}
	}

	deadline := t.opts.expandTime(latest)
	time.Sleep(deadline.Sub(time.Now()))

	for _, coll := range t.collectors {
		report := coll.check()
		t.Log(report)
	}

	for _, am := range t.ams {
		t.Logf("stdout:\n%v", am.cmd.Stdout)
		t.Logf("stderr:\n%v", am.cmd.Stderr)
	}
}

// Alertmanager encapsulates an Alertmanager process and allows
// declaring alerts being pushed to it at fixed points in time.
type Alertmanager struct {
	t    *testing.T
	opts *AcceptanceOpts

	addr     string
	client   alertmanager.Client
	cmd      *exec.Cmd
	confFile *os.File

	actions map[float64][]func()
}

// Push declares alerts that are to be pushed to the Alertmanager
// server at a relative point in time.
func (am *Alertmanager) Push(at float64, alerts ...*TestAlert) {
	var nas model.Alerts
	for _, a := range alerts {
		nas = append(nas, a.nativeAlert(am.opts))
	}

	am.Do(at, func() {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(nas); err != nil {
			am.t.Error(err)
			return
		}

		resp, err := http.Post(fmt.Sprintf("http://%s/api/alerts", am.addr), "application/json", &buf)
		if err != nil {
			am.t.Error(err)
			return
		}
		resp.Body.Close()
	})
}

// SetSilence updates or creates the given Silence.
func (am *Alertmanager) SetSilence(at float64, sil *TestSilence) {
	silences := alertmanager.NewSilenceAPI(am.client)

	am.Do(at, func() {
		sid, err := silences.Set(context.Background(), sil.nativeSilence(am.opts))
		if err != nil {
			am.t.Error(err)
			return
		}
		sil.ID = sid
	})
}

// DelSilence deletes the silence with the sid at the given time.
func (am *Alertmanager) DelSilence(at float64, sil *TestSilence) {
	silences := alertmanager.NewSilenceAPI(am.client)

	am.Do(at, func() {
		if err := silences.Del(context.Background(), sil.ID); err != nil {
			am.t.Error(err)
		}
	})
}

// Do sets the given function to be executed at the given time.
func (am *Alertmanager) Do(at float64, f func()) {
	am.actions[at] = append(am.actions[at], f)
}

// start the alertmanager and wait until it is ready to receive.
func (am *Alertmanager) start() {
	if err := am.cmd.Start(); err != nil {
		am.t.Fatalf("Starting alertmanager failed: %s", err)
	}

	time.Sleep(100 * time.Millisecond)
}

// runActions performs the stored actions at the defined times.
func (am *Alertmanager) runActions() {
	var wg sync.WaitGroup

	for at, fs := range am.actions {
		ts := am.opts.expandTime(at)
		wg.Add(len(fs))

		for _, f := range fs {
			go func() {
				time.Sleep(ts.Sub(time.Now()))
				f()
				wg.Done()
			}()
		}
	}

	wg.Wait()
}

// kill the underlying Alertmanager process and remove intermediate data.
func (am *Alertmanager) kill() {
	am.cmd.Process.Kill()
	os.RemoveAll(am.confFile.Name())
}
