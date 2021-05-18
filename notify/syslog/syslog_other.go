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

// Because Syslog only supports unix-based systems, we have to define seperate code for non-unix systems.

// +build windows,plan9

package syslog

import (
	"context"
	"errors"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements an empty Notifier for Syslog notifications.
type Notifier struct{}

// New returns an invalid system error.
func New(c *config.SyslogConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	return nil, errors.New("syslog only supports unix-based systems")
}

// Notify implements an empty Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	return false, errors.New("syslog only supports unix-based systems")
}
