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

package welink

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestWeLinkRetry(t *testing.T) {
	testWebhookURL, err := url.Parse("https://open.welink.huaweicloud.com/api/werobot/v1/webhook/send")
	require.NoError(t, err)

	notifier, err := New(
		&config.WeLinkConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			APIUrl:     &config.URL{URL: testWebhookURL},
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

func TestWeLinkMockSuccess(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"code": "0", "data": "success", "message": "ok"}`)
	})
	defer fn()

	notifier, err := New(
		&config.WeLinkConfig{
			APIUrl:     &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	ctx = notify.WithGroupKey(ctx, "1")
	ok, err := notifier.Notify(ctx, []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"lbl1": "val1",
				},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			},
		},
	}...)

	require.False(t, ok)
	require.Nil(t, err)
}

func TestWeLinkMockFail(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"code": "58404", "data": "", "message": "not ok"}`)
	})
	defer fn()

	notifier, err := New(
		&config.WeLinkConfig{
			APIUrl:     &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	ctx = notify.WithGroupKey(ctx, "1")
	ok, err := notifier.Notify(ctx, []*types.Alert{
		{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"lbl1": "val1",
				},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			},
		},
	}...)

	require.False(t, ok)
	require.Equal(t, "not ok", err.Error())
}
