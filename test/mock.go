package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/types"
)

type TestAlert struct {
	labels           model.LabelSet
	annotations      model.LabelSet
	startsAt, endsAt float64
}

// At is a convenience method to allow for declarative syntax of Acceptance
// test definitions.
func At(ts float64) float64 {
	return ts
}

type Interval struct {
	start, end float64
}

func (iv Interval) String() string {
	return fmt.Sprintf("[%v,%v]", iv.start, iv.end)
}

func (iv Interval) contains(f float64) bool {
	return f >= iv.start && f <= iv.end
}

// Between is a convenience constructor for an interval for declarative syntax
// of Acceptance test definitions.
func Between(start, end float64) Interval {
	return Interval{start: start, end: end}
}

// alert creates a new alert declaration with the given key/value pairs
// as identifying labels.
func Alert(keyval ...interface{}) *TestAlert {
	if len(keyval)%2 == 1 {
		panic("bad key/values")
	}
	a := &TestAlert{
		labels:      model.LabelSet{},
		annotations: model.LabelSet{},
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
func (a *TestAlert) nativeAlert(opts *AcceptanceOpts) *types.Alert {
	na := &types.Alert{}

	na.Labels = a.labels
	na.Annotations = a.annotations

	if a.startsAt > 0 {
		na.StartsAt = opts.expandTime(a.startsAt)
	}
	if a.endsAt > 0 {
		na.EndsAt = opts.expandTime(a.endsAt)
	}
	return na
}

// Annotate the alert with the given key/value pairs.
func (a *TestAlert) Annotate(keyval ...interface{}) *TestAlert {
	if len(keyval)%2 == 1 {
		panic("bad key/values")
	}

	for i := 0; i < len(keyval); i += 2 {
		ln := model.LabelName(keyval[i].(string))
		lv := model.LabelValue(keyval[i+1].(string))

		a.annotations[ln] = lv
	}

	return a
}

// Active declares the relative activity time for this alert. It
// must be a single starting value or two values where the second value
// declares the resolved time.
func (a *TestAlert) Active(tss ...float64) *TestAlert {
	if len(tss) > 2 || len(tss) == 0 {
		panic("only one or two timestamps allowed")
	}
	if len(tss) == 2 {
		a.endsAt = tss[1]
	}
	a.startsAt = tss[0]

	return a
}

func equalAlerts(a, b *types.Alert, opts *AcceptanceOpts) bool {
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

func equalTime(a, b time.Time, opts *AcceptanceOpts) bool {
	if a.IsZero() != b.IsZero() {
		return false
	}

	diff := a.Sub(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= opts.Tolerance
}

type MockWebhook struct {
	collector *Collector
	addr      string
}

func NewWebhook(addr string, c *Collector) *MockWebhook {
	return &MockWebhook{
		addr:      addr,
		collector: c,
	}
}

func (ws *MockWebhook) Run() {
	http.ListenAndServe(ws.addr, ws)
}

func (ws *MockWebhook) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	dec := json.NewDecoder(req.Body)
	defer req.Body.Close()

	var v notify.WebhookMessage
	if err := dec.Decode(&v); err != nil {
		panic(err)
	}

	ws.collector.add(v.Alerts...)
}
