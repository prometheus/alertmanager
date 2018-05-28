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

package parse

import (
	"reflect"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
)

func TestMatchers(t *testing.T) {
	testCases := []struct {
		input string
		want  []*labels.Matcher
		err   error
	}{
		{
			input: `{foo="bar"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=~"bar.*"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!="bar"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchNotEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!~"bar.*"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchNotRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo="bar", baz!="quux"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotEqual, "baz", "quux")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo="bar", baz!~"quux.*"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotRegexp, "baz", "quux.*")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo="bar",baz!~".*quux", derp="wat"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotRegexp, "baz", ".*quux")
				m3, _ := labels.NewMatcher(labels.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", baz!="quux", derp="wat"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotEqual, "baz", "quux")
				m3, _ := labels.NewMatcher(labels.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", baz!~".*quux.*", derp="wat"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotRegexp, "baz", ".*quux.*")
				m3, _ := labels.NewMatcher(labels.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", instance=~"some-api.*"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchRegexp, "instance", "some-api.*")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo=""}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo="bar,quux", job="job1"}`,
			want: func() []*labels.Matcher {
				ms := []*labels.Matcher{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar,quux")
				m2, _ := labels.NewMatcher(labels.MatchEqual, "job", "job1")
				return append(ms, m, m2)
			}(),
		},
	}

	for i, tc := range testCases {
		got, err := Matchers(tc.input)
		if tc.err != err {
			t.Fatalf("error not equal (i=%d):\ngot  %v\nwant %v", i, err, tc.err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("labels not equal (i=%d):\ngot  %v\nwant %v", i, got, tc.want)
		}
	}

}
