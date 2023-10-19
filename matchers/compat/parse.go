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

package compat

import (
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matchers/parse"
	"github.com/prometheus/alertmanager/pkg/labels"
)

var (
	parseMatcher  = classicMatcherParser(log.NewNopLogger())
	parseMatchers = classicMatchersParser(log.NewNopLogger())
)

type matcherParser func(s string) (*labels.Matcher, error)

type matchersParser func(s string) (labels.Matchers, error)

// Matcher parses the matcher in the input string. It returns an error
// if the input is invalid or contains two or more matchers.
func Matcher(s string) (*labels.Matcher, error) {
	return parseMatcher(s)
}

// Matchers parses one or more matchers in the input string. It returns
// an error if the input is invalid.
func Matchers(s string) (labels.Matchers, error) {
	return parseMatchers(s)
}

// InitFromFlags initializes the compat package from the flagger.
func InitFromFlags(l log.Logger, f featurecontrol.Flagger) {
	if f.ClassicMatchersParsing() {
		parseMatcher = classicMatcherParser(l)
		parseMatchers = classicMatchersParser(l)
	} else if f.UTF8MatchersParsing() {
		parseMatcher = utf8MatcherParser(l)
		parseMatchers = utf8MatchersParser(l)
	} else {
		parseMatcher = fallbackMatcherParser(l)
		parseMatchers = fallbackMatchersParser(l)
	}
}

// classicMatcherParser uses the old pkg/labels parser to parse the matcher in
// the input string.
func classicMatcherParser(l log.Logger) matcherParser {
	return func(s string) (*labels.Matcher, error) {
		level.Debug(l).Log("msg", "Parsing with classic matchers parser", "input", s)
		return labels.ParseMatcher(s)
	}
}

// classicMatchersParser uses the old pkg/labels parser to parse zero or more
// matchers in the input string. It returns an error if the input is invalid.
func classicMatchersParser(l log.Logger) matchersParser {
	return func(s string) (labels.Matchers, error) {
		level.Debug(l).Log("msg", "Parsing with classic matchers parser", "input", s)
		return labels.ParseMatchers(s)
	}
}

// utf8MatcherParser uses the new matchers/parse parser to parse
// the matcher in the input string. If this fails it does not fallback
// to the old pkg/labels parser.
func utf8MatcherParser(l log.Logger) matcherParser {
	return func(s string) (*labels.Matcher, error) {
		level.Debug(l).Log("msg", "Parsing with UTF-8 matchers parser", "input", s)
		if strings.HasPrefix(s, "{") || strings.HasSuffix(s, "}") {
			return nil, fmt.Errorf("unexpected open or close brace: %s", s)
		}
		return parse.Matcher(s)
	}
}

// utf8MatchersParser uses the new matchers/parse parser to parse
// zero or more matchers in the input string. If this fails it
// does not fallback to the old pkg/labels parser.
func utf8MatchersParser(l log.Logger) matchersParser {
	return func(s string) (labels.Matchers, error) {
		level.Debug(l).Log("msg", "Parsing with UTF-8 matchers parser", "input", s)
		return parse.Matchers(s)
	}
}

// fallbackMatcherParser uses the new matchers/parse parser to parse
// zero or more matchers in the string. If this fails it falls back to
// the old pkg/labels parser and emits a warning log line.
func fallbackMatcherParser(l log.Logger) matcherParser {
	return func(s string) (*labels.Matcher, error) {
		var (
			m          *labels.Matcher
			err        error
			invalidErr error
		)
		level.Debug(l).Log("msg", "Parsing with UTF-8 matchers parser, with fallback to classic matchers parser", "input", s)
		if strings.HasPrefix(s, "{") || strings.HasSuffix(s, "}") {
			return nil, fmt.Errorf("unexpected open or close brace: %s", s)
		}
		m, err = parse.Matcher(s)
		if err != nil {
			m, invalidErr = labels.ParseMatcher(s)
			if invalidErr != nil {
				// The input is not valid in the old pkg/labels parser either,
				// it cannot be valid input.
				return nil, invalidErr
			}
			// The input is valid in the old pkg/labels parser, but not the
			// new matchers/parse parser.
			suggestion := m.String()
			level.Warn(l).Log("msg", "Alertmanager is moving to a new parser for labels and matchers, and this input is incompatible. Alertmanager has instead parsed the input using the old matchers parser as a fallback. To make this input compatible with the new parser please make sure all regular expressions and values are double-quoted. If you are still seeing this message please open an issue.", "input", s, "err", err, "suggestion", suggestion)
		}
		return m, nil
	}
}

// fallbackMatchersParser uses the new matchers/parse parser to parse the
// matcher in the input string. If this fails it falls back to the old
// pkg/labels parser and emits a warning log line.
func fallbackMatchersParser(l log.Logger) matchersParser {
	return func(s string) (labels.Matchers, error) {
		var (
			m          []*labels.Matcher
			err        error
			invalidErr error
		)
		level.Debug(l).Log("msg", "Parsing with UTF-8 matchers parser, with fallback to classic matchers parser", "input", s)
		m, err = parse.Matchers(s)
		if err != nil {
			m, invalidErr = labels.ParseMatchers(s)
			if invalidErr != nil {
				// The input is not valid in the old pkg/labels parser either,
				// it cannot be valid input.
				return nil, invalidErr
			}
			var sb strings.Builder
			sb.WriteRune('{')
			for i, n := range m {
				sb.WriteString(n.String())
				if i < len(m)-1 {
					sb.WriteRune(',')
				}
			}
			sb.WriteRune('}')
			suggestion := sb.String()
			// The input is valid in the old pkg/labels parser, but not the
			// new matchers/parse parser.
			level.Warn(l).Log("msg", "Alertmanager is moving to a new parser for labels and matchers, and this input is incompatible. Alertmanager has instead parsed the input using the old matchers parser as a fallback. To make this input compatible with the new parser please make sure all regular expressions and values are double-quoted. If you are still seeing this message please open an issue.", "input", s, "err", err, "suggestion", suggestion)
		}
		return m, nil
	}
}
