package main

import (
	"fmt"
	"sync"

	"github.com/prometheus/log"
	"golang.org/x/net/context"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

type notifyKey int

const (
	notifyName notifyKey = iota
)

type Notifier interface {
	Notify(context.Context, ...*types.Alert) error
}

// routedNotifier dispatches the alerts to one of a set of
// named notifiers based on the name value provided in the context.
type routedNotifier struct {
	mtx       sync.RWMutex
	notifiers map[string]Notifier
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

	return notifier.Notify(ctx, alerts...)
}

func (n *routedNotifier) ApplyConfig(conf *config.Config) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.notifiers = map[string]Notifier{}
	for _, cn := range conf.NotificationConfigs {
		// TODO(fabxc): create proper notifiers.
		n.notifiers[cn.Name] = &LogNotifier{name: cn.Name}
	}
}

// mutingNotifier wraps a notifier and applies a Silencer
// before sending out an alert.
type mutingNotifier struct {
	Notifier

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

	return n.Notifier.Notify(ctx, filtered...)
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
