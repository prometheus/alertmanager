package manager

import (
	"github.com/prometheus/log"
)

type Notifier interface {
	Send(...interface{})
}

type LogNotifier struct {
}

func (*LogNotifier) Send(v ...interface{}) {
	log.Infoln(v...)
}
