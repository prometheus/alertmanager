// Copyright 2015 Prometheus Team
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
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/golang/protobuf/ptypes"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/nflog"
	"github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

var (
	numNotifications = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "alertmanager",
		Name:      "notifications_total",
		Help:      "The total number of attempted notifications.",
	}, []string{"integration"})

	numFailedNotifications = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "alertmanager",
		Name:      "notifications_failed_total",
		Help:      "The total number of failed notifications.",
	}, []string{"integration"})
)

func init() {
	prometheus.Register(numNotifications)
	prometheus.Register(numFailedNotifications)
}

// MinTimeout is the minimum timeout that is set for the context of a call
// to a notification pipeline.
const MinTimeout = 10 * time.Second

// notifyKey defines a custom type with which a context is populated to
// avoid accidental collisions.
type notifyKey int

const (
	keyReceiverName notifyKey = iota
	keyRepeatInterval
	keyGroupLabels
	keyGroupKey
	keyNotificationHash
	keyNow
)

// WithReceiverName populates a context with a receiver name.
func WithReceiverName(ctx context.Context, rcv string) context.Context {
	return context.WithValue(ctx, keyReceiverName, rcv)
}

// WithGroupKey populates a context with a group key.
func WithGroupKey(ctx context.Context, fp model.Fingerprint) context.Context {
	return context.WithValue(ctx, keyGroupKey, fp)
}

// WithNotificationHash populates a context with a notification hash.
func WithNotificationHash(ctx context.Context, hash []byte) context.Context {
	return context.WithValue(ctx, keyNotificationHash, hash)
}

// WithGroupLabels populates a context with grouping labels.
func WithGroupLabels(ctx context.Context, lset model.LabelSet) context.Context {
	return context.WithValue(ctx, keyGroupLabels, lset)
}

// WithNow populates a context with a now timestamp.
func WithNow(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, keyNow, t)
}

// WithRepeatInterval populates a context with a repeat interval.
func WithRepeatInterval(ctx context.Context, t time.Duration) context.Context {
	return context.WithValue(ctx, keyRepeatInterval, t)
}

// RepeatInterval extracts a repeat interval from the context. Iff none exists, the
// second argument is false.
func RepeatInterval(ctx context.Context) (time.Duration, bool) {
	v, ok := ctx.Value(keyRepeatInterval).(time.Duration)
	return v, ok
}

// ReceiverName extracts a receiver name from the context. Iff none exists, the
// second argument is false.
func ReceiverName(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyReceiverName).(string)
	return v, ok
}

func receiverName(ctx context.Context) string {
	recv, ok := ReceiverName(ctx)
	if !ok {
		log.Error("missing receiver")
	}
	return recv
}

// GroupKey extracts a group key from the context. Iff none exists, the
// second argument is false.
func GroupKey(ctx context.Context) (model.Fingerprint, bool) {
	v, ok := ctx.Value(keyGroupKey).(model.Fingerprint)
	return v, ok
}

func groupLabels(ctx context.Context) model.LabelSet {
	groupLabels, ok := GroupLabels(ctx)
	if !ok {
		log.Error("missing group labels")
	}
	return groupLabels
}

// GroupLabels extracts grouping label set from the context. Iff none exists, the
// second argument is false.
func GroupLabels(ctx context.Context) (model.LabelSet, bool) {
	v, ok := ctx.Value(keyGroupLabels).(model.LabelSet)
	return v, ok
}

// Now extracts a now timestamp from the context. Iff none exists, the
// second argument is false.
func Now(ctx context.Context) (time.Time, bool) {
	v, ok := ctx.Value(keyNow).(time.Time)
	return v, ok
}

// NotificationHash extracts a notification hash from the context. Iff none exists,
// the second argument is false.
func NotificationHash(ctx context.Context) ([]byte, bool) {
	v, ok := ctx.Value(keyNotificationHash).([]byte)
	return v, ok
}

// A Stage processes alerts under the constraints of the given context.
type Stage interface {
	Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error)
}

// StageFunc wraps a function to represent a Stage.
type StageFunc func(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error)

// Exec implements Stage interface.
func (f StageFunc) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	return f(ctx, alerts...)
}

