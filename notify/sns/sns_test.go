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
	"context"
	"net/url"
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/sigv4"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

var logger = promslog.NewNopLogger()

func TestValidateAndTruncateMessage(t *testing.T) {
	sBuff := make([]byte, 257*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	truncatedMessage, isTruncated, err := validateAndTruncateMessage(string(sBuff), 256*1024)
	require.True(t, isTruncated)
	require.NoError(t, err)
	require.NotEqual(t, sBuff, truncatedMessage)
	require.Len(t, truncatedMessage, 256*1024)

	sBuff = make([]byte, 100)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	truncatedMessage, isTruncated, err = validateAndTruncateMessage(string(sBuff), 100)
	require.False(t, isTruncated)
	require.NoError(t, err)
	require.Equal(t, string(sBuff), truncatedMessage)

	invalidUtf8String := "\xc3\x28"
	_, _, err = validateAndTruncateMessage(invalidUtf8String, 100)
	require.Error(t, err)
}

func TestNotifyWithInvalidTemplate(t *testing.T) {
	for _, tc := range []struct {
		title     string
		errMsg    string
		updateCfg func(*config.SNSConfig)
	}{
		{
			title:  "with invalid Attribute template",
			errMsg: "execute 'attributes' template",
			updateCfg: func(cfg *config.SNSConfig) {
				cfg.Attributes = map[string]string{
					"attribName1": "{{ template \"unknown_template\" . }}",
				}
			},
		},
		{
			title:  "with invalid TopicArn template",
			errMsg: "execute 'topic_arn' template",
			updateCfg: func(cfg *config.SNSConfig) {
				cfg.TopicARN = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid PhoneNumber template",
			errMsg: "execute 'phone_number' template",
			updateCfg: func(cfg *config.SNSConfig) {
				cfg.PhoneNumber = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid Message template",
			errMsg: "execute 'message' template",
			updateCfg: func(cfg *config.SNSConfig) {
				cfg.Message = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid Subject template",
			errMsg: "execute 'subject' template",
			updateCfg: func(cfg *config.SNSConfig) {
				cfg.Subject = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid APIUrl template",
			errMsg: "execute 'api_url' template",
			updateCfg: func(cfg *config.SNSConfig) {
				cfg.APIUrl = "{{ template \"unknown_template\" . }}"
			},
		},
		{
			title:  "with invalid TargetARN template",
			errMsg: "execute 'target_arn' template",
			updateCfg: func(cfg *config.SNSConfig) {
				cfg.TargetARN = "{{ template \"unknown_template\" . }}"
			},
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			snsCfg := &config.SNSConfig{
				HTTPConfig: &commoncfg.HTTPClientConfig{},
				TopicARN:   "TestTopic",
				Sigv4: sigv4.SigV4Config{
					Region: "us-west-2",
				},
			}
			if tc.updateCfg != nil {
				tc.updateCfg(snsCfg)
			}
			notifier, err := New(
				snsCfg,
				createTmpl(t),
				logger,
			)
			require.NoError(t, err)
			var alerts []*types.Alert
			_, err = notifier.Notify(context.Background(), alerts...)
			require.Error(t, err)
			require.Contains(t, err.Error(), "template \"unknown_template\" not defined")
			require.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

// CreateTmpl returns a ready-to-use template.
func createTmpl(t *testing.T) *template.Template {
	tmpl, err := template.FromGlobs([]string{})
	require.NoError(t, err)
	tmpl.ExternalURL, _ = url.Parse("http://am")
	return tmpl
}
