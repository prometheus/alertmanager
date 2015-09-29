package test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/types"
)

type webhookServer struct {
	collector *collector
}

func (ws *webhookServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	dec := json.NewDecoder(req.Body)
	defer req.Body.Close()

	var v notify.WebhookMessage
	if err := dec.Decode(&v); err != nil {
		panic(err)
	}

	ws.collector.Add(v.Alerts...)
}

type collector struct {
	alerts []*types.Alert
}

func (c *collector) Add(alerts ...*types.Alert) {
	c.alerts = append(c.alerts, alerts...)
}

// type Alertmanager struct {
// 	cmd  *exec.Cmd
// 	opts *E2ETestOpts
// }

type E2ETest struct {
	*testing.T

	ams      []*Alertmanager
	cmd      *exec.Cmd
	baseTime time.Time
	opts     *E2ETestOpts

	recorder *webhookServer

	input    map[float64][]*types.Alert
	expected map[interval][]*types.Alert
}

type E2ETestOpts struct {
	timeFactor float64
	conf       string
}

func NewE2ETest(t *testing.T, opts *E2ETestOpts) *E2ETest {
	return &E2ETest{
		T:          t,
		baseTime:   time.Now(),
		timeFactor: opts.timeFactor,
		amURL:      "http://localhost:9091/api/alerts",
		opts:       opts,

		recorder: &webhookServer{
			collector: &collector{},
		},

		input:    map[float64][]*types.Alert{},
		expected: map[interval][]*types.Alert{},
	}
}

// func (t *E2ETest) alertmanager() *Alertmanager

func (t *E2ETest) Run() {
	cf, err := ioutil.TempFile("", "am_config")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cf.Name())

	if _, err := cf.WriteString(t.opts.conf); err != nil {
		t.Fatal(err)
	}

	t.cmd = exec.Command("../alertmanager", "-config.file", cf.Name(), "-log.level=debug")

	var outb bytes.Buffer
	var errb bytes.Buffer
	t.cmd.Stdout = &outb
	t.cmd.Stderr = &errb

	if err := t.cmd.Start(); err != nil {
		t.Fatalf("Starting alertmanager failed: %s", err)
	}

	go http.ListenAndServe(":8088", t.recorder)

	var wg sync.WaitGroup

	for at, as := range t.input {
		ts := expandTime(at, t.baseTime, t.timeFactor)
		wg.Add(1)

		go func(as ...*types.Alert) {
			defer wg.Done()

			time.Sleep(ts.Sub(time.Now()))

			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(as); err != nil {
				t.Error(err)
				return
			}

			resp, err := http.Post(t.amURL, "application/json", &buf)
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
		}(as...)
	}

	wg.Wait()

	time.Sleep(time.Duration(float64(2*time.Second) * t.timeFactor))

	t.Logf("%s", t.cmd.Stderr)
	t.Logf("%s", t.cmd.Stdout)
	t.Logf("received %v", t.recorder.collector.alerts)
	t.Fail()

	t.cmd.Process.Kill()
	t.cmd.Wait()
}

func at(ts float64) float64 {
	return ts
}

type interval struct {
	start, end float64
}

func between(start, end float64) interval {
	return interval{
		start: start,
		end:   end,
	}
}

func (t *E2ETest) push(at float64, alerts ...*testAlert) {
	var nas []*types.Alert
	for _, a := range alerts {
		nas = append(nas, a.nativeAlert(t.baseTime, t.timeFactor))
	}
	t.input[at] = append(t.input[at], nas...)
}

func (t *E2ETest) want(iv interval, alert *testAlert) {
	t.expected[iv] = append(t.expected[iv], alert.nativeAlert(t.baseTime, t.timeFactor))
}

type testAlert struct {
	labels           model.LabelSet
	annotations      types.Annotations
	startsAt, endsAt float64
}

func expandTime(rel float64, base time.Time, factor float64) time.Time {
	return base.Add(time.Duration(rel*factor) * time.Second)
}

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
