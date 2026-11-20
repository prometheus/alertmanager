// Copyright 2023 Prometheus Team
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

package featurecontrol

import (
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

func TestFlags(t *testing.T) {
	tc := []struct {
		name         string
		featureFlags string
		err          error
	}{
		{
			name:         "with only valid feature flags",
			featureFlags: FeatureReceiverNameInMetrics,
		},
		{
			name:         "with only invalid feature flags",
			featureFlags: "somethingsomething",
			err:          errors.New("unknown option 'somethingsomething' for --enable-feature"),
		},
		{
			name:         "with both, valid and invalid feature flags",
			featureFlags: strings.Join([]string{FeatureReceiverNameInMetrics, "somethingbad"}, ","),
			err:          errors.New("unknown option 'somethingbad' for --enable-feature"),
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			fc, err := NewFlags(promslog.NewNopLogger(), tt.featureFlags)
			if tt.err != nil {
				require.EqualError(t, err, tt.err.Error())
			} else {
				require.NoError(t, err)
				require.NotNil(t, fc)
			}
		})
	}
}
