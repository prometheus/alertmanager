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

package main

import (
	"fmt"
	"testing"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
)

type sendResolved bool

func (s sendResolved) SendResolved() bool { return bool(s) }

func TestBuildReceiverIntegrations(t *testing.T) {
	for _, tc := range []struct {
		receiver *config.Receiver
		err      bool
		exp      []notify.Integration
	}{
		{
			receiver: &config.Receiver{
				Name: "foo",
				WebhookConfigs: []*config.WebhookConfig{
					&config.WebhookConfig{
						HTTPConfig: &commoncfg.HTTPClientConfig{},
					},
					&config.WebhookConfig{
						HTTPConfig: &commoncfg.HTTPClientConfig{},
						NotifierConfig: config.NotifierConfig{
							VSendResolved: true,
						},
					},
				},
			},
			exp: []notify.Integration{
				notify.NewIntegration(nil, sendResolved(false), "webhook", 0),
				notify.NewIntegration(nil, sendResolved(true), "webhook", 1),
			},
		},
		{
			receiver: &config.Receiver{
				Name: "foo",
				WebhookConfigs: []*config.WebhookConfig{
					&config.WebhookConfig{
						HTTPConfig: &commoncfg.HTTPClientConfig{
							TLSConfig: commoncfg.TLSConfig{
								CAFile: "not_existing",
							},
						},
					},
				},
			},
			err: true,
		},
	} {
		tc := tc
		t.Run("", func(t *testing.T) {
			integrations, err := buildReceiverIntegrations(tc.receiver, nil, nil)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, integrations, len(tc.exp))
			for i := range tc.exp {
				require.Equal(t, tc.exp[i].SendResolved(), integrations[i].SendResolved())
				require.Equal(t, tc.exp[i].Name(), integrations[i].Name())
				require.Equal(t, tc.exp[i].Index(), integrations[i].Index())
			}
		})
	}
}

func TestExternalURL(t *testing.T) {
	hostname := "foo"
	for _, tc := range []struct {
		hostnameResolver func() (string, error)
		external         string
		listen           string

		expURL string
		err    bool
	}{
		{
			listen: ":9093",
			expURL: "http://" + hostname + ":9093",
		},
		{
			listen: "localhost:9093",
			expURL: "http://" + hostname + ":9093",
		},
		{
			listen: "localhost:",
			expURL: "http://" + hostname + ":",
		},
		{
			external: "https://host.example.com",
			expURL:   "https://host.example.com",
		},
		{
			external: "https://host.example.com/",
			expURL:   "https://host.example.com",
		},
		{
			external: "http://host.example.com/alertmanager",
			expURL:   "http://host.example.com/alertmanager",
		},
		{
			external: "http://host.example.com/alertmanager/",
			expURL:   "http://host.example.com/alertmanager",
		},
		{
			external: "http://host.example.com/////alertmanager//",
			expURL:   "http://host.example.com/////alertmanager",
		},
		{
			err: true,
		},
		{
			hostnameResolver: func() (string, error) { return "", fmt.Errorf("some error") },
			err:              true,
		},
		{
			external: "://broken url string",
			err:      true,
		},
		{
			external: "host.example.com:8080",
			err:      true,
		},
	} {
		tc := tc
		if tc.hostnameResolver == nil {
			tc.hostnameResolver = func() (string, error) {
				return hostname, nil
			}
		}
		t.Run(fmt.Sprintf("external=%q,listen=%q", tc.external, tc.listen), func(t *testing.T) {
			u, err := extURL(log.NewNopLogger(), tc.hostnameResolver, tc.listen, tc.external)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expURL, u.String())
		})
	}
}
