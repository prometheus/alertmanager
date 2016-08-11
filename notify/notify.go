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
	"fmt"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/provider"
	meshprov "github.com/prometheus/alertmanager/provider/mesh"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// MinTimeout is the minimum timeout that is set for the context of a call
// to a notification pipeline.
const MinTimeout = 10 * time.Second

// notifyKey defines a custom type with which a context is populated to
// avoid accidental collisions.
type notifyKey int

const (
	keyReceiver notifyKey = iota
	keyRepeatInterval
	keyGroupLabels
	keyGroupKey
	keyNow
)

// WithReceiver populates a context with a receiver.
func WithReceiver(ctx context.Context, rcv string) context.Context {
	return context.WithValue(ctx, keyReceiver, rcv)
}

// WithRepeatInterval populates a context with a repeat interval.
func WithRepeatInterval(ctx context.Context, t time.Duration) context.Context {
	return context.WithValue(ctx, keyRepeatInterval, t)
}

// WithGroupKey populates a context with a group key.
func WithGroupKey(ctx context.Context, fp model.Fingerprint) context.Context {
	return context.WithValue(ctx, keyGroupKey, fp)
}

// WithGroupLabels populates a context with grouping labels.
func WithGroupLabels(ctx context.Context, lset model.LabelSet) context.Context {
	return context.WithValue(ctx, keyGroupLabels, lset)
}

// WithNow populates a context with a now timestamp.
func WithNow(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, keyNow, t)
}

func receiver(ctx context.Context) string {
	recv, ok := Receiver(ctx)
	if !ok {
		log.Error("missing receiver")
	}
	return recv
}

// Receiver extracts a receiver from the context. Iff none exists, the
// second argument is false.
func Receiver(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyReceiver).(string)
	return v, ok
}

// RepeatInterval extracts a repeat interval from the context. Iff none exists, the
// second argument is false.
func RepeatInterval(ctx context.Context) (time.Duration, bool) {
	v, ok := ctx.Value(keyRepeatInterval).(time.Duration)
	return v, ok
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

// A Notifier is a type which notifies about alerts under constraints of the
// given context.
type Notifier interface {
	Notify(context.Context, ...*types.Alert) error
}

type NotificationFilter interface {
	Filter(alerts ...*types.Alert) ([]*types.Alert, error)
}

func NewPipeline(rcvs []*config.Receiver, tmpl *template.Template, meshWait func() time.Duration, inhibitor *inhibit.Inhibitor, marker types.Marker, silences *meshprov.Silences, ni *meshprov.NotificationInfos) *Pipeline {
	return &Pipeline{
		inhibitor: inhibitor,
		silences:  silences,
		marker:    marker,
		router:    BuildRouter(rcvs, tmpl, meshWait, ni),
	}
}

type Pipeline struct {
	inhibitor         *inhibit.Inhibitor
	silences          *meshprov.Silences
	notificationInfos *meshprov.NotificationInfos
	marker            types.Marker
	router            Router
}

func (p *Pipeline) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var err error
	alerts, err = Inhibit(p.inhibitor, p.marker).Filter(alerts...)
	if err != nil {
		return err
	}

	alerts, err = Silence(p.silences, p.marker).Filter(alerts...)
	if err != nil {
		return err
	}

	return p.router.Notify(ctx, alerts...)
}

type FanoutPipeline struct {
	notificationInfos *meshprov.NotificationInfos
	meshWait          func() time.Duration
	notifier          Notifier
}

func (fp FanoutPipeline) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var err error
	err = Wait(ctx, fp.meshWait)
	if err != nil {
		return err
	}

	newNotifies, err := Dedup(fp.notificationInfos).ExtractNewNotifies(ctx, alerts...)
	if err != nil {
		return err
	}

	if newNotifies != nil {
		err = Retry(fp.notifier, ctx, alerts...)
		if err != nil {
			return err
		}
	}

	return fp.notificationInfos.Set(newNotifies...)
}

// Fanout sends notifications through all notifiers it holds at once.
type Fanout map[string]Notifier

// Notify attempts to notify all Notifiers concurrently. It returns a types.MultiError
// if any of them fails.
func (ns Fanout) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var (
		wg sync.WaitGroup
		me types.MultiError
	)
	wg.Add(len(ns))

	receiver, ok := Receiver(ctx)
	if !ok {
		return fmt.Errorf("receiver missing")
	}

	for suffix, n := range ns {
		// Suffix the receiver with the unique key for the fanout.
		foCtx := WithReceiver(ctx, fmt.Sprintf("%s/%s", receiver, suffix))

		go func(n Notifier) {
			if err := n.Notify(foCtx, alerts...); err != nil {
				me.Add(err)
				log.Errorf("Error on notify: %s", err)
			}
			wg.Done()
		}(n)
	}

	wg.Wait()

	if me.Len() > 0 {
		return &me
	}
	return nil
}

