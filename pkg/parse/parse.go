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
	"fmt"
	"regexp"
	"strings"

	"github.com/prometheus/prometheus/pkg/labels"
)

var (
	re      = regexp.MustCompile(`(?:\s?)(\w+)(=|=~|!=|!~)(?:\"([^"=~!]+)\"|([^"=~!]+)|\"\")`)
	typeMap = map[string]labels.MatchType{
		"=":  labels.MatchEqual,
		"!=": labels.MatchNotEqual,
		"=~": labels.MatchRegexp,
		"!~": labels.MatchNotRegexp,
	}
)

func Matchers(s string) ([]*labels.Matcher, error) {
	matchers := []*labels.Matcher{}
	if strings.HasPrefix(s, "{") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "}") {
		s = s[:len(s)-1]
	}

	var insideQuotes bool
	var token string
	var tokens []string
	for _, r := range s {
		if !insideQuotes && r == ',' {
			tokens = append(tokens, token)
			token = ""
			continue
		}
		token += string(r)
		if r == '"' {
			insideQuotes = !insideQuotes
		}
	}
	if token != "" {
		tokens = append(tokens, token)
	}
	for _, token := range tokens {
		m, err := Matcher(token)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, m)
	}

	return matchers, nil
}

func Matcher(s string) (*labels.Matcher, error) {
	name, value, matchType, err := Input(s)
	if err != nil {
		return nil, err
	}

	m, err := labels.NewMatcher(matchType, name, value)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func Input(s string) (name, value string, matchType labels.MatchType, err error) {
	ms := re.FindStringSubmatch(s)
	if len(ms) < 4 {
		return "", "", labels.MatchEqual, fmt.Errorf("bad matcher format: %s", s)
	}

	var prs bool
	name = ms[1]
	matchType, prs = typeMap[ms[2]]

	if ms[3] != "" {
		value = ms[3]
	} else {
		value = ms[4]
	}

	if name == "" || !prs {
		return "", "", labels.MatchEqual, fmt.Errorf("failed to parse")
	}

	return name, value, matchType, nil
}
