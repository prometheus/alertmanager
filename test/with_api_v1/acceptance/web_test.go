// Copyright 2018 Prometheus Team
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

package test

import (
	"testing"

	a "github.com/prometheus/alertmanager/test/with_api_v1"
)

func TestWebWithPrefix(t *testing.T) {
	t.Parallel()

	conf := `
route:
  receiver: "default"
  group_by: []
  group_wait:      1s
  group_interval:  1s
  repeat_interval: 1h

receivers:
- name: "default"
`

	// The test framework polls the API with the given prefix during
	// Alertmanager startup and thereby ensures proper configuration.
	at := a.NewAcceptanceTest(t, &a.AcceptanceOpts{RoutePrefix: "/foo"})
	at.Alertmanager(conf)
	at.Run()
}
