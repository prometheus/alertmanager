package manager

import (
	"github.com/prometheus/log"
)

type Notifier interface {
	Name() string
	Send(...interface{})
}

type LogNotifier struct {
}

func (*LogNotifier) Name() string {
	return "default"
}

func (*LogNotifier) Send(v ...interface{}) {
	log.Infoln(v...)
}
