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
		{
			// Valid configuration with session_name and tags.
			in: `topic_arn: topic
sigv4:
    role_arn: arn:aws:iam::123456789012:role/test
    session_name: test-session
    tags:
        team: observability
        env: prod
`,
			err: false,
		},
		{
			// session_name requires role_arn.
			in: `topic_arn: topic
sigv4:
    session_name: test-session
`,
			err: true,
		},
		{
			// tags require role_arn.
			in: `topic_arn: topic
sigv4:
    tags:
        team: observability
`,
			err: true,
		},
		{
			// Maximum 50 tags allowed (AWS STS limit).
			in: `topic_arn: topic
sigv4:
    role_arn: arn:aws:iam::123456789012:role/test
    tags:
        tag01: value
        tag02: value
        tag03: value
        tag04: value
        tag05: value
        tag06: value
        tag07: value
        tag08: value
        tag09: value
        tag10: value
        tag11: value
        tag12: value
        tag13: value
        tag14: value
        tag15: value
        tag16: value
        tag17: value
        tag18: value
        tag19: value
        tag20: value
        tag21: value
        tag22: value
        tag23: value
        tag24: value
        tag25: value
        tag26: value
        tag27: value
        tag28: value
        tag29: value
        tag30: value
        tag31: value
        tag32: value
        tag33: value
        tag34: value
        tag35: value
        tag36: value
        tag37: value
        tag38: value
        tag39: value
        tag40: value
        tag41: value
        tag42: value
        tag43: value
        tag44: value
        tag45: value
        tag46: value
        tag47: value
        tag48: value
        tag49: value
        tag50: value
        tag51: value
`,
			err: true,
		},
		{
			// Reserved aws: prefix not allowed in tag keys.
			in: `topic_arn: topic
sigv4:
    role_arn: arn:aws:iam::123456789012:role/test
    tags:
        aws:cloudformation: value
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
