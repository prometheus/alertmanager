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

package slack

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestSlackRetry(t *testing.T) {
	notifier, err := New(
		&config.SlackConfig{
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

func TestSlackPostMessageHappyPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"ok": true}`)
	}))
	defer ts.Close()
	tsUrl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	tsUrl.Path = "api/chat.postMessage"

	notifier, err := New(
		&config.SlackConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			APIURL:     &config.SecretURL{URL: tsUrl},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	_, err = notifier.Notify(context.TODO(), &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"lbl1": "val1",
			},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	})
	require.NoError(t, err)
}

func TestSlackPostMessageErrorHandling(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"ok": false, "error": "no_text"}`)
	}))
	defer ts.Close()
	tsUrl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	tsUrl.Path = "api/chat.postMessage"

	notifier, err := New(
		&config.SlackConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			APIURL:     &config.SecretURL{URL: tsUrl},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	retry, err := notifier.Notify(context.TODO(), &types.Alert{
		Alert: model.Alert{
			Labels: model.LabelSet{
				"lbl1": "val1",
			},
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(time.Hour),
		},
	})
	require.False(t, retry, "Should not retry configuration errors")
	require.Error(t, err)
}

func TestSlackRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	notifier, err := New(
		&config.SlackConfig{
			APIURL:     &config.SecretURL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestGettingSlackURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp("", "slack_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String())
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.SlackConfig{
			APIURLFile: f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}

func TestTrimmingSlackURLFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	f, err := os.CreateTemp("", "slack_test_newline")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(u.String() + "\n\n")
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.SlackConfig{
			APIURLFile: f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}
