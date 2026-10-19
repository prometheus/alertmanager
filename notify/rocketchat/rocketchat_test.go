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

package rocketchat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestRocketchatMessageText(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		messageText string
		wantText    bool
	}{
		{name: "unset"},
		{name: "title template", messageText: `{{ template "rocketchat.default.title" . }}`, wantText: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			secret := commoncfg.Secret("xxxxx")
			conf := DefaultRocketchatConfig
			conf.HTTPConfig = &commoncfg.HTTPClientConfig{}
			conf.APIURL = &amcommoncfg.URL{URL: &url.URL{Scheme: "https", Host: "example.com"}}
			conf.Token = &secret
			conf.TokenID = &secret
			conf.MessageText = tc.messageText
			notifier, err := New(&conf, test.CreateTmpl(t), promslog.NewNopLogger())
			require.NoError(t, err)

			var payload map[string]json.RawMessage
			notifier.postJSONFunc = func(_ context.Context, _ *http.Client, _ string, body io.Reader) (*http.Response, error) {
				require.NoError(t, json.NewDecoder(body).Decode(&payload))
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"success":true}`))}, nil
			}

			ctx := notify.WithGroupKey(context.Background(), "test-group")
			verdict := notifier.Notify(ctx, alert.New(model.Alert{StartsAt: time.Now()}, time.Time{}, false))
			require.NoError(t, verdict.Err())
			require.False(t, verdict.ShouldRetry())

			var attachments []Attachment
			require.NoError(t, json.Unmarshal(payload["attachments"], &attachments))
			require.Len(t, attachments, 1)
			require.Equal(t, "[FIRING:1]  ", attachments[0].Title)
			if tc.wantText {
				var text string
				require.NoError(t, json.Unmarshal(payload["text"], &text))
				require.Equal(t, attachments[0].Title, text)
			} else {
				require.NotContains(t, payload, "text")
			}
		})
	}
}

func TestRocketchatRetry(t *testing.T) {
	secret := commoncfg.Secret("xxxxx")
	notifier, err := New(
		&RocketchatConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			Token:      &secret,
			TokenID:    &secret,
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)

	for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
		actual, _ := notifier.retrier.Check(statusCode, nil)
		require.Equal(t, expected, actual, "error on status %d", statusCode)
	}
}

func TestGettingRocketchatTokenFromFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "rocketchat_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString("secret")
	require.NoError(t, err, "writing to temp file failed")

	_, err = New(
		&RocketchatConfig{
			TokenFile:   f.Name(),
			TokenIDFile: f.Name(),
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			APIURL:      &amcommoncfg.URL{URL: &url.URL{Scheme: "http", Host: "example.com", Path: "/api/v1/"}},
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)
}