// Retry calls the passed notifier with exponential backoff until it succeeds.
// It aborts if the context is canceled or timed out.
func Retry(n Notifier, ctx context.Context, alerts ...*types.Alert) error {
	var (
		i    = 0
		b    = backoff.NewExponentialBackOff()
		tick = backoff.NewTicker(b)
	)
	defer tick.Stop()

	for {
		i++
		// Always check the context first to not notify again.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		select {
		case <-tick.C:
			if err := n.Notify(ctx, alerts...); err != nil {
				log.Warnf("Notify attempt %d failed: %s", i, err)
			} else {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Deduplicator filters alerts.
// Filtering happens based on a provider of NotifyInfos.
type Deduplicator struct {
	notifies provider.Notifies
}

// Dedup wraps a Deduplicator that runs against the given NotifyInfo provider.
func Dedup(notifies provider.Notifies) *Deduplicator {
	return &Deduplicator{notifies: notifies}
}

// hasUpdates checks an alert against the last notification that was made
// about it.
func (n *Deduplicator) hasUpdate(alert *types.Alert, last *types.NotificationInfo, now time.Time, interval time.Duration) bool {
	if last != nil {
		if alert.Resolved() {
			if last.Resolved {
				return false
			}
		} else if !last.Resolved {
			// Do not send again if last was delivered unless
			// the repeat interval has already passed.
			if !now.After(last.Timestamp.Add(interval)) {
				return false
			}
		}
	} else if alert.Resolved() {
		// If the alert is resolved but we never notified about it firing,
		// there is nothing to do.
		return false
	}
	return true
}

// ExtractNewNotifies filters out the notifications that shall be sent
func (n *Deduplicator) ExtractNewNotifies(ctx context.Context, alerts ...*types.Alert) ([]*types.NotificationInfo, error) {
	name, ok := Receiver(ctx)
	if !ok {
		return nil, fmt.Errorf("notifier name missing")
	}

	repeatInterval, ok := RepeatInterval(ctx)
	if !ok {
		return nil, fmt.Errorf("repeat interval missing")
	}

	now, ok := Now(ctx)
	if !ok {
		return nil, fmt.Errorf("now time missing")
	}

	var fps []model.Fingerprint
	for _, a := range alerts {
		fps = append(fps, a.Fingerprint())
	}

	notifyInfo, err := n.notifies.Get(name, fps...)
	if err != nil {
		return nil, err
	}

	// If we have to notify about any of the alerts, we send a notification
	// for the entire batch.
	var send bool
	for i, alert := range alerts {
		if n.hasUpdate(alert, notifyInfo[i], now, repeatInterval) {
			send = true
			break
		}
	}
	if !send {
		return nil, nil
	}

	var newNotifies []*types.NotificationInfo

	for _, a := range alerts {
		newNotifies = append(newNotifies, &types.NotificationInfo{
			Alert:     a.Fingerprint(),
			Receiver:  name,
			Resolved:  a.Resolved(),
			Timestamp: now,
		})
	}

	return newNotifies, nil
}

func Wait(ctx context.Context, wait func() time.Duration) error {
	select {
	case <-time.After(wait()):
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// Router dispatches the alerts to one of a set of
// named fanouts based on the name value provided in the context.
type Router map[string]Notifier

// Notify dispatches the alerts to the fanout specified in the context.
func (rs Router) Notify(ctx context.Context, alerts ...*types.Alert) error {
	receiver, ok := Receiver(ctx)
	if !ok {
		return fmt.Errorf("notifier name missing")
	}

	n, ok := rs[receiver]
	if !ok {
		return fmt.Errorf("notifier %q does not exist", receiver)
	}

	return n.Notify(ctx, alerts...)
}

// SilenceNotifier filters alerts through a silence muter.
type SilenceNotifier struct {
	muter  types.Muter
	marker types.Marker
}

// Silence returns a new SilenceNotifier.
func Silence(m types.Muter, mk types.Marker) *SilenceNotifier {
	return &SilenceNotifier{
		muter:  m,
		marker: mk,
	}
}

// Filter implements the NotificationFilter interface.
func (n *SilenceNotifier) Filter(alerts ...*types.Alert) ([]*types.Alert, error) {
	var filtered []*types.Alert
	for _, a := range alerts {
		_, ok := n.marker.Silenced(a.Fingerprint())
		// TODO(fabxc): increment total alerts counter.
		// Do not send the alert if the silencer mutes it.
		if !n.muter.Mutes(a.Labels) {
			// TODO(fabxc): increment muted alerts counter.
			filtered = append(filtered, a)
			// Store whether a previously silenced alert is firing again.
			a.WasSilenced = ok
		}
	}

	return filtered, nil
}

// InhibitNotifier filters alerts through an inhibition muter.
type InhibitNotifier struct {
	muter  types.Muter
	marker types.Marker
}

// Inhibit return a new InhibitNotifier.
func Inhibit(m types.Muter, mk types.Marker) *InhibitNotifier {
	return &InhibitNotifier{
		muter:  m,
		marker: mk,
	}
}

// Filter implements the NotificationFilter interface.
func (n *InhibitNotifier) Filter(alerts ...*types.Alert) ([]*types.Alert, error) {
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

	return filtered, nil
}
