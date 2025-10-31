// Copyright 2017 The Prometheus Authors
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

package labels

import (
	"encoding/json"
	"testing"
)

func mustNewMatcher(t *testing.T, mType MatchType, value string) *Matcher {
	m, err := NewMatcher(mType, "", value)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMatcher(t *testing.T) {
	tests := []struct {
		matcher *Matcher
		value   string
		match   bool
	}{
		{
			matcher: mustNewMatcher(t, MatchEqual, "bar"),
			value:   "bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchEqual, "bar"),
			value:   "foo-bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchNotEqual, "bar"),
			value:   "bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchNotEqual, "bar"),
			value:   "foo-bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "bar"),
			value:   "bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "bar"),
			value:   "foo-bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, ".*bar"),
			value:   "foo-bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchNotRegexp, "bar"),
			value:   "bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchNotRegexp, "bar"),
			value:   "foo-bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchNotRegexp, ".*bar"),
			value:   "foo-bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, `foo.bar`),
			value:   "foo-bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, `foo\.bar`),
			value:   "foo-bar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, `foo\.bar`),
			value:   "foo.bar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchEqual, "foo\nbar"),
			value:   "foo\nbar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "foo.bar"),
			value:   "foo\nbar",
			match:   false,
		},
		{
			matcher: mustNewMatcher(t, MatchRegexp, "(?s)foo.bar"),
			value:   "foo\nbar",
			match:   true,
		},
		{
			matcher: mustNewMatcher(t, MatchEqual, "~!=\""),
			value:   "~!=\"",
			match:   true,
		},
	}

	for _, test := range tests {
		if test.matcher.Matches(test.value) != test.match {
			t.Fatalf("Unexpected match result for matcher %v and value %q; want %v, got %v", test.matcher, test.value, test.match, !test.match)
		}
	}
}

func TestMatcherString(t *testing.T) {
	tests := []struct {
		name  string
		op    MatchType
		value string
		want  string
	}{
		{
			name:  `foo`,
			op:    MatchEqual,
			value: `bar`,
			want:  `foo="bar"`,
		},
		{
			name:  `foo`,
			op:    MatchNotEqual,
			value: `bar`,
			want:  `foo!="bar"`,
		},
		{
			name:  `foo`,
			op:    MatchRegexp,
			value: `bar`,
			want:  `foo=~"bar"`,
		},
		{
			name:  `foo`,
			op:    MatchNotRegexp,
			value: `bar`,
			want:  `foo!~"bar"`,
		},
		{
			name:  `foo`,
			op:    MatchEqual,
			value: `back\slash`,
			want:  `foo="back\\slash"`,
		},
		{
			name:  `foo`,
			op:    MatchEqual,
			value: `double"quote`,
			want:  `foo="double\"quote"`,
		},
		{
			name: `foo`,
			op:   MatchEqual,
			value: `new
line`,
			want: `foo="new\nline"`,
		},
		{
			name:  `foo`,
			op:    MatchEqual,
			value: `tab	stop`,
			want:  `foo="tab	stop"`,
		},
		{
			name:  `foo`,
			op:    MatchEqual,
			value: `🙂`,
			want:  `foo="🙂"`,
		},
		{
			name:  `foo!`,
			op:    MatchNotEqual,
			value: `bar`,
			want:  `"foo!"!="bar"`,
		},
		{
			name:  `foo🙂`,
			op:    MatchEqual,
			value: `bar`,
			want:  `foo🙂="bar"`,
		},
		{
			name:  `foo bar`,
			op:    MatchEqual,
			value: `baz`,
			want:  `"foo bar"="baz"`,
		},
	}

	for _, test := range tests {
		m, err := NewMatcher(test.op, test.name, test.value)
		if err != nil {
			t.Fatal(err)
		}
		if got := m.String(); got != test.want {
			t.Errorf("Unexpected string representation of matcher; want %v, got %v", test.want, got)
		}
	}
}

func TestMatcherJSONMarshal(t *testing.T) {
	tests := []struct {
		name  string
		op    MatchType
		value string
		want  string
	}{
		{
			name:  `foo`,
			op:    MatchEqual,
			value: `bar`,
			want:  `{"name":"foo","value":"bar","isRegex":false,"isEqual":true}`,
		},
		{
			name:  `foo`,
			op:    MatchNotEqual,
			value: `bar`,
			want:  `{"name":"foo","value":"bar","isRegex":false,"isEqual":false}`,
		},
		{
			name:  `foo`,
			op:    MatchRegexp,
			value: `bar`,
			want:  `{"name":"foo","value":"bar","isRegex":true,"isEqual":true}`,
		},
		{
			name:  `foo`,
			op:    MatchNotRegexp,
			value: `bar`,
			want:  `{"name":"foo","value":"bar","isRegex":true,"isEqual":false}`,
		},
	}

	cmp := func(m1, m2 Matcher) bool {
		return m1.Name == m2.Name && m1.Value == m2.Value && m1.Type == m2.Type
	}

	for _, test := range tests {
		m, err := NewMatcher(test.op, test.name, test.value)
		if err != nil {
			t.Fatal(err)
		}

		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(b); got != test.want {
			t.Errorf("Unexpected JSON representation of matcher:\nwant:\t%v\ngot:\t%v", test.want, got)
		}

		var m2 Matcher
		if err := json.Unmarshal(b, &m2); err != nil {
			t.Fatal(err)
		}
		if !cmp(*m, m2) {
			t.Errorf("Doing Marshal and Unmarshal seems to be losing data; before %#v, after %#v", m, m2)
		}
	}
}

func TestMatcherJSONUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		op    MatchType
		value string
		want  string
	}{
		{
			name:  "foo",
			op:    MatchEqual,
			value: "bar",
			want:  `{"name":"foo","value":"bar","isRegex":false}`,
		},
		{
			name:  `foo`,
			op:    MatchEqual,
			value: `bar`,
			want:  `{"name":"foo","value":"bar","isRegex":false,"isEqual":true}`,
		},
		{
			name:  `foo`,
			op:    MatchNotEqual,
			value: `bar`,
			want:  `{"name":"foo","value":"bar","isRegex":false,"isEqual":false}`,
		},
		{
			name:  `foo`,
			op:    MatchRegexp,
			value: `bar`,
			want:  `{"name":"foo","value":"bar","isRegex":true,"isEqual":true}`,
		},
		{
			name:  `foo`,
			op:    MatchNotRegexp,
			value: `bar`,
			want:  `{"name":"foo","value":"bar","isRegex":true,"isEqual":false}`,
		},
	}

	cmp := func(m1, m2 Matcher) bool {
		return m1.Name == m2.Name && m1.Value == m2.Value && m1.Type == m2.Type
	}

	for _, test := range tests {
		var m Matcher
		if err := json.Unmarshal([]byte(test.want), &m); err != nil {
			t.Fatal(err)
		}

		m2, err := NewMatcher(test.op, test.name, test.value)
		if err != nil {
			t.Fatal(err)
		}

		if !cmp(m, *m2) {
			t.Errorf("Unmarshaling seems to be producing unexpected matchers; got %#v, expected %#v", m, m2)
		}
	}
}
