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

package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/timeinterval"
	"github.com/prometheus/alertmanager/tracing"
)

var tracer = tracing.NewTracer("github.com/prometheus/alertmanager/notify")

// ResolvedSender returns true if resolved notifications should be sent.
type ResolvedSender interface {
	SendResolved() bool
}

// Peer represents the cluster node from where we are the sending the notification.
type Peer interface {
	// WaitReady waits until the node silences and notifications have settled before attempting to send a notification.
	WaitReady(context.Context) error
}

// MinTimeout is the minimum timeout that is set for the context of a call
// to a notification pipeline.
const MinTimeout = 10 * time.Second

// Notifier notifies about alerts under constraints of the given context. It
// returns an error if unsuccessful and a flag whether the error is
// recoverable. This information is useful for a retry logic.
type Notifier interface {
	Notify(context.Context, ...*alert.Alert) (bool, error)
}

// Integration wraps a notifier and its configuration to be uniquely identified
// by name and index from its origin in the configuration.
type Integration struct {
	notifier     Notifier
	rs           ResolvedSender
	name         string
	idx          int
	receiverName string
}

// NewIntegration returns a new integration.
func NewIntegration(notifier Notifier, rs ResolvedSender, name string, idx int, receiverName string) Integration {
	return Integration{
		notifier:     notifier,
		rs:           rs,
		name:         name,
		idx:          idx,
		receiverName: receiverName,
	}
}

// Notify implements the Notifier interface.
func (i *Integration) Notify(ctx context.Context, alerts ...*alert.Alert) (recoverable bool, err error) {
	ctx, span := tracer.Start(ctx, "notify.Integration.Notify",
		trace.WithAttributes(attribute.String("alerting.notify.integration.name", i.name)),
		trace.WithAttributes(attribute.Int("alerting.alerts.count", len(alerts))),
		trace.WithSpanKind(trace.SpanKindClient),
	)

	defer func() {
		span.SetAttributes(attribute.Bool("alerting.notify.error.recoverable", recoverable))
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
		}
		span.End()
	}()

	recoverable, err = i.notifier.Notify(ctx, alerts...)
	return recoverable, err
}

// SendResolved implements the ResolvedSender interface.
func (i *Integration) SendResolved() bool {
	return i.rs.SendResolved()
}

// Name returns the name of the integration.
func (i *Integration) Name() string {
	return i.name
}

// Index returns the index of the integration.
func (i *Integration) Index() int {
	return i.idx
}

// String implements the Stringer interface.
func (i *Integration) String() string {
	return fmt.Sprintf("%s[%d]", i.name, i.idx)
}

// A Stage processes alerts under the constraints of the given context.
type Stage interface {
	Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error)
}

// StageFunc wraps a function to represent a Stage.
type StageFunc func(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error)

// Exec implements Stage interface.
func (f StageFunc) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	return f(ctx, l, alerts...)
}

type NotificationLog interface {
	Log(r *nflogpb.Receiver, gkey string, firingAlerts, resolvedAlerts []uint64, store *nflog.Store, expiry time.Duration) error
	Query(params ...nflog.QueryParam) ([]*nflogpb.Entry, error)
}

type Metrics struct {
	numNotifications                   *prometheus.CounterVec
	numTotalFailedNotifications        *prometheus.CounterVec
	numNotificationRequestsTotal       *prometheus.CounterVec
	numNotificationRequestsFailedTotal *prometheus.CounterVec
	numNotificationSuppressedTotal     *prometheus.CounterVec
	notificationLatencySeconds         *prometheus.HistogramVec

	ff featurecontrol.Flagger
}

func NewMetrics(r prometheus.Registerer, ff featurecontrol.Flagger) *Metrics {
	labels := []string{"integration"}

	if ff.EnableReceiverNamesInMetrics() {
		labels = append(labels, "receiver_name")
	}

	m := &Metrics{
		numNotifications: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notifications_total",
			Help:      "The total number of attempted notifications.",
		}, labels),
		numTotalFailedNotifications: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notifications_failed_total",
			Help:      "The total number of failed notifications.",
		}, append(labels, "reason")),
		numNotificationRequestsTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notification_requests_total",
			Help:      "The total number of attempted notification requests.",
		}, labels),
		numNotificationRequestsFailedTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notification_requests_failed_total",
			Help:      "The total number of failed notification requests.",
		}, labels),
		numNotificationSuppressedTotal: promauto.With(r).NewCounterVec(prometheus.CounterOpts{
			Namespace: "alertmanager",
			Name:      "notifications_suppressed_total",
			Help:      "The total number of notifications suppressed for being silenced, inhibited, outside of active time intervals or within muted time intervals.",
		}, []string{"reason"}),
		notificationLatencySeconds: promauto.With(r).NewHistogramVec(prometheus.HistogramOpts{
			Namespace:                       "alertmanager",
			Name:                            "notification_latency_seconds",
			Help:                            "The latency of notifications in seconds.",
			Buckets:                         []float64{1, 5, 10, 15, 20},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		}, labels),
		ff: ff,
	}

	return m
}

