// Copyright The Prometheus Authors
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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/template"
)

func TestComposeBlockKitPayload(t *testing.T) {
	blocksTmpl := []any{
		map[string]any{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": `*Summary:* {{ index .CommonAnnotations "summary" }}`,
			},
		},
	}

	tmpl := test.CreateTmpl(t)
	data := &template.Data{
		Status:            "firing",
		CommonAnnotations: template.KV{"summary": "CPU usage above 90 percent"},
	}
	var tmplTextErr error
	tmplText := notify.TmplText(tmpl, data, &tmplTextErr)

	payload, err := composeBlockKitPayload(blocksTmpl, "#alerts-channel", tmplText(`{{ .Status }}`), tmplText, &tmplTextErr)
	require.NoError(t, err)
	require.NoError(t, tmplTextErr)

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)

	require.JSONEq(t, `{
		"channel": "#alerts-channel",
		"text": "firing",
		"blocks": [
			{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": "*Summary:* CPU usage above 90 percent"
				}
			}
		]
	}`, string(encoded))
}
