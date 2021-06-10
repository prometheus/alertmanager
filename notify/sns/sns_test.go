// Copyright 2021 Prometheus Team
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

package sns

import (
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestNotifier_Notify(t *testing.T) {
	ctx, _, fn := test.GetContextWithCancelingURL()
	defer fn()
	attrTest := map[string]string{}
	attrTest["key"] = "testVal"
	// These are fake values
	notifier, err := New(
		&config.SNSConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{},
			Message:    `{{ template "sns.default.message" . }}`,
			TopicARN:   "arn:aws:sns:us-east-2:123456789012:My-Topic",
			Sigv4: config.SigV4Config{
				Region:    "us-east-2",
				AccessKey: "access_key",
				SecretKey: "secret_key",
			},
			Attributes: attrTest,
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	ok, err := notifier.Notify(ctx, []*types.Alert{
		&types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"lbl1": "val1",
				},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(time.Hour),
			},
		},
	}...)
	require.NoError(t, err)
	require.False(t, ok)
}
