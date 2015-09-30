package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

type E2ETest struct {
	*testing.T

	opts *E2ETestOpts

	ams        []*Alertmanager
	collectors []*collector

	input    map[float64][]*types.Alert
	expected map[interval][]*types.Alert
}

type E2ETestOpts struct {
	baseTime   time.Time
	timeFactor float64
	tolerance  float64

	conf string
}

func NewE2ETest(t *testing.T, opts *E2ETestOpts) *E2ETest {
	test := &E2ETest{
		T:    t,
		opts: opts,

		input:    map[float64][]*types.Alert{},
		expected: map[interval][]*types.Alert{},
	}
	opts.baseTime = time.Now()

	return test
}

// alertmanager returns a new structure that allows starting an instance
// of Alertmanager on a random port.
func (t *E2ETest) alertmanager() *Alertmanager {
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

	if _, err := cf.WriteString(t.opts.conf); err != nil {
		t.Fatal(err)
	}

	am.url = fmt.Sprintf("http://localhost:%d", 9091)
	am.cmd = exec.Command("../alertmanager", "-config.file", cf.Name(), "-log.level=debug")

	var outb, errb bytes.Buffer
	am.cmd.Stdout = &outb
	am.cmd.Stderr = &errb

	t.ams = append(t.ams, am)

	return am
}

func (t *E2ETest) collector() *collector {
	co := &collector{
		t:         t.T,
		opts:      t.opts,
		collected: map[float64][]*types.Alert{},
		exepected: map[interval][]*types.Alert{},
	}
	t.collectors = append(t.collectors, co)

	return co
}

// Run starts all Alertmanagers and runs queries against them. It then checks
// whether all expected notifications have arrived at the expected destination.
func (t *E2ETest) Run() {
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

	deadline := expandTime(latest, time.Now(), t.opts.timeFactor)
	time.Sleep(deadline.Sub(time.Now()))

	for _, coll := range t.collectors {
		report := coll.check()
		t.Log(report)
	}

	for _, am := range t.ams {
		t.Logf("stdout: %v", am.cmd.Stdout)
		t.Logf("stderr: %v", am.cmd.Stderr)
	}
}

// Alertmanager encapsulates an Alertmanager process and allows
// declaring alerts being pushed to it at fixed points in time.
type Alertmanager struct {
	t    *testing.T
	url  string
	cmd  *exec.Cmd
	opts *E2ETestOpts

	confFile *os.File

	input map[float64][]*types.Alert
}

