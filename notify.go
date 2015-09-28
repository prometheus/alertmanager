package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
)

type notifyKey int

const (
	notifyName notifyKey = iota
	notifyRepeatInterval
	notifySendResolved
)

type Notifier interface {
	Notify(context.Context, ...*types.Alert) error
}

type dedupingNotifier struct {
	notifies provider.Notifies
	notifier Notifier
}

func newDedupingNotifier(notifies provider.Notifies, n Notifier) Notifier {
	return &dedupingNotifier{
		notifies: notifies,
		notifier: n,
	}
}

func (n *dedupingNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	name, ok := ctx.Value(notifyName).(string)
	if !ok {
		return fmt.Errorf("notifier name missing")
	}

	repeatInterval, ok := ctx.Value(notifyRepeatInterval).(time.Duration)
	if !ok {
		return fmt.Errorf("repeat interval missing")
	}

	sendResolved, ok := ctx.Value(notifySendResolved).(bool)
	if !ok {
		return fmt.Errorf("send resolved missing")
	}

	var fps []model.Fingerprint
	for _, a := range alerts {
		fps = append(fps, a.Fingerprint())
	}

	notifies, err := n.notifies.Get(name, fps...)
	if err != nil {
		return err
	}

	now := time.Now()

	var newNotifies []*types.Notify

	var filtered []*types.Alert
	for i, a := range alerts {
		last := notifies[i]

		if last != nil {
			// If the initial alert was not delivered successfully,
			// there is no point in sending a resolved notification.
			if a.Resolved() && (!last.Delivered || !sendResolved) {
				continue
			}

			// Always send if the alert went from resolved to unresolved.
			if last.Resolved && !a.Resolved() {
				// Do not send again if last was delivered unless
				// the repeat interval has already passed.
				if last.Delivered && !now.After(last.Timestamp.Add(repeatInterval)) {
					continue
				}
			}
		}

		filtered = append(filtered, a)

		newNotifies = append(newNotifies, &types.Notify{
			Alert:     a.Fingerprint(),
			SendTo:    name,
			Delivered: true,
			Resolved:  a.Resolved(),
			Timestamp: now,
		})
	}

	if err := n.notifier.Notify(ctx, filtered...); err != nil {
		return err
	}

	return n.notifies.Set(name, newNotifies...)
}

// routedNotifier dispatches the alerts to one of a set of
// named notifiers based on the name value provided in the context.
type routedNotifier struct {
	mtx          sync.RWMutex
	notifiers    map[string]Notifier
	notifierOpts map[string]*config.NotificationConfig

	// build creates a new set of named notifiers based on a config.
	build func(*config.Config) map[string]Notifier
}

func newRoutedNotifier(build func(*config.Config) map[string]Notifier) *routedNotifier {
	return &routedNotifier{build: build}
}

func (n *routedNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	name, ok := ctx.Value(notifyName).(string)
	if !ok {
		return fmt.Errorf("notifier name missing")
	}

	n.mtx.RLock()
	defer n.mtx.RUnlock()

	notifier, ok := n.notifiers[name]
	if !ok {
		return fmt.Errorf("notifier %q does not exist", name)
	}
	opts := n.notifierOpts[name]

	// Populate the context with the the filtering options
	// of the notifier.
	ctx = context.WithValue(ctx, notifyRepeatInterval, time.Duration(opts.RepeatInterval))
	ctx = context.WithValue(ctx, notifySendResolved, opts.SendResolved)

	return notifier.Notify(ctx, alerts...)
}

func (n *routedNotifier) ApplyConfig(conf *config.Config) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.notifiers = n.build(conf)
	n.notifierOpts = map[string]*config.NotificationConfig{}

	for _, opts := range conf.NotificationConfigs {
		n.notifierOpts[opts.Name] = opts
	}
}

// mutingNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type mutingNotifier struct {
	notifier Notifier
	silencer types.Silencer
}

func (n *mutingNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	var filtered []*types.Alert
	for _, a := range alerts {
		// TODO(fabxc): increment total alerts counter.
		// Do not send the alert if the silencer mutes it.
		if !n.silencer.Mutes(a.Labels) {
			// TODO(fabxc): increment muted alerts counter.
			filtered = append(filtered, a)
		}
	}

	return n.notifier.Notify(ctx, filtered...)
}

type LogNotifier struct {
	name string
}

func (ln *LogNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	log.Infof("notify %q", ln.name)

	for _, a := range alerts {
		log.Infof("- %v", a)
	}
	return nil
}
