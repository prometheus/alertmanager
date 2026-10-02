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

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// TestNotifierConfigMuteAction covers the two predicates mute_action answers,
// including the nesting: a receiver that wants a still-firing group closed also
// wants one closed that resolved after muting hid it.
func TestNotifierConfigMuteAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		send bool
		// sendResolvedWhenMuted and treatsMuteAsResolved are the two predicates the
		// retry stage reads.
		sendResolvedWhenMuted bool
		treatsMuteAsResolved  bool
		err                   string
	}{{
		name: "muted groups are ignored by default",
		in:   "{}",
	}, {
		name: "ignore spelled out",
		in:   "mute_action: ignore",
	}, {
		name:                  "send_resolved_when_muted",
		in:                    "mute_action: send_resolved_when_muted",
		sendResolvedWhenMuted: true,
	}, {
		name:                  "treat_mute_as_resolved implies send_resolved_when_muted",
		in:                    "mute_action: treat_mute_as_resolved",
		sendResolvedWhenMuted: true,
		treatsMuteAsResolved:  true,
	}, {
		name:                  "alongside send_resolved",
		in:                    "send_resolved: true\nmute_action: treat_mute_as_resolved",
		send:                  true,
		sendResolvedWhenMuted: true,
		treatsMuteAsResolved:  true,
	}, {
		// A typo has to be an error: silently ignoring it would leave the
		// receiver with the behaviour the option was set to change.
		name: "unknown action",
		in:   "mute_action: resolved",
		err:  `invalid mute_action "resolved": expected "ignore", "send_resolved_when_muted" or "treat_mute_as_resolved"`,
	}, {
		// The option the reviewer asked to defer until a notifier can support
		// it. Accepting it now would promise behaviour that does not exist.
		name: "notifier defined is not implemented yet",
		in:   "mute_action: notify",
		err:  `invalid mute_action "notify": expected "ignore", "send_resolved_when_muted" or "treat_mute_as_resolved"`,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var nc NotifierConfig
			err := yaml.Unmarshal([]byte(test.in), &nc)
			if test.err != "" {
				require.EqualError(t, err, test.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.send, nc.SendResolved())
			require.Equal(t, test.sendResolvedWhenMuted, nc.SendsResolvedWhenMuted())
			require.Equal(t, test.treatsMuteAsResolved, nc.TreatsMuteAsResolved())
		})
	}
}
