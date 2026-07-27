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
	"fmt"

	"github.com/prometheus/alertmanager/template"
)

func composeBlockKitPayload(blocksTmpl any, channel, text string, tmplText func(string) string, tmplTextErr *error) (map[string]any, error) {
	tmplTextFunc := func(tmpl string) (string, error) {
		return tmplText(tmpl), *tmplTextErr
	}

	renderedBlocks, err := template.DeepCopyWithTemplate(blocksTmpl, tmplTextFunc)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"channel": channel,
		"text":    text,
		"blocks":  renderedBlocks,
	}, nil
}

func setBlockKitPayloadStringValue(payload any, key, value string) error {
	payloadMap, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("block_kit_payload must render to an object, got %T", payload)
	}
	payloadMap[key] = value
	return nil
}
