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

package metrics

import (
	"testing"

	"github.com/go-kit/log"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/client_golang/prometheus"
)

func Test_NewAlerts(t *testing.T) {
	t.Run("metrics are registered and collected successfully despite being registered previously", func(t *testing.T) {
		r := prometheus.NewRegistry()
		l := log.NewNopLogger()

		require.NotPanics(t, func() {
			for i := 0; i < 3; i++ {
				as := NewAlerts(r, l)
				as.Firing().Inc()
				as.Resolved().Inc()
				as.Invalid().Inc()

				mf, err := r.Gather()
				require.NoError(t, err)

				for j := 0; j < len(mf); j++ {
					require.Equal(t, float64(1), mf[j].GetMetric()[0].GetCounter().GetValue())
				}
			}
		})
	})
}
