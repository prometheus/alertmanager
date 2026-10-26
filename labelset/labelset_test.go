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

package labelset

import (
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
)

func TestFingerprint(t *testing.T) {
	ls := model.LabelSet{"alertname": "test", "instance": "a"}
	want := ls.Fingerprint()

	if got := FromModel(ls).Fingerprint(); got != want {
		t.Fatalf("FromModel: got %v, want %v", got, want)
	}
	if got := New(ls, want).Fingerprint(); got != want {
		t.Fatalf("New: got %v, want %v", got, want)
	}
	// The provided value is returned as is, without hashing the labels.
	if got := New(ls, 42).Fingerprint(); got != 42 {
		t.Fatalf("New provided: got %v, want 42", got)
	}
}

// BenchmarkConstruct measures the cost that reusing a known fingerprint saves
// per LabelSet construction, for label sets of typical sizes.
func BenchmarkConstruct(b *testing.B) {
	for _, n := range []int{1, 5, 10, 20} {
		ls := make(model.LabelSet, n)
		for i := range n {
			ls[model.LabelName("label_"+strconv.Itoa(i))] = model.LabelValue("value_" + strconv.Itoa(i))
		}
		fp := ls.Fingerprint()

		b.Run("labels="+strconv.Itoa(n)+"/FromModel", func(b *testing.B) {
			for b.Loop() {
				sink = FromModel(ls).Fingerprint()
			}
		})
		b.Run("labels="+strconv.Itoa(n)+"/New", func(b *testing.B) {
			for b.Loop() {
				sink = New(ls, fp).Fingerprint()
			}
		})
	}
}

var sink model.Fingerprint
