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
	"sort"
	"testing"

	"github.com/prometheus/common/model"
)

type alertLabelSetsByFingerprint []model.LabelSet

func (a alertLabelSetsByFingerprint) Len() int {
	return len(a)
}

func (a alertLabelSetsByFingerprint) Less(i, j int) bool {
	return a[i].Fingerprint() < a[i].Fingerprint()
}

func (a alertLabelSetsByFingerprint) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func labelSetsMustBeEqual(i int, t *testing.T, expected, actual []model.LabelSet) {
	if len(actual) != len(expected) {
		t.Fatalf("%d. Expected %d labelsets, got %d", i, len(expected), len(actual))
	}

	sort.Sort(alertLabelSetsByFingerprint(expected))
	sort.Sort(alertLabelSetsByFingerprint(actual))

	for j, l := range expected {
		if !l.Equal(actual[j]) {
			t.Fatalf("%d. Expected %v, got %v", i, l, actual[j])
		}
	}
}