// BuildPipeline builds a map of receivers to Stages.
func BuildPipeline(
	confs []*config.Receiver,
	tmpl *template.Template,
	wait func() time.Duration,
	inhibitor *inhibit.Inhibitor,
	silences *silence.Silences,
	notificationLog nflog.Log,
	marker types.Marker,
) RoutingStage {
	rs := RoutingStage{}

	is := NewInhibitStage(inhibitor, marker)
	ss := NewSilenceStage(silences, marker)

	for _, rc := range confs {
		rs[rc.Name] = MultiStage{is, ss, createStage(rc, tmpl, wait, notificationLog)}
	}
	return rs
}

// createStage creates a pipeline of stages for a receiver.
func createStage(rc *config.Receiver, tmpl *template.Template, wait func() time.Duration, notificationLog nflog.Log) Stage {
	var fs FanoutStage
	for _, i := range BuildReceiverIntegrations(rc, tmpl) {
		recv := &nflogpb.Receiver{
			GroupName:   rc.Name,
			Integration: i.name,
			Idx:         uint32(i.idx),
		}
		var s MultiStage
		s = append(s, NewWaitStage(wait))
		s = append(s, NewDedupStage(notificationLog, recv))
		s = append(s, NewRetryStage(i))
		s = append(s, NewSetNotifiesStage(notificationLog, recv))

		fs = append(fs, s)
	}
	return fs
}

// RoutingStage executes the inner stages based on the receiver specified in
// the context.
type RoutingStage map[string]Stage

// Exec implements the Stage interface.
func (rs RoutingStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	receiver, ok := ReceiverName(ctx)
	if !ok {
		return ctx, nil, fmt.Errorf("receiver missing")
	}

	s, ok := rs[receiver]
	if !ok {
		return ctx, nil, fmt.Errorf("stage for receiver missing")
	}

	return s.Exec(ctx, alerts...)
}

// A MultiStage executes a series of stages sequencially.
type MultiStage []Stage

// Exec implements the Stage interface.
func (ms MultiStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	var err error
	for _, s := range ms {
		if len(alerts) == 0 {
			return ctx, nil, nil
		}

		ctx, alerts, err = s.Exec(ctx, alerts...)
		if err != nil {
			return ctx, nil, err
		}
	}
	return ctx, alerts, nil
}

// FanoutStage executes its stages concurrently
type FanoutStage []Stage

// Exec attempts to execute all stages concurrently and discards the results.
// It returns its input alerts and a types.MultiError if one or more stages fail.
func (fs FanoutStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	var (
		wg sync.WaitGroup
		me types.MultiError
	)
	wg.Add(len(fs))

	for _, s := range fs {
		go func(s Stage) {
			if _, _, err := s.Exec(ctx, alerts...); err != nil {
				me.Add(err)
				log.Errorf("Error on notify: %s", err)
			}
			wg.Done()
		}(s)
	}
	wg.Wait()

	if me.Len() > 0 {
		return ctx, alerts, &me
	}
	return ctx, alerts, nil
}

// InhibitStage filters alerts through an inhibition muter.
type InhibitStage struct {
	muter  types.Muter
	marker types.Marker
}

// NewInhibitStage return a new InhibitStage.
func NewInhibitStage(m types.Muter, mk types.Marker) *InhibitStage {
	return &InhibitStage{
		muter:  m,
		marker: mk,
	}
}

// Exec implements the Stage interface.
func (n *InhibitStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	var filtered []*types.Alert
	for _, a := range alerts {
		ok := n.marker.Inhibited(a.Fingerprint())
		// TODO(fabxc): increment total alerts counter.
		// Do not send the alert if the silencer mutes it.
		if !n.muter.Mutes(a.Labels) {
			// TODO(fabxc): increment muted alerts counter.
			filtered = append(filtered, a)
			// Store whether a previously inhibited alert is firing again.
			a.WasInhibited = ok
		}
	}

	return ctx, filtered, nil
}

// SilenceStage filters alerts through a silence muter.
type SilenceStage struct {
	silences *silence.Silences
	marker   types.Marker
}