func (m *Metrics) InitializeFor(receiver map[string][]Integration) {
	if m.ff.EnableReceiverNamesInMetrics() {

		// Reset the vectors to take into account receiver names changing after hot reloads.
		m.numNotifications.Reset()
		m.numNotificationRequestsTotal.Reset()
		m.numNotificationRequestsFailedTotal.Reset()
		m.notificationLatencySeconds.Reset()
		m.numTotalFailedNotifications.Reset()

		for name, integrations := range receiver {
			for _, integration := range integrations {

				m.numNotifications.WithLabelValues(integration.Name(), name)
				m.numNotificationRequestsTotal.WithLabelValues(integration.Name(), name)
				m.numNotificationRequestsFailedTotal.WithLabelValues(integration.Name(), name)
				m.notificationLatencySeconds.WithLabelValues(integration.Name(), name)

				for _, reason := range possibleFailureReasonCategory {
					m.numTotalFailedNotifications.WithLabelValues(integration.Name(), name, reason)
				}
			}
		}

		return
	}

	// When the feature flag is not enabled, we just carry on registering _all_ the integrations.
	for _, integration := range []string{
		"email",
		"pagerduty",
		"wechat",
		"pushover",
		"slack",
		"opsgenie",
		"webhook",
		"victorops",
		"sns",
		"telegram",
		"discord",
		"webex",
		"msteams",
		"msteamsv2",
		"incidentio",
		"jira",
		"rocketchat",
		"mattermost",
	} {
		m.numNotifications.WithLabelValues(integration)
		m.numNotificationRequestsTotal.WithLabelValues(integration)
		m.numNotificationRequestsFailedTotal.WithLabelValues(integration)
		m.notificationLatencySeconds.WithLabelValues(integration)

		for _, reason := range possibleFailureReasonCategory {
			m.numTotalFailedNotifications.WithLabelValues(integration, reason)
		}
	}
}

type PipelineBuilder struct {
	metrics  *Metrics
	ff       featurecontrol.Flagger
	recorder eventrecorder.Recorder
}

func NewPipelineBuilder(r prometheus.Registerer, ff featurecontrol.Flagger, recorder eventrecorder.Recorder) *PipelineBuilder {
	return &PipelineBuilder{
		metrics:  NewMetrics(r, ff),
		ff:       ff,
		recorder: recorder,
	}
}

// New returns a map of receivers to Stages.
func (pb *PipelineBuilder) New(
	receivers map[string][]Integration,
	wait func() time.Duration,
	inhibitor *inhibit.Inhibitor,
	silencer *silence.Silencer,
	intervener *timeinterval.Intervener,
	marker marker.GroupMarker,
	notificationLog NotificationLog,
	peer Peer,
) RoutingStage {
	rs := make(RoutingStage, len(receivers))

	ms := NewClusterGossipSettleStage(peer)
	is := NewMuteStage(inhibitor, pb.metrics)
	tas := NewTimeActiveStage(intervener, marker, pb.metrics)
	tms := NewTimeMuteStage(intervener, marker, pb.metrics)
	ss := NewMuteStage(silencer, pb.metrics)

	for name := range receivers {
		st := createReceiverStage(name, receivers[name], wait, notificationLog, pb.metrics, pb.recorder)
		rs[name] = MultiStage{ms, is, tas, tms, ss, st}
	}

	pb.metrics.InitializeFor(receivers)

	return rs
}

