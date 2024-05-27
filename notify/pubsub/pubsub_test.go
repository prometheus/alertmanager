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

package pubsub

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestWebhookRetry(t *testing.T) {
	u, err := url.Parse("http://example.com")
	if err != nil {
		require.NoError(t, err)
	}
	u.Path = "/v1/projects/{project}/topics/{topic}:publish"

	secret := GetFakeServiceAccount()
	file, err2 := os.CreateTemp("", "service_account.json")
	require.NoError(t, err2)
	_, err3 := file.WriteString(secret)
	require.NoError(t, err3)
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			t.Errorf("failed to remove file %s: %s", name, err)
		}
	}(file.Name())
	notifier, err := New(
		&config.GooglePubsubConfig{
			Project:     "test-project",
			Topic:       "test-topic",
			TemplateURL: &config.URL{URL: u},
			Authorization: &config.GooglePubsubAuthorization{
				ServiceAccountFile: file.Name(),
			},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	if err != nil {
		require.NoError(t, err)
	}

	t.Run("test retry status code", func(t *testing.T) {
		for statusCode, expected := range test.RetryTests(test.DefaultRetryCodes()) {
			actual, _ := notifier.retrier.Check(statusCode, nil)
			require.Equal(t, expected, actual, fmt.Sprintf("error on status %d", statusCode))
		}
	})

	t.Run("test retry error details", func(t *testing.T) {
		for _, tc := range []struct {
			status int
			body   io.Reader

			exp string
		}{
			{
				status: http.StatusBadRequest,
				body: bytes.NewBuffer([]byte(
					`{"status":"invalid event"}`,
				)),

				exp: fmt.Sprintf(`unexpected status code %d: %s: {"status":"invalid event"}`, http.StatusBadRequest, u.String()),
			},
			{
				status: http.StatusBadRequest,

				exp: fmt.Sprintf(`unexpected status code %d: %s`, http.StatusBadRequest, u.String()),
			},
		} {
			t.Run("", func(t *testing.T) {
				_, err = notifier.retrier.Check(tc.status, tc.body)
				require.Equal(t, tc.exp, err.Error())
			})
		}
	})
}

func GetFakeServiceAccount() string {
	// source: https://github.com/google/oauth2l/blob/master/integration/fixtures/fake-service-account.json
	return "{\n  \"type\": \"service_account\",\n  \"private_key_id\": \"abc\",\n  \"private_key\": \"-----BEGIN PRIVATE KEY-----\\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDY3E8o1NEFcjMM\\nHW/5ZfFJw29/8NEqpViNjQIx95Xx5KDtJ+nWn9+OW0uqsSqKlKGhAdAo+Q6bjx2c\\nuXVsXTu7XrZUY5Kltvj94DvUa1wjNXs606r/RxWTJ58bfdC+gLLxBfGnB6CwK0YQ\\nxnfpjNbkUfVVzO0MQD7UP0Hl5ZcY0Puvxd/yHuONQn/rIAieTHH1pqgW+zrH/y3c\\n59IGThC9PPtugI9ea8RSnVj3PWz1bX2UkCDpy9IRh9LzJLaYYX9RUd7++dULUlat\\nAaXBh1U6emUDzhrIsgApjDVtimOPbmQWmX1S60mqQikRpVYZ8u+NDD+LNw+/Eovn\\nxCj2Y3z1AgMBAAECggEAWDBzoqO1IvVXjBA2lqId10T6hXmN3j1ifyH+aAqK+FVl\\nGjyWjDj0xWQcJ9ync7bQ6fSeTeNGzP0M6kzDU1+w6FgyZqwdmXWI2VmEizRjwk+/\\n/uLQUcL7I55Dxn7KUoZs/rZPmQDxmGLoue60Gg6z3yLzVcKiDc7cnhzhdBgDc8vd\\nQorNAlqGPRnm3EqKQ6VQp6fyQmCAxrr45kspRXNLddat3AMsuqImDkqGKBmF3Q1y\\nxWGe81LphUiRqvqbyUlh6cdSZ8pLBpc9m0c3qWPKs9paqBIvgUPlvOZMqec6x4S6\\nChbdkkTRLnbsRr0Yg/nDeEPlkhRBhasXpxpMUBgPywKBgQDs2axNkFjbU94uXvd5\\nznUhDVxPFBuxyUHtsJNqW4p/ujLNimGet5E/YthCnQeC2P3Ym7c3fiz68amM6hiA\\nOnW7HYPZ+jKFnefpAtjyOOs46AkftEg07T9XjwWNPt8+8l0DYawPoJgbM5iE0L2O\\nx8TU1Vs4mXc+ql9F90GzI0x3VwKBgQDqZOOqWw3hTnNT07Ixqnmd3dugV9S7eW6o\\nU9OoUgJB4rYTpG+yFqNqbRT8bkx37iKBMEReppqonOqGm4wtuRR6LSLlgcIU9Iwx\\nyfH12UWqVmFSHsgZFqM/cK3wGev38h1WBIOx3/djKn7BdlKVh8kWyx6uC8bmV+E6\\nOoK0vJD6kwKBgHAySOnROBZlqzkiKW8c+uU2VATtzJSydrWm0J4wUPJifNBa/hVW\\ndcqmAzXC9xznt5AVa3wxHBOfyKaE+ig8CSsjNyNZ3vbmr0X04FoV1m91k2TeXNod\\njMTobkPThaNm4eLJMN2SQJuaHGTGERWC0l3T18t+/zrDMDCPiSLX1NAvAoGBAN1T\\nVLJYdjvIMxf1bm59VYcepbK7HLHFkRq6xMJMZbtG0ryraZjUzYvB4q4VjHk2UDiC\\nlhx13tXWDZH7MJtABzjyg+AI7XWSEQs2cBXACos0M4Myc6lU+eL+iA+OuoUOhmrh\\nqmT8YYGu76/IBWUSqWuvcpHPpwl7871i4Ga/I3qnAoGBANNkKAcMoeAbJQK7a/Rn\\nwPEJB+dPgNDIaboAsh1nZhVhN5cvdvCWuEYgOGCPQLYQF0zmTLcM+sVxOYgfy8mV\\nfbNgPgsP5xmu6dw2COBKdtozw0HrWSRjACd1N4yGu75+wPCcX/gQarcjRcXXZeEa\\nNtBLSfcqPULqD+h7br9lEJio\\n-----END PRIVATE KEY-----\\n\",\n  \"client_email\": \"123-abc@developer.gserviceaccount.com\",\n  \"client_id\": \"123-abc.apps.googleusercontent.com\",\n  \"auth_uri\": \"https://accounts.google.com/o/oauth2/auth\",\n  \"token_uri\": \"http://localhost:8080/token\"\n}"
}

func TestWebhookTruncateAlerts(t *testing.T) {
	alerts := make([]*types.Alert, 10)

	truncatedAlerts, numTruncated := truncateAlerts(0, alerts)
	require.Len(t, truncatedAlerts, 10)
	require.EqualValues(t, numTruncated, 0)

	truncatedAlerts, numTruncated = truncateAlerts(4, alerts)
	require.Len(t, truncatedAlerts, 4)
	require.EqualValues(t, numTruncated, 6)

	truncatedAlerts, numTruncated = truncateAlerts(100, alerts)
	require.Len(t, truncatedAlerts, 10)
	require.EqualValues(t, numTruncated, 0)
}

func TestWebhookRedactedURL(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	u.Path = "/v1/projects/{project}/topics/{topic}:publish"

	secret := GetFakeServiceAccount()
	file, err := os.CreateTemp("", "service_account.json")
	require.NoError(t, err)
	_, err2 := file.WriteString(secret)
	require.NoError(t, err2)
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			t.Errorf("failed to remove file %s: %s", name, err)
		}
	}(file.Name())
	notifier, err := New(
		&config.GooglePubsubConfig{
			TemplateURL: &config.URL{URL: u},
			Project:     "test-project",
			Topic:       "test-topic",
			Authorization: &config.GooglePubsubAuthorization{
				ServiceAccountFile: file.Name(),
			},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, secret)
}

func TestWebhookReadingServiceAccountFromFile(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	u.Path = "/v1/projects/{project}/topics/{topic}:publish"

	secret := GetFakeServiceAccount()
	file, err := os.CreateTemp("", "service_account.json")
	require.NoError(t, err)
	_, err2 := file.WriteString(secret)
	require.NoError(t, err2)
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			t.Errorf("failed to remove file %s: %s", name, err)
		}
	}(file.Name())

	notifier, err := New(
		&config.GooglePubsubConfig{
			Project:     "test-project",
			Topic:       "test-topic",
			TemplateURL: &config.URL{URL: u},
			Authorization: &config.GooglePubsubAuthorization{
				ServiceAccountFile: file.Name(),
			},
		},
		test.CreateTmpl(t),
		log.NewNopLogger(),
	)
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, notifier, u.String())
}
