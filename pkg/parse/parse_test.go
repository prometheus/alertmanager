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
	}

	for i, tc := range testCases {
		got, err := Matchers(tc.input)
		if err != nil {
			t.Fatalf("error (i=%d): %v", i, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("error not equal (i=%d):\ngot  %v\nwant %v", i, got, tc.want)
		}
	}

}
