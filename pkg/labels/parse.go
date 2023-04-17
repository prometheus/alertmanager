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

package labels

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
)

var (
	// '=~' has to come before '=' because otherwise only the '='
	// will be consumed, and the '~' will be part of the 3rd token.
	// only prometheus-compatible labels are allowed to be without wrapping double quotes.
	re      = regexp.MustCompile(`^\s*((?s)".+"|[a-zA-Z_:][a-zA-Z0-9_:]*)\s*(=~|=|!=|!~)\s*((?s).*?)\s*$`)
	typeMap = map[string]MatchType{
		"=":  MatchEqual,
		"!=": MatchNotEqual,
		"=~": MatchRegexp,
		"!~": MatchNotRegexp,
	}
)

// ParseMatchers parses a comma-separated list of Matchers. A leading '{' and/or
// a trailing '}' is optional and will be trimmed before further
// parsing. Individual Matchers are separated by commas outside of quoted parts
// of the input string. Those commas may be surrounded by whitespace. Parts of the
// string inside unescaped double quotes ('"…"') are considered quoted (and
// commas don't act as separators there). If double quotes are escaped with a
// single backslash ('\"'), they are ignored for the purpose of identifying
// quoted parts of the input string. If the input string, after trimming the
// optional trailing '}', ends with a comma, followed by optional whitespace,
// this comma and whitespace will be trimmed.
//
// Examples for valid input strings:
//
//	{foo = "bar", dings != "bums", }
//	foo=bar,dings!=bums
//	foo=bar, dings!=bums
//	{quote="She said: \"Hi, ladies! That's gender-neutral…\""}
//	statuscode=~"5.."
//
// See ParseMatcher for details on how an individual Matcher is parsed.
func ParseMatchers(s string) ([]*Matcher, error) {
	matchers := []*Matcher{}
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")

	var (
		insideQuotes bool
		escaped      bool
		token        strings.Builder
		tokens       []string
	)
	for _, r := range s {
		switch r {
		case ',':
			if !insideQuotes {
				tokens = append(tokens, token.String())
				token.Reset()
				continue
			}
		case '"':
			if !escaped {
				insideQuotes = !insideQuotes
			} else {
				escaped = false
			}
		case '\\':
			escaped = !escaped
		default:
			escaped = false
		}
		token.WriteRune(r)
	}
	if s := strings.TrimSpace(token.String()); s != "" {
		tokens = append(tokens, s)
	}
	for _, token := range tokens {
		m, err := ParseMatcher(token)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, m)
	}

	return matchers, nil
}

// ParseMatcher parses a matcher with a syntax inspired by PromQL and
// OpenMetrics. This syntax is convenient to describe filters and selectors in
// UIs and config files. To support the interactive nature of the use cases, the
// parser is in various aspects fairly tolerant.
//
// The syntax of a matcher consists of three tokens: (1) Either a UTF-8 string
// enclosed in double quotes or a valid Prometheus label name.
// (2) One of '=', '!=', '=~', or '!~', with the same meaning as
// known from PromQL selectors. (3) A UTF-8 string, which may be enclosed in
// double quotes. Before or after each token, there may be any amount of
// whitespace, which will be discarded. The 3rd token may be the empty
// string. Within the 3rd token, OpenMetrics escaping rules apply: '\"' for a
// double-quote, '\n' for a line feed, '\\' for a literal backslash. Unescaped
// '"' must not occur inside the 3rd token (only as the 1st or last
// character). However, literal line feed characters are tolerated, as are
// single '\' characters not followed by '\', 'n', or '"'. They act as a literal
// backslash in that case.
func ParseMatcher(s string) (_ *Matcher, err error) {
	ms := re.FindStringSubmatch(s)
	if len(ms) == 0 {
		return nil, errors.Errorf("bad matcher format: %s", s)
	}

	var (
		rawName  = ms[1]
		rawValue = ms[3]
	)

	matcherType, ok := typeMap[ms[2]]
	if !ok {
		return nil, errors.Errorf("matcher type is unknown: %s", ms[2])
	}

	if !utf8.ValidString(rawName) {
		return nil, errors.Errorf("matcher label not valid UTF-8: %s", rawName)
	}
	if !utf8.ValidString(rawValue) {
		return nil, errors.Errorf("matcher value not valid UTF-8: %s", rawValue)
	}

	value, err := unescapeMatcherString(rawValue)
	if err != nil {
		return nil, fmt.Errorf("matcher value %w", err)
	}

	name := rawName
	// if name is quoted, then it can contain any UTF-8 character. Unescape some escape sequences.
	if strings.HasPrefix(rawName, `"`) {
		name, err = unescapeMatcherString(rawName)
		if err != nil {
			return nil, fmt.Errorf("matcher label name %w", err)
		}
	}

	if !IsValidName(model.LabelName(name)) {
		return nil, errors.Errorf("invalid matcher label name: \"%s\"", name)
	}

	return NewMatcher(matcherType, name, value)
}

// unescapeMatcherString unescapes sequences that are escaped by openMetricsEscape: \n, \\, \", and removes leading and trailing double quotes.
func unescapeMatcherString(raw string) (string, error) {
	var (
		escaped             bool
		value               strings.Builder
		rawValue            = raw
		expectTrailingQuote = false
	)
	if strings.HasPrefix(rawValue, `"`) {
		rawValue = rawValue[1:]
		expectTrailingQuote = true
	}
	// Unescape the rawValue:
	for i, r := range rawValue {
		if escaped {
			escaped = false
			switch r {
			case 'n':
				value.WriteByte('\n')
			case '"', '\\':
				value.WriteRune(r)
			default:
				// This was a spurious escape, so treat the '\' as literal.
				value.WriteByte('\\')
				value.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\\':
			if i < len(rawValue)-1 {
				escaped = true
				continue
			}
			// '\' encountered as last byte. Treat it as literal.
			value.WriteByte('\\')
		case '"':
			if !expectTrailingQuote || i < len(rawValue)-1 {
				return "", errors.Errorf("contains unescaped double quote: %s", raw)
			}
			expectTrailingQuote = false
		default:
			value.WriteRune(r)
		}
	}
	if expectTrailingQuote {
		return "", errors.Errorf("contains unescaped double quote: %s", raw)
	}
	return value.String(), nil
}
