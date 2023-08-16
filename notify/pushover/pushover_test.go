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

package pushover

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestPushoverRetry(t *testing.T) {
	notifier, err := New(
		&config.PushoverConfig{
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

func TestPushoverRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	key, token := "user_key", "token"
	notifier, err := New(
		&config.PushoverConfig{
			UserKey:    config.Secret(key),
			Token:      config.Secret(token),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)
	notifier.apiURL = u.String()

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, key, token)
}

func TestPushoverReadingUserKeyFromFile(t *testing.T) {
	ctx, apiURL, fn := test.GetContextWithCancelingURL()
	defer fn()

	const userKey = "user key"
	f, err := os.CreateTemp("", "pushover_user_key")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(userKey)
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.PushoverConfig{
			UserKeyFile: f.Name(),
			Token:       config.Secret("token"),
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	notifier.apiURL = apiURL.String()
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, userKey)
}

func TestPushoverReadingTokenFromFile(t *testing.T) {
	ctx, apiURL, fn := test.GetContextWithCancelingURL()
	defer fn()

	const token = "token"
	f, err := os.CreateTemp("", "pushover_token")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString(token)
	require.NoError(t, err, "writing to temp file failed")

	notifier, err := New(
		&config.PushoverConfig{
			UserKey:    config.Secret("user key"),
			TokenFile:  f.Name(),
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	notifier.apiURL = apiURL.String()
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, token)
}
