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

// The minimum timeout that is set for the context of a call
// to a notification pipeline.
const MinTimeout = 10 * time.Second

type notifyKey int

const (
	keyDestination notifyKey = iota
	keyRepeatInterval
	keySendResolved
	keyGroupLabels
	keyGroupKey
	keyNow
)

func WithDestination(ctx context.Context, dest string) context.Context {
	return context.WithValue(ctx, keyDestination, dest)
}

func WithRepeatInterval(ctx context.Context, t time.Duration) context.Context {
	return context.WithValue(ctx, keyRepeatInterval, t)
}

func WithSendResolved(ctx context.Context, b bool) context.Context {
	return context.WithValue(ctx, keySendResolved, b)
}

func WithGroupKey(ctx context.Context, fp model.Fingerprint) context.Context {
	return context.WithValue(ctx, keyGroupKey, fp)
}

func WithGroupLabels(ctx context.Context, lset model.LabelSet) context.Context {
	return context.WithValue(ctx, keyGroupLabels, lset)
}

func WithNow(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, keyNow, t)
}

func Destination(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyDestination).(string)
	return v, ok
}

func RepeatInterval(ctx context.Context) (time.Duration, bool) {
	v, ok := ctx.Value(keyRepeatInterval).(time.Duration)
	return v, ok
}

func SendResolved(ctx context.Context) (bool, bool) {
	v, ok := ctx.Value(keySendResolved).(bool)
	return v, ok
}

func GroupKey(ctx context.Context) (model.Fingerprint, bool) {
	v, ok := ctx.Value(keyGroupKey).(model.Fingerprint)
	return v, ok
}

func GroupLabels(ctx context.Context) (model.LabelSet, bool) {
	v, ok := ctx.Value(keyGroupLabels).(model.LabelSet)
	return v, ok
}

func Now(ctx context.Context) (time.Time, bool) {
	v, ok := ctx.Value(keyNow).(time.Time)
	return v, ok
}

type Notifier interface {
	Notify(context.Context, ...*types.Alert) error
}

// Notifiers fans out notifications to all notifiers it holds
// at once.
type Fanout map[string]Notifier

func (ns Fanout) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var (
		wg sync.WaitGroup
		me types.MultiError
	)
	wg.Add(len(ns))

	dest, ok := Destination(ctx)
	if !ok {
		return fmt.Errorf("destination missing")
	}

	for suffix, n := range ns {
		// Suffix the destination with the unique key for the fanout.
		foCtx := WithDestination(ctx, fmt.Sprintf("%s/%s", dest, suffix))

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

type RetryNotifier struct {
	notifier Notifier
}

func Retry(n Notifier) *RetryNotifier {
	return &RetryNotifier{notifier: n}
}

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

type DedupingNotifier struct {
	notifies provider.Notifies
	notifier Notifier
}

func Dedup(notifies provider.Notifies, n Notifier) *DedupingNotifier {
	return &DedupingNotifier{notifies: notifies, notifier: n}
}

func (n *DedupingNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	name, ok := Destination(ctx)
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
			SendTo:    name,
			Resolved:  a.Resolved(),
			Timestamp: now,
		})
	}

	if err := n.notifier.Notify(ctx, filtered...); err != nil {
		return err
	}

	return n.notifies.Set(newNotifies...)
}

// RoutedNotifier dispatches the alerts to one of a set of
// named notifiers based on the name value provided in the context.
type Router map[string]Notifier

func (rs Router) Notify(ctx context.Context, alerts ...*types.Alert) error {
	dest, ok := Destination(ctx)
	if !ok {
		return fmt.Errorf("notifier name missing")
	}

	notifier, ok := rs[dest]
	if !ok {
		return fmt.Errorf("notifier %q does not exist", dest)
	}

	return notifier.Notify(ctx, alerts...)
}

// MutingNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type MutingNotifier struct {
	types.Muter
	notifier Notifier
}

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

type LogNotifier struct {
	log      log.Logger
	notifier Notifier
}

func Log(n Notifier, log log.Logger) *LogNotifier {
	return &LogNotifier{log: log, notifier: n}
}

func (n *LogNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	n.log.Debugf("notify %v", alerts)

	if n.notifier != nil {
		return n.notifier.Notify(ctx, alerts...)
	}
	return nil
}
