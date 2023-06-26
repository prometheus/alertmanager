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

package old_parse

import (
	"reflect"
	"testing"

	"github.com/prometheus/alertmanager/matchers"
)

func TestMatchers(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  matchers.Matchers
		err   string
	}{
		{
			input: `{}`,
			want:  make(matchers.Matchers, 0),
		},
		{
			input: `{foo='}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "'")
				return append(ms, m)
			}(),
		},
		{
			input: "{foo=`}",
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "`")
				return append(ms, m)
			}(),
		},
		{
			input: "{foo=\\\"}",
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "\"")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=bar}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo="bar"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=~bar.*}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=~"bar.*"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!=bar}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchNotEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!="bar"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchNotEqual, "foo", "bar")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!~bar.*}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchNotRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo!~"bar.*"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchNotRegexp, "foo", "bar.*")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo="bar", baz!="quux"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				m2, _ := matchers.NewMatcher(matchers.MatchNotEqual, "baz", "quux")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo="bar", baz!~"quux.*"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				m2, _ := matchers.NewMatcher(matchers.MatchNotRegexp, "baz", "quux.*")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo="bar",baz!~".*quux", derp="wat"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				m2, _ := matchers.NewMatcher(matchers.MatchNotRegexp, "baz", ".*quux")
				m3, _ := matchers.NewMatcher(matchers.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", baz!="quux", derp="wat"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				m2, _ := matchers.NewMatcher(matchers.MatchNotEqual, "baz", "quux")
				m3, _ := matchers.NewMatcher(matchers.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", baz!~".*quux.*", derp="wat"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				m2, _ := matchers.NewMatcher(matchers.MatchNotRegexp, "baz", ".*quux.*")
				m3, _ := matchers.NewMatcher(matchers.MatchEqual, "derp", "wat")
				return append(ms, m, m2, m3)
			}(),
		},
		{
			input: `{foo="bar", instance=~"some-api.*"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				m2, _ := matchers.NewMatcher(matchers.MatchRegexp, "instance", "some-api.*")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo=""}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo="bar,quux", job="job1"}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar,quux")
				m2, _ := matchers.NewMatcher(matchers.MatchEqual, "job", "job1")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo = "bar", dings != "bums", }`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				m2, _ := matchers.NewMatcher(matchers.MatchNotEqual, "dings", "bums")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `foo=bar,dings!=bums`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar")
				m2, _ := matchers.NewMatcher(matchers.MatchNotEqual, "dings", "bums")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{quote="She said: \"Hi, ladies! That's gender-neutral…\""}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "quote", `She said: "Hi, ladies! That's gender-neutral…"`)
				return append(ms, m)
			}(),
		},
		{
			input: `statuscode=~"5.."`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchRegexp, "statuscode", "5..")
				return append(ms, m)
			}(),
		},
		{
			input: `tricky=~~~`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchRegexp, "tricky", "~~")
				return append(ms, m)
			}(),
		},
		{
			input: `trickier==\\=\=\"`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "trickier", `=\=\="`)
				return append(ms, m)
			}(),
		},
		{
			input: `contains_quote != "\"" , contains_comma !~ "foo,bar" , `,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchNotEqual, "contains_quote", `"`)
				m2, _ := matchers.NewMatcher(matchers.MatchNotRegexp, "contains_comma", "foo,bar")
				return append(ms, m, m2)
			}(),
		},
		{
			input: `{foo=bar}}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar}")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=bar}},}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "bar}}")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=,bar=}}`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m1, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "")
				m2, _ := matchers.NewMatcher(matchers.MatchEqual, "bar", "}")
				return append(ms, m1, m2)
			}(),
		},
		{
			input: `job=`,
			want: func() matchers.Matchers {
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "job", "")
				return matchers.Matchers{m}
			}(),
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
			input: `{invalid-name = "valid label"}`,
			err:   `bad matcher format: invalid-name = "valid label"`,
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
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "")
				return append(ms, m)
			}(),
		},
		{
			input: `{foo=}b`,
			want: func() matchers.Matchers {
				ms := matchers.Matchers{}
				m, _ := matchers.NewMatcher(matchers.MatchEqual, "foo", "}b")
				return append(ms, m)
			}(),
		},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseMatchers(tc.input)
			if err != nil && tc.err == "" {
				t.Fatalf("got error where none expected: %v", err)
			}
			if err == nil && tc.err != "" {
				t.Fatalf("expected error but got none: %v", tc.err)
			}
			if err != nil && err.Error() != tc.err {
				t.Fatalf("error not equal:\ngot  %v\nwant %v", err, tc.err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("labels not equal:\ngot  %v\nwant %v", got, tc.want)
			}
		})
	}
}
