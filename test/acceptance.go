package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/types"
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

// Alertmanager returns a new structure that allows starting an instance
// of Alertmanager on a random port.
func (t *AcceptanceTest) Alertmanager() *Alertmanager {
	am := &Alertmanager{
		t:     t.T,
		opts:  t.opts,
		input: map[float64][]*types.Alert{},
	}

	cf, err := ioutil.TempFile("", "am_config")
	if err != nil {
		t.Fatal(err)
	}
	am.confFile = cf

	if _, err := cf.WriteString(t.opts.Config); err != nil {
		t.Fatal(err)
	}

	am.url = fmt.Sprintf("http://localhost:%d", 9091)
	am.cmd = exec.Command("../../alertmanager", "-config.file", cf.Name(), "-log.level=debug")

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
		collected: map[float64][]*types.Alert{},
		exepected: map[Interval][]*types.Alert{},
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
		go am.runQueries()
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
	url  string
	cmd  *exec.Cmd
	opts *AcceptanceOpts

	confFile *os.File

	input map[float64][]*types.Alert
}

// push declares alerts that are to be pushed to the Alertmanager
// server at a relative point in time.
func (am *Alertmanager) Push(at float64, alerts ...*TestAlert) {
	var nas []*types.Alert
	for _, a := range alerts {
		nas = append(nas, a.nativeAlert(am.opts))
	}
	am.input[at] = append(am.input[at], nas...)
}

// start the alertmanager and wait until it is ready to receive.
func (am *Alertmanager) start() {
	if err := am.cmd.Start(); err != nil {
		am.t.Fatalf("Starting alertmanager failed: %s", err)
	}

	time.Sleep(100 * time.Millisecond)
}

// runQueries starts sending the declared alerts over time.
func (am *Alertmanager) runQueries() {
	var wg sync.WaitGroup

	for at, as := range am.input {
		ts := am.opts.expandTime(at)
		wg.Add(1)

		go func(as ...*types.Alert) {
			defer wg.Done()

			time.Sleep(ts.Sub(time.Now()))

			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(as); err != nil {
				am.t.Error(err)
				return
			}

			resp, err := http.Post(am.url+"/api/alerts", "application/json", &buf)
			if err != nil {
				am.t.Error(err)
				return
			}
			resp.Body.Close()
		}(as...)
	}

	wg.Wait()
}

// kill the underlying Alertmanager process and remove intermediate data.
func (am *Alertmanager) kill() {
	am.cmd.Process.Kill()
	os.RemoveAll(am.confFile.Name())
}
