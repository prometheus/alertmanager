// Copyright 2023 Prometheus Team
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

package receiver

import (
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
)

type sendResolved bool

func (s sendResolved) SendResolved() bool { return bool(s) }

func TestBuildReceiverIntegrations(t *testing.T) {
	for _, tc := range []struct {
		receiver config.Receiver
		err      bool
		exp      []notify.Integration
	}{
		{
			receiver: config.Receiver{
				Name: "foo",
				WebhookConfigs: []*config.WebhookConfig{
					{
						HTTPConfig: &commoncfg.HTTPClientConfig{},
					},
					{
						HTTPConfig: &commoncfg.HTTPClientConfig{},
						NotifierConfig: config.NotifierConfig{
							VSendResolved: true,
						},
					},
				},
			},
			exp: []notify.Integration{
				notify.NewIntegration(nil, sendResolved(false), "webhook", 0, "foo"),
				notify.NewIntegration(nil, sendResolved(true), "webhook", 1, "foo"),
			},
		},
		{
			receiver: config.Receiver{
				Name: "foo",
				WebhookConfigs: []*config.WebhookConfig{
					{
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
			integrations, err := BuildReceiverIntegrations(tc.receiver, nil, nil)
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
