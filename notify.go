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

type dedupingNotifier struct {
	notifies provider.Notifies
	notifier Notifier
}

func newDedupingNotifier(notifies provider.Notifies, n Notifier) *dedupingNotifier {
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
	build func([]*conf.NotificationConfigs) map[string]Notifier
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

func (n *routedNotifier) ApplyConfig(conf *config.Config) bool {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.notifiers = n.build(conf.NotificationConfigs)
	n.notifierOpts = map[string]*config.NotificationConfig{}

	for _, opts := range conf.NotificationConfigs {
		n.notifierOpts[opts.Name] = opts
	}

	return true
}

// mutingNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type mutingNotifier struct {
	types.Muter
	notifier Notifier
}

func (n *mutingNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
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
	name string
}

func (ln *LogNotifier) Notify(ctx context.Context, alerts ...*types.Alert) error {
	log.Infof("notify %q", ln.name)

	for _, a := range alerts {
		log.Infof("- %v", a)
	}
	return nil
}
