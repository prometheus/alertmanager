// Copyright 2015 Prometheus Team
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

package feishu

import (
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFeishuInitialAuthentication(t *testing.T) {
	_, u, fn := test.GetContextWithCancelingURL()
	defer fn()
	app_id := "app_id"
	app_secret := "app_secret"
	_, err := New(
		&config.FeishuConfig{
			APIURL:     &config.URL{URL: u},
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			APPID:      app_id,
			APPSecret:  config.Secret(app_secret),
		},
		test.CreateTmpl(t),
		promslog.NewNopLogger(),
	)
	require.NoError(t, err)
}