// push declares alerts that are to be pushed to the Alertmanager
// server at a relative point in time.
func (am *Alertmanager) push(at float64, alerts ...*testAlert) {
	var nas []*types.Alert
	for _, a := range alerts {
		nas = append(nas, a.nativeAlert(am.opts.baseTime, am.opts.timeFactor))
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
		ts := expandTime(at, am.opts.baseTime, am.opts.timeFactor)
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

// collector gathers alerts received by a notification destination
// and verifies whether all arrived and within the correct time boundaries.
type collector struct {
	t    *testing.T
	opts *E2ETestOpts

	collected map[float64][]*types.Alert
	exepected map[interval][]*types.Alert
}

// latest returns the latest relative point in time where a notification is
// expected.
func (c *collector) latest() float64 {
	var latest float64
	for iv := range c.exepected {
		if iv.end > latest {
			latest = iv.end
		}
	}
	return latest
}

// want declares that the collector expects to receive the given alerts
// within the given time boundaries.
func (c *collector) want(iv interval, alerts ...*testAlert) {
	var nas []*types.Alert
	for _, a := range alerts {
		nas = append(nas, a.nativeAlert(c.opts.baseTime, c.opts.timeFactor))
	}

	c.exepected[iv] = append(c.exepected[iv], nas...)
}

// add the given alerts to the collected alerts.
func (c *collector) add(alerts ...*types.Alert) {
	arrival := relativeTime(time.Now(), c.opts.baseTime, c.opts.timeFactor)

	c.collected[arrival] = append(c.collected[arrival], alerts...)
}

func (c *collector) check() string {
	report := fmt.Sprintf("\ncollector %q:\n", c)

	for iv, expected := range c.exepected {
		report += fmt.Sprintf("interval %v\n", iv)

		for _, exp := range expected {
			var found *types.Alert
			report += fmt.Sprintf("  expected: %v\n", exp)

			for at, got := range c.collected {
				if !iv.contains(at) {
					continue
				}
				for _, a := range got {
					if equalAlerts(exp, a, c.opts) {
						found = a
						break
					}
				}
				if found != nil {
					break
				}
			}

			if found != nil {
				report += fmt.Sprintf("  found:   %v\n", found)
			} else {
				c.t.Fail()
				report += fmt.Sprintf("  not found [X]\n")
			}
		}
	}

	// Detect unexpected notifications.
	var totalExp, totalAct int
	for _, exp := range c.exepected {
		totalExp += len(exp)
	}
	for _, act := range c.collected {
		totalAct += len(act)
	}
	if totalExp != totalAct {
		c.t.Fail()
		report += fmt.Sprintf("\nExpected total of %d alerts, got %d", totalExp, totalAct)
	}

	if c.t.Failed() {
		report += "\n\nreceived:\n"

		for at, col := range c.collected {
			for _, a := range col {
				report += fmt.Sprintf("- %v @ %v\n", a.String(), at)
			}
		}
	}

	return report
}

func equalAlerts(a, b *types.Alert, opts *E2ETestOpts) bool {
	if !reflect.DeepEqual(a.Labels, b.Labels) {
		return false
	}
	if !reflect.DeepEqual(a.Annotations, b.Annotations) {
		return false
	}

	if !equalTime(a.StartsAt, b.StartsAt, opts) {
		return false
	}
	if !equalTime(a.EndsAt, b.EndsAt, opts) {
		return false
	}
	return true
}

func equalTime(a, b time.Time, opts *E2ETestOpts) bool {
	if a.IsZero() != b.IsZero() {
		return false
	}

	tol := time.Duration(float64(time.Second) * opts.timeFactor * opts.tolerance)
	diff := a.Sub(b)

	if diff < 0 {
		diff = -diff
	}
	return diff <= tol
}

type testAlert struct {
	labels           model.LabelSet
	annotations      types.Annotations
	startsAt, endsAt float64
}

func expandTime(rel float64, base time.Time, factor float64) time.Time {
	return base.Add(time.Duration(rel * factor * float64(time.Second)))
}

func relativeTime(act time.Time, base time.Time, factor float64) float64 {
	return float64(act.Sub(base)) / factor / float64(time.Second)
}

// at is a convenience method to allow for declarative syntax of e2e
// test definitions.
func at(ts float64) float64 {
	return ts
}

type interval struct {
	start, end float64
}

func (iv interval) String() string {
	return fmt.Sprintf("[%v,%v]", iv.start, iv.end)
}

func (iv interval) contains(f float64) bool {
	return f >= iv.start && f <= iv.end
}

// between is a convenience constructor for an interval for declarative syntax
// of e2e test definitions.
func between(start, end float64) interval {
	return interval{start: start, end: end}
}

// alert creates a new alert declaration with the given key/value pairs
// as identifying labels.
func alert(keyval ...interface{}) *testAlert {
	if len(keyval)%2 == 1 {
		panic("bad key/values")
	}
	a := &testAlert{
		labels:      model.LabelSet{},
		annotations: types.Annotations{},
	}

	for i := 0; i < len(keyval); i += 2 {
		ln := model.LabelName(keyval[i].(string))
		lv := model.LabelValue(keyval[i+1].(string))

		a.labels[ln] = lv
	}

	return a
}

// nativeAlert converts the declared test alert into a full alert based
// on the given paramters.
func (a *testAlert) nativeAlert(base time.Time, f float64) *types.Alert {
	na := &types.Alert{
		Labels:      a.labels,
		Annotations: a.annotations,
	}
	if a.startsAt > 0 {
		na.StartsAt = expandTime(a.startsAt, base, f)
	}
	if a.endsAt > 0 {
		na.EndsAt = expandTime(a.endsAt, base, f)
	}
	return na
}

// annotate the alert with the given key/value pairs.
func (a *testAlert) annotate(keyval ...interface{}) *testAlert {
	if len(keyval)%2 == 1 {
		panic("bad key/values")
	}

	for i := 0; i < len(keyval); i += 2 {
		ln := model.LabelName(keyval[i].(string))
		lv := keyval[i+1].(string)

		a.annotations[ln] = lv
	}

	return a
}

// active declares the relative activity time for this alert. It
// must be a single starting value or two values where the second value
// declares the resolved time.
func (a *testAlert) active(tss ...float64) *testAlert {
	if len(tss) > 2 || len(tss) == 0 {
		panic("only one or two timestamps allowed")
	}
	if len(tss) == 1 {
		a.startsAt = tss[0]
	} else {
		a.endsAt = tss[1]
	}

	return a
}
