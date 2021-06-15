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
	"io/ioutil"
	"testing"

	"github.com/go-kit/kit/log"
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

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, key, token)
}

func TestPushoverSecretsFromFile(t *testing.T) {
	key := "secret"
	f, err := ioutil.TempFile("", "pushover_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString("")
	require.NoError(t, err, "writing to temp file failed")

	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	notifier, err := New(
		&config.PushoverConfig{
			UserKeyFile: f.Name(),
			TokenFile:   f.Name(),
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)
	notifier.apiURL = u.String()

	test.AssertNotifyLeaksNoSecret(t, ctx, notifier, key)
}
