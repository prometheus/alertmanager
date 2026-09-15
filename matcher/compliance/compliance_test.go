// Copyright 2023 The Prometheus Authors
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

package compliance

import (
	"reflect"
	"testing"

	"github.com/prometheus/alertmanager/matcher/parse"
	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestCompliance(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  labels.Matchers
		err   string
		skip  bool
	}{
		{
			input: `{}`,
			want:  labels.Matchers{},
			skip:  true,
		},
		{
			input: `{foo='}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "'")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: "{foo=`}",
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "`")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=\n}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "\n")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=bar\n}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar\n")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=\t}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "\\t")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=bar\t}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar\\t")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=bar\}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar\\")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=bar\\}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar\\")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=\"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "\"")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=bar\"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar\"")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=bar}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo="bar"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=~bar.*}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=~"bar.*"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!=bar}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchNotEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!="bar"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchNotEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!~bar.*}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchNotRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!~"bar.*"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchNotRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo="bar", baz!="quux"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotEqual, "baz", "quux")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo="bar", baz!~"quux.*"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotRegexp, "baz", "quux.*")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo="bar",baz!~".*quux", derp="wat"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotRegexp, "baz", ".*quux")
				m3, _ := labels.NewMatcher(labels.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", baz!="quux", derp="wat"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotEqual, "baz", "quux")
				m3, _ := labels.NewMatcher(labels.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", baz!~".*quux.*", derp="wat"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotRegexp, "baz", ".*quux.*")
				m3, _ := labels.NewMatcher(labels.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", instance=~"some-api.*"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchRegexp, "instance", "some-api.*")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo=""}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo="bar,quux", job="job1"}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar,quux")
				m2, _ := labels.NewMatcher(labels.MatchEqual, "job", "job1")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo = "bar", dings != "bums", }`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotEqual, "dings", "bums")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `foo=bar,dings!=bums`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar")
				m2, _ := labels.NewMatcher(labels.MatchNotEqual, "dings", "bums")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{quote="She said: \"Hi, ladies! That's gender-neutral…\""}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "quote", `She said: "Hi, ladies! That's gender-neutral…"`)
				return append(ms, m)
			}(),
		},
		{
			input: `statuscode=~"5.."`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchRegexp, "statuscode", "5..")
				return append(ms, m)
			}(),
		},
		{
			input: `tricky=~~~`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchRegexp, "tricky", "~~")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `trickier==\\=\=\"`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "trickier", `=\=\="`)
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `contains_quote != "\"" , contains_comma !~ "foo,bar" , `,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchNotEqual, "contains_quote", `"`)
				m2, _ := labels.NewMatcher(labels.MatchNotRegexp, "contains_comma", "foo,bar")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo=bar}}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar}")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=bar}},}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "bar}}")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=,bar=}}`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m1, _ := labels.NewMatcher(labels.MatchEqual, "foo", "")
				m2, _ := labels.NewMatcher(labels.MatchEqual, "bar", "}")
				return append(ms, m1, m2)
			}(),
			skip: true,
		},
		{
			input: `job=`,
			want: func() labels.Matchers {
				m, _ := labels.NewMatcher(labels.MatchEqual, "job", "")
				return labels.Matchers{m}
			}(),
			skip: true,
		},
		{
			input: `{name-with-dashes = "bar"}`,
			want: func() labels.Matchers {
				m, _ := labels.NewMatcher(labels.MatchEqual, "name-with-dashes", "bar")
				return labels.Matchers{m}
			}(),
		},
		{
			input: `{,}`,
			err:   "bad matcher format: ",
		},
		{
			input: `job="value`,
			err:   `matcher value contains unescaped double quote: "value`,
		},
		{
			input: `job=value"`,
			err:   `matcher value contains unescaped double quote: value"`,
		},
		{
			input: `trickier==\\=\=\""`,
			err:   `matcher value contains unescaped double quote: =\\=\=\""`,
		},
		{
			input: `contains_unescaped_quote = foo"bar`,
			err:   `matcher value contains unescaped double quote: foo"bar`,
		},
		{
			input: `{foo=~"invalid[regexp"}`,
			err:   "error parsing regexp: missing closing ]: `[regexp)$`",
		},
		// Double escaped strings.
		{
			input: `"{foo=\"bar"}`,
			err:   `bad matcher format: "{foo=\"bar"`,
		},
		{
			input: `"foo=\"bar"`,
			err:   `bad matcher format: "foo=\"bar"`,
		},
		{
			input: `"foo=\"bar\""`,
			err:   `bad matcher format: "foo=\"bar\""`,
		},
		{
			input: `"foo=\"bar\"`,
			err:   `bad matcher format: "foo=\"bar\"`,
		},
		{
			input: `"{foo=\"bar\"}"`,
			err:   `bad matcher format: "{foo=\"bar\"}"`,
		},
		{
			input: `"foo="bar""`,
			err:   `bad matcher format: "foo="bar""`,
		},
		{
			input: `{{foo=`,
			err:   `bad matcher format: {foo=`,
		},
		{
			input: `{foo=`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "")
				return append(ms, m)
			}(),
			skip: true,
		},
		{
			input: `{foo=}b`,
			want: func() labels.Matchers {
				ms := labels.Matchers{}
				m, _ := labels.NewMatcher(labels.MatchEqual, "foo", "}b")
				return append(ms, m)
			}(),
			skip: true,
		},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}
			got, err := parse.Matchers(tc.input)
			if err != nil && tc.err == "" {
				t.Fatalf("got error where none expected: %v", err)
			}
			if err == nil && tc.err != "" {
				t.Fatalf("expected error but got none: %v", tc.err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("matchers not equal:\ngot %s\nwant %s", got, tc.want)
			}
		})
	}
}
