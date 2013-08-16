// Copyright 2013 Prometheus Team
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

package manager

import (
	"bytes"
	"testing"
)

func TestWriteEmailBody(t *testing.T) {
	event := &Event{
		Summary:     "Testsummary",
		Description: "Test alert description, something went wrong here.",
		Labels: EventLabels{
			"alertname":       "TestAlert",
			"grouping_label1": "grouping_value1",
			"grouping_label2": "grouping_value2",
		},
		Payload: EventPayload{
			"payload_label1": "payload_value1",
			"payload_label2": "payload_value2",
		},
	}
	buf := &bytes.Buffer{}
	writeEmailBody(buf, event)

	expected := `Subject: [ALERT] TestAlert: Testsummary

Test alert description, something went wrong here.

Grouping labels:

  alertname = "TestAlert"
  grouping_label1 = "grouping_value1"
  grouping_label2 = "grouping_value2"

Payload labels:

  payload_label1 = "payload_value1"
  payload_label2 = "payload_value2"`

	if buf.String() != expected {
		t.Fatalf("Expected:\n%s\n\nGot:\n%s", expected, buf.String())
	}
}
