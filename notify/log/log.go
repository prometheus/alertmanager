// Copyright 2019 Prometheus Team
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

package log

import (
	"context"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for generic loggers.
type Notifier struct {
	conf   *config.LogConfig
	tmpl   *template.Template
	logger log.Logger
}

// New returns a new Log Notifier.
func New(conf *config.LogConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	var loggerTmp log.Logger

	if conf.Logger == "json" {
		logfile, err := os.OpenFile(*conf.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		loggerTmp = log.With(log.NewJSONLogger(log.NewSyncWriter(logfile)), "ts", log.DefaultTimestampUTC)
	} else if conf.Logger == "logfmt" {
		logfile, err := os.OpenFile(*conf.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		loggerTmp = log.With(log.NewLogfmtLogger(log.NewSyncWriter(logfile)), "ts", log.DefaultTimestampUTC)
	} else {
		loggerTmp = log.With(l)
	}

	return &Notifier{
		conf:   conf,
		tmpl:   t,
		logger: loggerTmp,
	}, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	data := notify.GetTemplateData(ctx, n.tmpl, alerts, n.logger)

	var logger = n.logger
	logger = logWith(data.CommonAnnotations, logger)
	logger = logWith(data.CommonLabels, logger)
	logger = logWith(data.GroupLabels, logger)
	for _, alert := range data.Alerts {
		logger = logWith(alert.Labels, logger)
		logger = logWith(alert.Annotations, logger)

		err := logger.Log("status", alert.Status, "startsAt", alert.StartsAt, "endsAt", alert.EndsAt)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func logWith(values map[string]string, logger log.Logger) log.Logger {
	for k, v := range values {
		logger = log.With(logger, k, v)
	}
	return logger
}
