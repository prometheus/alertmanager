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

package victorops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestVictorOpsCustomFields(t *testing.T) {
	logger := log.NewNopLogger()
	tmpl := test.CreateTmpl(t)

	url, err := url.Parse("http://nowhere.com")

	require.NoError(t, err, "unexpected error parsing mock url")

	conf := &config.VictorOpsConfig{
		APIKey:            `12345`,
		APIURL:            &config.URL{URL: url},
		EntityDisplayName: `{{ .CommonLabels.Message }}`,
		StateMessage:      `{{ .CommonLabels.Message }}`,
		RoutingKey:        `test`,
		MessageType:       ``,
		MonitoringTool:    `AM`,
		CustomFields: map[string]string{
			"Field_A": "{{ .CommonLabels.Message }}",
		},
		HTTPConfig: &commoncfg.HTTPClientConfig{},
	}

	notifier, err := New(conf, tmpl, logger)
	require.NoError(t, err)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	alert := &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"Message": "message",
			},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	}

	msg, err := notifier.createVictorOpsPayload(ctx, alert)
	require.NoError(t, err)

	var m map[string]string
	err = json.Unmarshal(msg.Bytes(), &m)

	require.NoError(t, err)

	// Verify that a custom field was added to the payload and templatized.
	require.Equal(t, "message", m["Field_A"])
}

func TestVictorOpsRetry(t *testing.T) {
	notifier, err := New(
		&config.VictorOpsConfig{
			APIKey:     config.Secret("secret"),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)
	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
	}
}

func TestVictorOpsRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	secret := "secret"
	notifier, err := New(
		&config.VictorOpsConfig{
			APIURL:     &config.URL{URL: u},
			APIKey:     config.Secret(secret),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, secret)
}