// NewSilenceStage returns a new SilenceStage.
func NewSilenceStage(s *silence.Silences, mk types.Marker) *SilenceStage {
	return &SilenceStage{
		silences: s,
		marker:   mk,
	}
}

// Exec implements the Stage interface.
func (n *SilenceStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	var filtered []*types.Alert
	for _, a := range alerts {
		_, ok := n.marker.Silenced(a.Fingerprint())
		// TODO(fabxc): increment total alerts counter.
		// Do not send the alert if the silencer mutes it.
		sils, err := n.silences.Query(
			silence.QState(silence.StateActive),
			silence.QMatches(a.Labels),
		)
		if err != nil {
			log.Errorf("Querying silences failed: %s", err)
		}
		if len(sils) == 0 {
			// TODO(fabxc): increment muted alerts counter.
			filtered = append(filtered, a)
			n.marker.SetSilenced(a.Labels.Fingerprint())
			// Store whether a previously silenced alert is firing again.
			a.WasSilenced = ok
		} else {
			n.marker.SetSilenced(a.Labels.Fingerprint(), sils[0].Id)
		}
	}

	return ctx, filtered, nil
}

// WaitStage waits for a certain amount of time before continuing or until the
// context is done.
type WaitStage struct {
	wait func() time.Duration
}

// NewWaitStage returns a new WaitStage.
func NewWaitStage(wait func() time.Duration) *WaitStage {
	return &WaitStage{
		wait: wait,
	}
}

// Exec implements the Stage interface.
func (ws *WaitStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	select {
	case <-time.After(ws.wait()):
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	}
	return ctx, alerts, nil
}

// DedupStage filters alerts.
// Filtering happens based on a notification log.
type DedupStage struct {
	nflog nflog.Log
	recv  *nflogpb.Receiver

	// TODO(fabxc): consider creating an AlertBatch type received
	// by stages that implements these functions.
	// This can then also handle caching so we can skip passing
	// the hash around as a context.
	hash     func([]*types.Alert) []byte
	resolved func([]*types.Alert) bool
	now      func() time.Time
}

// NewDedupStage wraps a DedupStage that runs against the given notification log.
func NewDedupStage(l nflog.Log, recv *nflogpb.Receiver) *DedupStage {
	return &DedupStage{
		nflog:    l,
		recv:     recv,
		hash:     hashAlerts,
		resolved: allAlertsResolved,
		now:      utcNow,
	}
}

func utcNow() time.Time {
	return time.Now().UTC()
}

// TODO(fabxc): this could get slow, but is fine for now. We may want to
// have something mor sophisticated at some point.
// Alternatives are FNV64a as in fingerprints or xxhash.
func hashAlerts(alerts []*types.Alert) []byte {
	// The xor'd sum so we don't have to sort the alerts.
	// XXX(fabxc): this approach caused collision issues with FNV64a in
	// the past. However, sha256 should not suffer from the bit cancelation
	// in in small input changes.
	xsum := [sha256.Size]byte{}

	for _, a := range alerts {
		b := make([]byte, 9)
		binary.BigEndian.PutUint64(b, uint64(a.Fingerprint()))
		// Resolved status is part of the identity.
		if a.Resolved() {
			b[8] = 1
		}
		for i, b := range sha256.Sum256(b) {
			xsum[i] ^= b
		}
	}
	return xsum[:]
}

func allAlertsResolved(alerts []*types.Alert) bool {
	for _, a := range alerts {
		if !a.Resolved() {
			return false
		}
	}
	return true
}

func (n *DedupStage) needsUpdate(entry *nflogpb.Entry, hash []byte, resolved bool, repeat time.Duration) (bool, error) {
	// If we haven't notified about the alert group before, notify right away
	// unless we only have resolved alerts.
	if entry == nil {
		return !resolved, nil
	}
	// Check whether the contents have changed.
	if !bytes.Equal(entry.GroupHash, hash) {
		return true, nil
	}

	// Nothing changed, only notify if the repeat interval has passed.
	ts, err := ptypes.Timestamp(entry.Timestamp)
	if err != nil {
		return false, err
	}
	return ts.Before(n.now().Add(-repeat)), nil
}

