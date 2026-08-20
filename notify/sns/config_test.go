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

package sns

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestSNS(t *testing.T) {
	for _, tc := range []struct {
		in  string
		err bool
	}{
		{
			// Valid configuration without sigv4.
			in:  `target_arn: target`,
			err: false,
		},
		{
			// Valid configuration without sigv4.
			in:  `topic_arn: topic`,
			err: false,
		},
		{
			// Valid configuration with sigv4.
			in: `phone_number: phone
sigv4:
    access_key: abc
    secret_key: abc
`,
			err: false,
		},
		{
			// at most one of 'target_arn', 'topic_arn' or 'phone_number' must be provided without sigv4.
			in: `topic_arn: topic
target_arn: target
`,
			err: true,
		},
		{
			// at most one of 'target_arn', 'topic_arn' or 'phone_number' must be provided without sigv4.
			in: `topic_arn: topic
phone_number: phone
`,
			err: true,
		},
		{
			// one of 'target_arn', 'topic_arn' or 'phone_number' must be provided without sigv4.
			in:  "{}",
			err: true,
		},
		{
			// one of 'target_arn', 'topic_arn' or 'phone_number' must be provided with sigv4.
			in: `sigv4:
    access_key: abc
    secret_key: abc
`,
			err: true,
		},
		{
			// 'secret_key' must be provided with 'access_key'.
			in: `topic_arn: topic
sigv4:
    access_key: abc
`,
			err: true,
		},
		{
			// 'access_key' must be provided with 'secret_key'.
			in: `topic_arn: topic
sigv4:
    secret_key: abc
`,
			err: true,
		},
	} {
		t.Run("", func(t *testing.T) {
			var cfg SNSConfig
			err := yaml.UnmarshalStrict([]byte(tc.in), &cfg)
			if err != nil {
				if !tc.err {
					t.Errorf("expecting no error, got %q", err)
				}
				return
			}

			if tc.err {
				t.Logf("%#v", cfg)
				t.Error("expecting error, got none")
			}
		})
	}
}
