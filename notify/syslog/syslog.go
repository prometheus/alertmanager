// Copyright 2021 Prometheus Team
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

// +build !windows,!plan9

package syslog

import (
	"context"
	"log/syslog"
	"net/url"

	"github.com/pkg/errors"

	"github.com/go-kit/kit/log"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for Syslog notifications.
type Notifier struct {
	conf   *config.SyslogConfig
	tmpl   *template.Template
	logger log.Logger
	writer *syslog.Writer
}

// New returns a new Syslog notification handler.
func New(c *config.SyslogConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	var (
		w   *syslog.Writer
		err error
	)

	if c.Daemon != "" {
		var u *url.URL
		u, err = url.Parse(c.Daemon)
		if err != nil {
			return nil, errors.Errorf("error parsing url: %v", err)
		}
		w, err = syslog.Dial(u.Scheme, u.Host, syslog.Priority(c.Priority), c.Tag)
	} else {
		w, err = syslog.New(syslog.Priority(c.Priority), c.Tag)
	}
	if err != nil {
		return nil, errors.Errorf("error initializing syslog %v", err)
	}

	return &Notifier{conf: c, tmpl: t, logger: l, writer: w}, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var (
		err  error
		data = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmpl = notify.TmplText(n.tmpl, data, &err)
	)
	if err != nil {
		return false, err
	}

	_, err = n.writer.Write([]byte(tmpl(n.conf.Message)))
	if err != nil {
		return true, errors.Errorf("error writing to syslog: %v", err)
	}

	return true, nil
}
