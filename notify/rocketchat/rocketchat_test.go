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
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/go-kit/log"
	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
)

func TestRocketchatRetry(t *testing.T) {
	secret := config.Secret("xxxxx")
	notifier, err := New(
		&config.RocketchatConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			Token:      &secret,
			TokenID:    &secret,
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

func TestGettingRocketchatTokenFromFile(t *testing.T) {
	f, err := os.CreateTemp("", "rocketchat_test")
	require.NoError(t, err, "creating temp file failed")
	_, err = f.WriteString("secret")
	require.NoError(t, err, "writing to temp file failed")

	_, err = New(
		&config.RocketchatConfig{
			TokenFile:   f.Name(),
			TokenIDFile: f.Name(),
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			APIURL:      &config.URL{URL: &url.URL{Scheme: "http", Host: "example.com", Path: "/api/v1/"}},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)
}
