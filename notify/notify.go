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

type Stage interface {
	Exec(ctx context.Context, alerts ...*types.Alert) ([]*types.Alert, error)
}

type MultiStage []Stage

func (ms MultiStage) Exec(ctx context.Context, alerts ...*types.Alert) ([]*types.Alert, error) {
	var err error
	for _, s := range ms {
		alerts, err = s.Exec(ctx, alerts...)
		if err != nil {
			return nil, err
		}
	}
	return alerts, nil
}

type FanoutStage map[string]Stage

func createStage(receiverNotifiers map[string]Notifier, receiverName string, meshWait func() time.Duration, inhibitor *inhibit.Inhibitor, silences *meshprov.Silences, ni *meshprov.NotificationInfos, marker types.Marker) Stage {
	var ms MultiStage
	ms = append(ms, NewLogStage(log.With("step", "inhibit")))
	ms = append(ms, NewInhibitStage(inhibitor, marker))
	ms = append(ms, NewLogStage(log.With("step", "silence")))
	ms = append(ms, NewSilenceStage(silences, marker))

	var fs = make(FanoutStage)
	for integrationId, integration := range receiverNotifiers {
		var s MultiStage
		s = append(s, NewLogStage(log.With("step", "wait")))
		s = append(s, NewWaitStage(meshWait))
		s = append(s, NewLogStage(log.With("step", "integration")))
		s = append(s, NewIntegrationStage(integration, ni))
		fs[integrationId] = s
	}

	return append(ms, fs)
}

func NewPipeline(rcvs []*config.Receiver, tmpl *template.Template, meshWait func() time.Duration, inhibitor *inhibit.Inhibitor, silences *meshprov.Silences, ni *meshprov.NotificationInfos, marker types.Marker) *Pipeline {
	receiverConfigs := BuildReceiverTree(rcvs, tmpl)
	receiverPipelines := make(map[string]Stage)

	for receiver, notifiers := range receiverConfigs {
		receiverPipelines[receiver] = createStage(notifiers, receiver, meshWait, inhibitor, silences, ni, marker)
	}

	return &Pipeline{receiverPipelines: receiverPipelines}
}

type Pipeline struct {
	receiverPipelines map[string]Stage
}

func (p *Pipeline) Notify(ctx context.Context, alerts ...*types.Alert) error {
	r, ok := Receiver(ctx)
	if !ok {
		return fmt.Errorf("receiver missing")
	}

	_, err := p.receiverPipelines[r].Exec(ctx, alerts...)
	return err
}

type IntegrationStage struct {
	notificationInfos *meshprov.NotificationInfos
	notifier          Notifier
}

func NewIntegrationStage(n Notifier, ni *meshprov.NotificationInfos) *IntegrationStage {
	return &IntegrationStage{
		notificationInfos: ni,
		notifier:          n,
	}
}

func (i IntegrationStage) Exec(ctx context.Context, alerts ...*types.Alert) ([]*types.Alert, error) {
	newNotifies, err := Dedup(i.notificationInfos).ExtractNewNotifies(ctx, alerts...)
	if err != nil {
		return nil, err
	}

	if newNotifies != nil {
		err = Retry(i.notifier, ctx, alerts...)
		if err != nil {
			return nil, err
		}
	}

	return nil, i.notificationInfos.Set(newNotifies...)
}

// Exec attempts to notify all Notifiers concurrently. It returns a types.MultiError
// if any of them fails.
func (fs FanoutStage) Exec(ctx context.Context, alerts ...*types.Alert) ([]*types.Alert, error) {
	var (
		wg sync.WaitGroup
		me types.MultiError
	)
	wg.Add(len(fs))

	receiver, ok := Receiver(ctx)
	if !ok {
		return nil, fmt.Errorf("receiver missing")
	}

	for suffix, s := range fs {
		// Suffix the receiver with the unique key for the fanout.
		foCtx := WithReceiver(ctx, fmt.Sprintf("%s/%s", receiver, suffix))

		go func(s Stage) {
			_, err := s.Exec(foCtx, alerts...)
			if err != nil {
				me.Add(err)
				log.Errorf("Error on notify: %s", err)
			}
			wg.Done()
		}(s)
	}

	wg.Wait()

	if me.Len() > 0 {
		return nil, &me
	}

	return nil, nil
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
func (ws *WaitStage) Exec(ctx context.Context, alerts ...*types.Alert) ([]*types.Alert, error) {
	select {
	case <-time.After(ws.wait()):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return alerts, nil
}

// SilenceStage filters alerts through a silence muter.
type SilenceStage struct {
	muter  types.Muter
	marker types.Marker
}

// NewSilenceStage returns a new SilenceStage.
func NewSilenceStage(m types.Muter, mk types.Marker) *SilenceStage {
	return &SilenceStage{
		muter:  m,
		marker: mk,
	}
}

// Exec implements the Stage interface.
func (n *SilenceStage) Exec(ctx context.Context, alerts ...*types.Alert) ([]*types.Alert, error) {
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
func (n *InhibitStage) Exec(ctx context.Context, alerts ...*types.Alert) ([]*types.Alert, error) {
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

type LogStage struct {
	log log.Logger
}

func NewLogStage(log log.Logger) *LogStage {
	return &LogStage{log: log}
}

func (l *LogStage) Exec(ctx context.Context, alerts ...*types.Alert) ([]*types.Alert, error) {
	l.log.Debugf("notify %v", alerts)

	return alerts, nil
}
