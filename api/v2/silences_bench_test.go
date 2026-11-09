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

package v2

import (
	"strconv"
	"testing"

	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/alertmanager/silence/silencepb"
)

func BenchmarkCheckSilenceMatchesFilterLabels(b *testing.B) {
	sil := &silencepb.Silence{MatcherSets: []*silencepb.MatcherSet{{Matchers: []*silencepb.Matcher{
		{Name: "alertname", Pattern: "Foo", Type: silencepb.Matcher_EQUAL},
		{Name: "severity", Pattern: "critical", Type: silencepb.Matcher_NOT_EQUAL},
		{Name: "service", Pattern: "api|worker", Type: silencepb.Matcher_REGEXP},
	}}}}
	matchers := []*labels.Matcher{
		{Name: "alertname", Value: "Foo", Type: labels.MatchEqual},
		{Name: "severity", Value: "critical", Type: labels.MatchNotEqual},
		{Name: "service", Value: "api|worker", Type: labels.MatchRegexp},
	}
	for _, count := range []int{0, 1, 3} {
		b.Run(strconv.Itoa(count)+"_matchers", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if !CheckSilenceMatchesFilterLabels(sil, matchers[:count]) {
					b.Fatal("silence must match the filter")
				}
			}
		})
	}
}
