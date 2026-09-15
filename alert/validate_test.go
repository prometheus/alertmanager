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

package alert

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
)

func TestValidateUTF8Ls(t *testing.T) {
	tests := []struct {
		name string
		ls   model.LabelSet
		err  string
	}{{
		name: "valid UTF-8 label set",
		ls: model.LabelSet{
			"a":                "a",
			"00":               "b",
			"Σ":                "c",
			"\xf0\x9f\x99\x82": "dΘ",
		},
	}, {
		name: "invalid UTF-8 label set",
		ls: model.LabelSet{
			"\xff": "a",
		},
		err: "invalid name \"\\xff\"",
	}}

	// Change the mode to UTF-8 mode.
	ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureUTF8StrictMode)
	require.NoError(t, err)
	compat.InitFromFlags(promslog.NewNopLogger(), ff)

	// Restore the mode to classic at the end of the test.
	ff, err = featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureClassicMode)
	require.NoError(t, err)
	defer compat.InitFromFlags(promslog.NewNopLogger(), ff)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateLs(test.ls)
			if err != nil && err.Error() != test.err {
				t.Errorf("unexpected err for %s: %s", test.ls, err)
			} else if err == nil && test.err != "" {
				t.Error("expected error, got nil")
			}
		})
	}
}