// Exec implements the Stage interface.
func (n *DedupStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	// TODO(fabxc): GroupKey will turn into []byte eventually.
	gkey, ok := GroupKey(ctx)
	if !ok {
		return ctx, nil, fmt.Errorf("group key missing")
	}
	gkeyb := make([]byte, 8)
	binary.BigEndian.PutUint64(gkeyb, uint64(gkey))

	repeatInterval, ok := RepeatInterval(ctx)
	if !ok {
		return ctx, nil, fmt.Errorf("repeat interval missing")
	}

	hash := n.hash(alerts)
	resolved := n.resolved(alerts)

	ctx = WithNotificationHash(ctx, hash)

	entries, err := n.nflog.Query(nflog.QGroupKey(gkeyb), nflog.QReceiver(n.recv))

	if err != nil && err != nflog.ErrNotFound {
		return ctx, nil, err
	}
	var entry *nflogpb.Entry
	switch len(entries) {
	case 0:
	case 1:
		entry = entries[0]
	case 2:
		return ctx, nil, fmt.Errorf("Unexpected entry result size %d", len(entries))
	}
	if ok, err := n.needsUpdate(entry, hash, resolved, repeatInterval); err != nil {
		return ctx, nil, err
	} else if ok {
		return ctx, alerts, nil
	}
	return ctx, nil, nil

}

// RetryStage notifies via passed integration with exponential backoff until it
// succeeds. It aborts if the context is canceled or timed out.
type RetryStage struct {
	integration Integration
}

// NewRetryStage returns a new instance of a RetryStage.
func NewRetryStage(i Integration) *RetryStage {
	return &RetryStage{
		integration: i,
	}
}

// Exec implements the Stage interface.
func (r RetryStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	var (
		i    = 0
		b    = backoff.NewExponentialBackOff()
		tick = backoff.NewTicker(b)
		iErr error
	)
	defer tick.Stop()

	for {
		i++
		// Always check the context first to not notify again.
		select {
		case <-ctx.Done():
			if iErr != nil {
				return ctx, nil, iErr
			}

			return ctx, nil, ctx.Err()
		default:
		}

		select {
		case <-tick.C:
			if retry, err := r.integration.Notify(ctx, alerts...); err != nil {
				numFailedNotifications.WithLabelValues(r.integration.name).Inc()
				log.Debugf("Notify attempt %d failed: %s", i, err)
				if !retry {
					return ctx, alerts, fmt.Errorf("Cancelling notify retry due to unrecoverable error: %s", err)
				}

				// Save this error to be able to return the last seen error by an
				// integration upon context timeout.
				iErr = err
			} else {
				numNotifications.WithLabelValues(r.integration.name).Inc()
				return ctx, alerts, nil
			}
		case <-ctx.Done():
			if iErr != nil {
				return ctx, nil, iErr
			}

			return ctx, nil, ctx.Err()
		}
	}
}

// SetNotifiesStage sets the notification information about passed alerts. The
// passed alerts should have already been sent to the receivers.
type SetNotifiesStage struct {
	nflog nflog.Log
	recv  *nflogpb.Receiver

	resolved func([]*types.Alert) bool
}

// NewSetNotifiesStage returns a new instance of a SetNotifiesStage.
func NewSetNotifiesStage(l nflog.Log, recv *nflogpb.Receiver) *SetNotifiesStage {
	return &SetNotifiesStage{
		nflog:    l,
		recv:     recv,
		resolved: allAlertsResolved,
	}
}

// Exec implements the Stage interface.
func (n SetNotifiesStage) Exec(ctx context.Context, alerts ...*types.Alert) (context.Context, []*types.Alert, error) {
	hash, ok := NotificationHash(ctx)
	if !ok {
		return ctx, nil, fmt.Errorf("notification hash missing")
	}
	gkey, ok := GroupKey(ctx)
	if !ok {
		return ctx, nil, fmt.Errorf("group key missing")
	}
	gkeyb := make([]byte, 8)
	binary.BigEndian.PutUint64(gkeyb, uint64(gkey))

	if n.resolved(alerts) {
		return ctx, alerts, n.nflog.LogResolved(n.recv, gkeyb, hash)
	}
	return ctx, alerts, n.nflog.LogActive(n.recv, gkeyb, hash)
}
