package manager

import (
	"github.com/prometheus/log"
)

type Notifier interface {
	Name() string
	Send(...*Alert) error
}

type LogNotifier struct {
	name string
}

func NewLogNotifier(name string) Notifier {
	return &LogNotifier{name}
}

func (ln *LogNotifier) Name() string {
	return ln.name
}

func (ln *LogNotifier) Send(alerts ...*Alert) error {
	log.Infof("notify %q", ln.name)

	for _, a := range alerts {
		log.Infof("  - %v", a)
	}
	return nil
}
