// Copyright The Prometheus Authors
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

package mem

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/store"
	"github.com/prometheus/alertmanager/types"
)

const alertChannelLength = 200

var tracer = otel.Tracer("github.com/prometheus/alertmanager/provider/mem")

// Alerts gives access to a set of alerts. All methods are goroutine-safe.
type Alerts struct {
	cancel context.CancelFunc

	mtx sync.Mutex

	alerts *store.Alerts
	marker types.AlertMarker

	listeners map[int]listeningAlerts
	next      int

	callback AlertStoreCallback

	logger     *slog.Logger
	propagator propagation.TextMapPropagator
	flagger    featurecontrol.Flagger

	alertsLimit             prometheus.Gauge
	alertsLimitedTotal      *prometheus.CounterVec
	subscriberChannelWrites *prometheus.CounterVec
}

type AlertStoreCallback interface {
	// PreStore is called before alert is stored into the store. If this method returns error,
	// alert is not stored.
	// Existing flag indicates whether alert has existed before (and is only updated) or not.
	// If alert has existed before, then alert passed to PreStore is result of merging existing alert with new alert.
	PreStore(alert *types.Alert, existing bool) error

	// PostStore is called after alert has been put into store.
	PostStore(alert *types.Alert, existing bool)

	// PostDelete is called after alert have been removed from the store due to alert garbage collection.
	PostDelete(alert *types.Alert)

	// PostGC is called after alerts have been removed from the store due to alert garbage collection.
	PostGC(fingerprints model.Fingerprints)
}

type listeningAlerts struct {
	name   string
	alerts chan *provider.Alert
	done   chan struct{}
}

func (a *Alerts) registerMetrics(r prometheus.Registerer) {
	r.MustRegister(&alertsCollector{alerts: a})

	a.alertsLimit = promauto.With(r).NewGauge(prometheus.GaugeOpts{
		Name: "alertmanager_alerts_per_alert_limit",
		Help: "Current limit on number of alerts per alert name",
	})

	labels := []string{}
	if a.flagger.EnableAlertNamesInMetrics() {
		labels = append(labels, "alertname")
	}
	a.alertsLimitedTotal = promauto.With(r).NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmanager_alerts_limited_total",
			Help: "Total number of alerts that were dropped due to per alert name limit",
		},
		labels,
	)

	a.subscriberChannelWrites = promauto.With(r).NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmanager_alerts_subscriber_channel_writes_total",
			Help: "Number of times alerts were written to subscriber channels",
		},
		[]string{"subscriber"},
	)
}

// NewAlerts returns a new alert provider.
func NewAlerts(
	ctx context.Context,
	m types.AlertMarker,
	intervalGC time.Duration,
	perAlertNameLimit int,
	alertCallback AlertStoreCallback,
	l *slog.Logger,
	r prometheus.Registerer,
	flagger featurecontrol.Flagger,
) (*Alerts, error) {
	if alertCallback == nil {
		alertCallback = noopCallback{}
	}

	if perAlertNameLimit > 0 {
		l.Info("per alert name limit enabled", "limit", perAlertNameLimit)
	}

	if flagger == nil {
		flagger = featurecontrol.NoopFlags{}
	}

	ctx, cancel := context.WithCancel(ctx)
	a := &Alerts{
		marker:     m,
		alerts:     store.NewAlerts().WithPerAlertLimit(perAlertNameLimit),
		cancel:     cancel,
		listeners:  map[int]listeningAlerts{},
		next:       0,
		logger:     l.With("component", "provider"),
		propagator: otel.GetTextMapPropagator(),
		callback:   alertCallback,
		flagger:    flagger,
	}

	if r != nil {
		a.registerMetrics(r)
		a.alertsLimit.Set(float64(perAlertNameLimit))
	}

	go a.gcLoop(ctx, intervalGC)

	return a, nil
}

func (a *Alerts) gcLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.gc()
		}
	}
}

func (a *Alerts) gc() {
	a.gcListeners()

	// As we don't persist alerts, we no longer consider them after
	// they are resolved. Alerts waiting for resolved notifications are
	// held in memory in aggregation groups redundantly.
	deleted := a.gcAlerts()

	// If there are no deleted alerts, there is nothing to do.
	if len(deleted) == 0 {
		return
	}

	// Delete markers for deleted alerts.
	ff := make(model.Fingerprints, len(deleted))
	for i, alert := range deleted {
		ff[i] = alert.Fingerprint()
		a.callback.PostDelete(alert)
	}
	a.marker.Delete(ff...)
	a.callback.PostGC(ff)
}

func (a *Alerts) gcAlerts() []*types.Alert {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.alerts.GC()
}

func (a *Alerts) gcListeners() {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	for i, l := range a.listeners {
		select {
		case <-l.done:
			delete(a.listeners, i)
			close(l.alerts)
		default:
			// listener is not closed yet, hence proceed.
		}
	}
}

// Close the alert provider.
func (a *Alerts) Close() {
	if a.cancel != nil {
		a.cancel()
	}
}

// Subscribe returns an iterator over active alerts that have not been
// resolved and successfully notified about.
// They are not guaranteed to be in chronological order.
func (a *Alerts) Subscribe(name string) provider.AlertIterator {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	var (
		done   = make(chan struct{})
		alerts = a.alerts.List()
		ch     = make(chan *provider.Alert, max(len(alerts), alertChannelLength))
	)

	for _, a := range alerts {
		ch <- &provider.Alert{
			Header: map[string]string{},
			Data:   a,
		}
	}

	a.listeners[a.next] = listeningAlerts{name: name, alerts: ch, done: done}
	a.next++

	return provider.NewAlertIterator(ch, done, nil)
}

