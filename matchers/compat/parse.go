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
	"unicode/utf8"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matchers/parse"
	"github.com/prometheus/alertmanager/pkg/labels"
)

var (
	isValidLabelName = isValidClassicLabelName(log.NewNopLogger())
	parseMatcher     = ClassicMatcherParser(log.NewNopLogger())
	parseMatchers    = ClassicMatchersParser(log.NewNopLogger())
)

// IsValidLabelName returns true if the string is a valid label name.
func IsValidLabelName(name model.LabelName) bool {
	return isValidLabelName(name)
}

type ParseMatcher func(s string) (*labels.Matcher, error)

type ParseMatchers func(s string) (labels.Matchers, error)

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
	if f.ClassicMode() {
		isValidLabelName = isValidClassicLabelName(l)
		parseMatcher = ClassicMatcherParser(l)
		parseMatchers = ClassicMatchersParser(l)
	} else if f.UTF8StrictMode() {
		isValidLabelName = isValidUTF8LabelName(l)
		parseMatcher = UTF8MatcherParser(l)
		parseMatchers = UTF8MatchersParser(l)
	} else {
		isValidLabelName = isValidUTF8LabelName(l)
		parseMatcher = FallbackMatcherParser(l)
		parseMatchers = FallbackMatchersParser(l)
	}
}

// ClassicMatcherParser uses the old pkg/labels parser to parse the matcher in
// the input string.
func ClassicMatcherParser(l log.Logger) ParseMatcher {
	return func(s string) (*labels.Matcher, error) {
		level.Debug(l).Log("msg", "Parsing with classic matchers parser", "input", s)
		return labels.ParseMatcher(s)
	}
}

// ClassicMatchersParser uses the old pkg/labels parser to parse zero or more
// matchers in the input string. It returns an error if the input is invalid.
func ClassicMatchersParser(l log.Logger) ParseMatchers {
	return func(s string) (labels.Matchers, error) {
		level.Debug(l).Log("msg", "Parsing with classic matchers parser", "input", s)
		return labels.ParseMatchers(s)
	}
}

// UTF8MatcherParser uses the new matchers/parse parser to parse
// the matcher in the input string. If this fails it does not fallback
// to the old pkg/labels parser.
func UTF8MatcherParser(l log.Logger) ParseMatcher {
	return func(s string) (*labels.Matcher, error) {
		level.Debug(l).Log("msg", "Parsing with UTF-8 matchers parser", "input", s)
		if strings.HasPrefix(s, "{") || strings.HasSuffix(s, "}") {
			return nil, fmt.Errorf("unexpected open or close brace: %s", s)
		}
		return parse.Matcher(s)
	}
}

// UTF8MatchersParser uses the new matchers/parse parser to parse
// zero or more matchers in the input string. If this fails it
// does not fallback to the old pkg/labels parser.
func UTF8MatchersParser(l log.Logger) ParseMatchers {
	return func(s string) (labels.Matchers, error) {
		level.Debug(l).Log("msg", "Parsing with UTF-8 matchers parser", "input", s)
		return parse.Matchers(s)
	}
}

// FallbackMatcherParser uses the new matchers/parse parser to parse
// zero or more matchers in the string. If this fails it falls back to
// the old pkg/labels parser and emits a warning log line.
func FallbackMatcherParser(l log.Logger) ParseMatcher {
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

// FallbackMatchersParser uses the new matchers/parse parser to parse the
// matcher in the input string. If this fails it falls back to the old
// pkg/labels parser and emits a warning log line.
func FallbackMatchersParser(l log.Logger) ParseMatchers {
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
			for i, n := range m {
				sb.WriteString(n.String())
				if i < len(m)-1 {
					sb.WriteRune(',')
				}
			}
			suggestion := sb.String()
			// The input is valid in the old pkg/labels parser, but not the
			// new matchers/parse parser.
			level.Warn(l).Log("msg", "Alertmanager is moving to a new parser for labels and matchers, and this input is incompatible. Alertmanager has instead parsed the input using the old matchers parser as a fallback. To make this input compatible with the new parser please make sure all regular expressions and values are double-quoted. If you are still seeing this message please open an issue.", "input", s, "err", err, "suggestion", suggestion)
		}
		return m, nil
	}
}

// isValidClassicLabelName returns true if the string is a valid classic label name.
func isValidClassicLabelName(_ log.Logger) func(model.LabelName) bool {
	return func(name model.LabelName) bool {
		return name.IsValid()
	}
}

// isValidUTF8LabelName returns true if the string is a valid UTF-8 label name.
func isValidUTF8LabelName(_ log.Logger) func(model.LabelName) bool {
	return func(name model.LabelName) bool {
		return utf8.ValidString(string(name))
	}
}
