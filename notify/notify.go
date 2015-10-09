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
	keyGroup
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

func WithGroup(ctx context.Context, g string) context.Context {
	return context.WithValue(ctx, keyGroup, g)
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

func Group(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyGroup).(string)
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
type Notifiers []Notifier

func (ns Notifiers) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var wg sync.WaitGroup
	wg.Add(len(ns))

	for _, n := range ns {
		go func(n Notifier) {
			err := n.Notify(ctx, alerts...)
			if err != nil {
				log.Errorf("Error on notify: %s", err)
			}
			wg.Done()
		}(n)
	}

	wg.Wait()

	return nil
}

type RetryNotifier struct {
	Notifier Notifier
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
			if err := n.Notifier.Notify(ctx, alerts...); err != nil {
				log.Warnf("Notify attempt %d failed: %s", i, err)
			} else {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

type DedupingNotifier struct {
	notifies provider.Notifies
	notifier Notifier
}

func NewDedupingNotifier(notifies provider.Notifies, n Notifier) *DedupingNotifier {
	return &DedupingNotifier{
		notifies: notifies,
		notifier: n,
	}
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
				if !sendResolved {
					continue
				}
				// If the initial alert was not delivered successfully,
				// there is no point in sending a resolved notification.
				if !last.Resolved && !last.Delivered {
					continue
				}
				if last.Resolved && last.Delivered {
					continue
				}
			} else if !last.Resolved {
				// Do not send again if last was delivered unless
				// the repeat interval has already passed.
				if last.Delivered {
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
			}
		} else if a.Resolved() && !sendResolved {
			continue
		}

		filtered = append(filtered, a)
	}

	// As we are resending an alert anyway, resend all of them even if their
	// repeat interval has not yet passed.
	if doResend {
		filtered = append(filtered, resendQueue...)
	}

	var newNotifies []*types.Notify

	for _, a := range filtered {
		newNotifies = append(newNotifies, &types.Notify{
			Alert:     a.Fingerprint(),
			SendTo:    name,
			Delivered: true,
			Resolved:  a.Resolved(),
			Timestamp: now,
		})
	}

	// The deduping notifier is the last one before actually sending notifications.
	// Thus, this is the place where we abort if after all filtering, nothing is left.
	if len(filtered) == 0 {
		return nil
	}

	if err := n.notifier.Notify(ctx, filtered...); err != nil {
		return err
	}

	return n.notifies.Set(newNotifies...)
}

// RoutedNotifier dispatches the alerts to one of a set of
// named notifiers based on the name value provided in the context.
type RoutedNotifier struct {
	mtx       sync.RWMutex
	notifiers map[string]Notifier

	// build creates a new set of named notifiers based on a config.
	build func([]*config.NotificationConfig) map[string]Notifier
}

func NewRoutedNotifier(build func([]*config.NotificationConfig) map[string]Notifier) *RoutedNotifier {
	return &RoutedNotifier{build: build}
}

func (n *RoutedNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	name, ok := Destination(ctx)
	if !ok {
		return fmt.Errorf("notifier name missing")
	}

	n.mtx.RLock()
	defer n.mtx.RUnlock()

	notifier, ok := n.notifiers[name]
	if !ok {
		return fmt.Errorf("notifier %q does not exist", name)
	}

	return notifier.Notify(ctx, alerts...)
}

func (n *RoutedNotifier) ApplyConfig(conf *config.Config) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.notifiers = n.build(conf.NotificationConfigs)
}

// MutingNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type MutingNotifier struct {
	types.Muter
	Notifier Notifier
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

	return n.Notifier.Notify(ctx, filtered...)
}

type LogNotifier struct {
	Log      log.Logger
	Notifier Notifier
}

func (ln *LogNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	ln.Log.Debugf("notify %v", alerts)

	if ln.Notifier != nil {
		return ln.Notifier.Notify(ctx, alerts...)
	}
	return nil
}