func (a *Alerts) SlurpAndSubscribe(name string) ([]*types.Alert, provider.AlertIterator) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	var (
		done   = make(chan struct{})
		alerts = a.alerts.List()
		ch     = make(chan *provider.Alert, alertChannelLength)
	)

	a.listeners[a.next] = listeningAlerts{name: name, alerts: ch, done: done}
	a.next++

	return alerts, provider.NewAlertIterator(ch, done, nil)
}

// GetPending returns an iterator over all the alerts that have
// pending notifications.
func (a *Alerts) GetPending() provider.AlertIterator {
	var (
		ch   = make(chan *provider.Alert, alertChannelLength)
		done = make(chan struct{})
	)
	a.mtx.Lock()
	defer a.mtx.Unlock()
	alerts := a.alerts.List()

	go func() {
		defer close(ch)
		for _, a := range alerts {
			select {
			case ch <- &provider.Alert{
				Header: map[string]string{},
				Data:   a,
			}:
			case <-done:
				return
			}
		}
	}()

	return provider.NewAlertIterator(ch, done, nil)
}

// Get returns the alert for a given fingerprint.
func (a *Alerts) Get(fp model.Fingerprint) (*types.Alert, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.alerts.Get(fp)
}

// Put adds the given alert to the set.
func (a *Alerts) Put(ctx context.Context, alerts ...*types.Alert) error {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	ctx, span := tracer.Start(ctx, "provider.mem.Put",
		trace.WithAttributes(
			attribute.Int("alerting.alerts.count", len(alerts)),
		),
		trace.WithSpanKind(trace.SpanKindProducer),
	)
	defer span.End()

	for _, alert := range alerts {
		fp := alert.Fingerprint()

		existing := false

		// Check that there's an alert existing within the store before
		// trying to merge.
		if old, err := a.alerts.Get(fp); err == nil {
			existing = true

			// Merge alerts if there is an overlap in activity range.
			if (alert.EndsAt.After(old.StartsAt) && alert.EndsAt.Before(old.EndsAt)) ||
				(alert.StartsAt.After(old.StartsAt) && alert.StartsAt.Before(old.EndsAt)) {
				alert = old.Merge(alert)
			}
		}

		if err := a.callback.PreStore(alert, existing); err != nil {
			a.logger.Error("pre-store callback returned error on set alert", "err", err)
			continue
		}

		if err := a.alerts.Set(alert); err != nil {
			a.logger.Warn("error on set alert", "alertname", alert.Name(), "err", err)
			if errors.Is(err, store.ErrLimited) {
				labels := []string{}
				if a.flagger.EnableAlertNamesInMetrics() {
					labels = append(labels, alert.Name())
				}
				a.alertsLimitedTotal.WithLabelValues(labels...).Inc()
			}
			continue
		}

		a.callback.PostStore(alert, existing)

		metadata := map[string]string{}
		a.propagator.Inject(ctx, propagation.MapCarrier(metadata))
		msg := &provider.Alert{
			Data:   alert,
			Header: metadata,
		}

		for _, l := range a.listeners {
			select {
			case l.alerts <- msg:
				a.subscriberChannelWrites.WithLabelValues(l.name).Inc()
			case <-l.done:
			}
		}
	}

	return nil
}

// countByState returns the number of non-resolved alerts by state.
func (a *Alerts) countByState() (active, suppressed, unprocessed int) {
	for _, alert := range a.alerts.List() {
		if alert.Resolved() {
			continue
		}

		switch a.marker.Status(alert.Fingerprint()).State {
		case types.AlertStateActive:
			active++
		case types.AlertStateSuppressed:
			suppressed++
		case types.AlertStateUnprocessed:
			unprocessed++
		}
	}
	return active, suppressed, unprocessed
}

// alertsCollector implements prometheus.Collector to collect all alert count metrics in a single pass.
type alertsCollector struct {
	alerts *Alerts
}

var alertsDesc = prometheus.NewDesc(
	"alertmanager_alerts",
	"How many alerts by state.",
	[]string{"state"}, nil,
)

func (c *alertsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- alertsDesc
}

func (c *alertsCollector) Collect(ch chan<- prometheus.Metric) {
	active, suppressed, unprocessed := c.alerts.countByState()

	ch <- prometheus.MustNewConstMetric(alertsDesc, prometheus.GaugeValue, float64(active), string(types.AlertStateActive))
	ch <- prometheus.MustNewConstMetric(alertsDesc, prometheus.GaugeValue, float64(suppressed), string(types.AlertStateSuppressed))
	ch <- prometheus.MustNewConstMetric(alertsDesc, prometheus.GaugeValue, float64(unprocessed), string(types.AlertStateUnprocessed))
}

type noopCallback struct{}

func (n noopCallback) PreStore(_ *types.Alert, _ bool) error { return nil }
func (n noopCallback) PostStore(_ *types.Alert, _ bool)      {}
func (n noopCallback) PostDelete(_ *types.Alert)             {}
func (n noopCallback) PostGC(_ model.Fingerprints)           {}
