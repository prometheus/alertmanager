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

// +build !windows,!plan9

package syslog

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

var alert = types.Alert{
	Alert: model.Alert{
		Labels:      model.LabelSet{"test": "test"},
		StartsAt:    time.Now(),
		EndsAt:      time.Now().Add(time.Hour),
		Annotations: model.LabelSet{"test": "test"},
	},
}

func TestSyslogNotify(t *testing.T) {
	tmpl := getTemplate(t)

	c := config.DefaultSyslogConfig
	c.Priority = 7

	notifier := getNotifier(t, c, tmpl)

	format, err := notifier.Notify(context.Background(), &alert)
	require.NoError(t, err)
	require.True(t, format)
}

func TestSyslogMessage(t *testing.T) {
	tmpl := getTemplate(t)

	c := config.DefaultSyslogConfig
	c.Priority = 7

	notifier := getNotifier(t, c, tmpl)

	var (
		err   error
		data  = notify.GetTemplateData(context.Background(), tmpl, []*types.Alert{&alert}, notifier.logger)
		tmplf = notify.TmplText(tmpl, data, &err)
	)
	require.NoError(t, err)

	require.Equal(t, "[FIRING:1 (test) test] Alerts Firing: | Labels: - test = test; Annotations: - test = test; Source: AlertmanagerUrl: http://example.com//#/alerts?receiver= |", tmplf(notifier.conf.Message), "Syslog notification message is different than expected.")
}

var recieved bool
var m = &sync.Mutex{}

func TestSyslogNotifyDial(t *testing.T) {
	tmpl := getTemplate(t)

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	_, p, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	port, err := strconv.Atoi(p)
	require.NoError(t, err)

	c := config.DefaultSyslogConfig
	c.Priority = 7
	c.Daemon = config.SyslogDaemon{
		Network:  "tcp",
		Hostname: "localhost",
		Port:     port,
	}

	notifier := getNotifier(t, c, tmpl)

	go func() {
		conn, err := listener.Accept()
		require.NoError(t, err)
		defer conn.Close()

		m.Lock()
		defer m.Unlock()
		recieved = true
	}()

	format, err := notifier.Notify(context.Background(), &alert)
	require.NoError(t, err)
	require.True(t, format)

	time.Sleep(time.Second)
	m.Lock()
	defer m.Unlock()
	require.True(t, recieved, "Syslog has not connected to the daemon")
}

func getTemplate(t *testing.T) *template.Template {
	tmpl, err := template.FromGlobs(config.DefaultSyslogConfig.Message)
	require.NoError(t, err)

	u, _ := url.Parse("http://example.com/")
	tmpl.ExternalURL = u

	return tmpl
}

func getNotifier(t *testing.T, c config.SyslogConfig, tmpl *template.Template) *Notifier {
	notifier, err := New(&c, tmpl, log.NewNopLogger())
	require.NoError(t, err)

	return notifier
}
