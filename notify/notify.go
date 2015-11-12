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

	"github.com/prometheus/alertmanager/provider"
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
	keySendResolved
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

// WithSendResolved populates a context with a send resolved boolean.
func WithSendResolved(ctx context.Context, b bool) context.Context {
	return context.WithValue(ctx, keySendResolved, b)
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

// SendResolved extracts a send resolved boolean from the context.
// Iff none exists, the second argument is false.
func SendResolved(ctx context.Context) (bool, bool) {
	v, ok := ctx.Value(keySendResolved).(bool)
	return v, ok
}

// GroupKey extracts a group key from the context. Iff none exists, the
// second argument is false.
func GroupKey(ctx context.Context) (model.Fingerprint, bool) {
	v, ok := ctx.Value(keyGroupKey).(model.Fingerprint)
	return v, ok
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
				me = append(me, err)
				log.Errorf("Error on notify: %s", err)
			}
			wg.Done()
		}(n)
	}

	wg.Wait()

	if len(me) > 0 {
		return me
	}
	return nil
}

// RetryNotifier accepts another notifier and retries notifying
// on error with exponential backoff.
type RetryNotifier struct {
	notifier Notifier
}

// Retry wraps the given notifier in a RetryNotifier.
func Retry(n Notifier) *RetryNotifier {
	return &RetryNotifier{notifier: n}
}

// Notify calls the underlying notifier with exponential backoff until it succeeds.
// It aborts if the context is canceled or timed out.
func (n *RetryNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var (
		i    = 0
		b    = backoff.NewExponentialBackOff()
		tick = backoff.NewTicker(b)
	)
	defer tick.Stop()

	for {
		i++

		select {
		case <-tick.C:
			if err := n.notifier.Notify(ctx, alerts...); err != nil {
				log.Warnf("Notify attempt %d failed: %s", i, err)
			} else {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// DedupingNotifier filters and forwards alerts to another notifier.
// Filtering happens based on a provider of NotifyInfos.
// On successful notification new NotifyInfos are set.
type DedupingNotifier struct {
	notifies provider.Notifies
	notifier Notifier
}

// Dedup wraps a Notifier in a DedupingNotifier that runs against the given NotifyInfo provider.
func Dedup(notifies provider.Notifies, n Notifier) *DedupingNotifier {
	return &DedupingNotifier{notifies: notifies, notifier: n}
}

// Notify implements the Notifier interface.
func (n *DedupingNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	name, ok := Receiver(ctx)
	if !ok {
		return fmt.Errorf("notifier name missing")
	}

	repeatInterval, ok := RepeatInterval(ctx)
	if !ok {
		return fmt.Errorf("repeat interval missing")
	}

	sendResolved, ok := SendResolved(ctx)
	if !ok {
		return fmt.Errorf("send resolved missing")
	}

	now, ok := Now(ctx)
	if !ok {
		return fmt.Errorf("now time missing")
	}

	var fps []model.Fingerprint
	for _, a := range alerts {
		fps = append(fps, a.Fingerprint())
	}

	notifies, err := n.notifies.Get(name, fps...)
	if err != nil {
		return err
	}

	var (
		doResend    bool
		resendQueue []*types.Alert
		filtered    []*types.Alert
	)
	for i, a := range alerts {
		last := notifies[i]

		if last != nil {
			if a.Resolved() {
				if !sendResolved || last.Resolved {
					continue
				}
			} else if !last.Resolved {
				// Do not send again if last was delivered unless
				// the repeat interval has already passed.
				if !now.After(last.Timestamp.Add(repeatInterval)) {
					// To not repeat initial batch fragmentation after the repeat interval
					// has passed, store them and send them anyway if on of the other
					// alerts has already passed the repeat interval.
					// This way we unify batches again.
					resendQueue = append(resendQueue, a)

					continue
				} else {
					doResend = true
				}
			}
		} else if a.Resolved() {
			// If the alert is resolved but we never notified about it firing,
			// there is nothing to do.
			continue
		}

		filtered = append(filtered, a)
	}

	// As we are resending an alert anyway, resend all of them even if their
	// repeat interval has not yet passed.
	if doResend {
		filtered = append(filtered, resendQueue...)
	}

	// The deduping notifier is the last one before actually sending notifications.
	// Thus, this is the place where we abort if after all filtering, nothing is left.
	if len(filtered) == 0 {
		return nil
	}

	var newNotifies []*types.NotifyInfo

	for _, a := range filtered {
		newNotifies = append(newNotifies, &types.NotifyInfo{
			Alert:     a.Fingerprint(),
			Receiver:  name,
			Resolved:  a.Resolved(),
			Timestamp: now,
		})
	}

	if err := n.notifier.Notify(ctx, filtered...); err != nil {
		return err
	}

	return n.notifies.Set(newNotifies...)
}

// Router dispatches the alerts to one of a set of
// named notifiers based on the name value provided in the context.
type Router map[string]Notifier

// Notify implements the Notifier interface.
func (rs Router) Notify(ctx context.Context, alerts ...*types.Alert) error {
	receiver, ok := Receiver(ctx)
	if !ok {
		return fmt.Errorf("notifier name missing")
	}

	notifier, ok := rs[receiver]
	if !ok {
		return fmt.Errorf("notifier %q does not exist", receiver)
	}

	return notifier.Notify(ctx, alerts...)
}

// MutingNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type MutingNotifier struct {
	types.Muter
	notifier Notifier
}

// Mute wraps a notifier in a MutingNotifier with the given Muter.
func Mute(m types.Muter, n Notifier) *MutingNotifier {
	return &MutingNotifier{Muter: m, notifier: n}
}

func (n *MutingNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var filtered []*types.Alert
	for _, a := range alerts {
		// TODO(fabxc): increment total alerts counter.
		// Do not send the alert if the silencer mutes it.
		if !n.Mutes(a.Labels) {
			// TODO(fabxc): increment muted alerts counter.
			filtered = append(filtered, a)
		}
	}

	return n.notifier.Notify(ctx, filtered...)
}

// LogNotifier logs the alerts to be notified about. It forwards to another Notifier
// afterwards, if any is provided.
type LogNotifier struct {
	log      log.Logger
	notifier Notifier
}

// Log wraps a Notifier in a LogNotifier with the given Logger.
func Log(n Notifier, log log.Logger) *LogNotifier {
	return &LogNotifier{log: log, notifier: n}
}

// Notify implements the Notifier interface.
func (n *LogNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	n.log.Debugf("notify %v", alerts)

	if n.notifier != nil {
		return n.notifier.Notify(ctx, alerts...)
	}
	return nil
}
