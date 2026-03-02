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
package silence

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCurrentState(t *testing.T) {
	var (
		pastStartTime = time.Now()
		pastEndTime   = time.Now()

		futureStartTime = time.Now().Add(time.Hour)
		futureEndTime   = time.Now().Add(time.Hour)
	)

	expected := CurrentState(futureStartTime, futureEndTime)
	require.Equal(t, SilenceStatePending, expected)

	expected = CurrentState(pastStartTime, futureEndTime)
	require.Equal(t, SilenceStateActive, expected)

	expected = CurrentState(pastStartTime, pastEndTime)
	require.Equal(t, SilenceStateExpired, expected)
}