// createReceiverStage creates a pipeline of stages for a receiver.
func createReceiverStage(
	name string,
	integrations []Integration,
	wait func() time.Duration,
	notificationLog NotificationLog,
	metrics *Metrics,
	recorder eventrecorder.Recorder,
) Stage {
	var fs FanoutStage
	for i := range integrations {
		recv := &nflogpb.Receiver{
			GroupName:   name,
			Integration: integrations[i].Name(),
			Idx:         uint32(integrations[i].Index()),
		}
		var s MultiStage
		s = append(s, NewClusterWaitStage(wait))
		s = append(s, NewDedupStage(&integrations[i], notificationLog, recv))
		s = append(s, NewRetryStage(integrations[i], name, metrics, recorder))
		s = append(s, NewSetNotifiesStage(notificationLog, recv))

		fs = append(fs, s)
	}
	return fs
}

// RoutingStage executes the inner stages based on the receiver specified in
// the context.
type RoutingStage map[string]Stage

// Exec implements the Stage interface.
func (rs RoutingStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	receiver, ok := ReceiverName(ctx)
	if !ok {
		return ctx, nil, errors.New("receiver missing")
	}

	ctx, span := tracer.Start(ctx, "notify.RoutingStage.Exec",
		trace.WithAttributes(
			attribute.String("alerting.notify.receiver.name", receiver),
			attribute.Int("alerting.alerts.count", len(alerts)),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	s, ok := rs[receiver]
	if !ok {
		return ctx, nil, errors.New("stage for receiver missing")
	}

	return s.Exec(ctx, l, alerts...)
}

// A MultiStage executes a series of stages sequentially.
type MultiStage []Stage

// Exec implements the Stage interface.
func (ms MultiStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	var err error
	for _, s := range ms {
		if len(alerts) == 0 {
			return ctx, nil, nil
		}

		ctx, alerts, err = s.Exec(ctx, l, alerts...)
		if err != nil {
			return ctx, nil, err
		}
	}
	return ctx, alerts, nil
}

// FanoutStage executes its stages concurrently.
type FanoutStage []Stage

// Exec attempts to execute all stages concurrently and discards the results.
// It returns its input alerts and an error if one or more stages fail.
func (fs FanoutStage) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	var (
		wg   sync.WaitGroup
		mtx  sync.Mutex
		errs error
	)
	wg.Add(len(fs))

	for _, s := range fs {
		go func(s Stage) {
			if _, _, err := s.Exec(ctx, l, alerts...); err != nil {
				mtx.Lock()
				errs = errors.Join(errs, err)
				mtx.Unlock()
			}
			wg.Done()
		}(s)
	}
	wg.Wait()

	return ctx, alerts, errs
}

const (
	SuppressedReasonSilence            = "silence"
	SuppressedReasonInhibition         = "inhibition"
	SuppressedReasonMuteTimeInterval   = "mute_time_interval"
	SuppressedReasonActiveTimeInterval = "active_time_interval"
)

func utcNow() time.Time {
	return time.Now().UTC()
}

// Wrap a slice in a struct so we can store a pointer in sync.Pool.
type hashBuffer struct {
	buf []byte
}

var hashBuffers = sync.Pool{
	New: func() any { return &hashBuffer{buf: make([]byte, 0, 1024)} },
}

func hashAlert(a *alert.Alert) uint64 {
	const sep = '\xff'

	hb := hashBuffers.Get().(*hashBuffer)
	defer hashBuffers.Put(hb)
	b := hb.buf[:0]

	names := make(model.LabelNames, 0, len(a.Labels))

	for ln := range a.Labels {
		names = append(names, ln)
	}
	sort.Sort(names)

	for _, ln := range names {
		b = append(b, string(ln)...)
		b = append(b, sep)
		b = append(b, string(a.Labels[ln])...)
		b = append(b, sep)
	}

	hash := xxhash.Sum64(b)

	return hash
}

type NotifyReason int

const (
	ReasonDoNotNotify NotifyReason = iota
	ReasonFirstNotification
	ReasonNewAlertsInGroup
	ReasonNewResolvedAlerts
	ReasonAllAlertsResolved
	ReasonRepeatIntervalElapsed
	ReasonUnknown
)

func (r NotifyReason) shouldNotify() bool {
	return r != ReasonDoNotNotify
}

func (r NotifyReason) String() string {
	switch r {
	case ReasonDoNotNotify:
		return "none"
	case ReasonFirstNotification:
		return "first notification"
	case ReasonNewAlertsInGroup:
		return "new alerts added"
	case ReasonNewResolvedAlerts:
		return "some alerts resolved"
	case ReasonAllAlertsResolved:
		return "all alerts resolved"
	case ReasonRepeatIntervalElapsed:
		return "repeat interval elapsed"
	default:
		return "unknown"
	}
}
