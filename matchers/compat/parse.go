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
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matchers/parse"
	"github.com/prometheus/alertmanager/pkg/labels"
)

var (
	parseMatcher  = stableMatcherParser(log.NewNopLogger())
	parseMatchers = stableMatchersParser(log.NewNopLogger())
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
	if f.StableMatchersParsing() {
		parseMatcher = stableMatcherParser(l)
		parseMatchers = stableMatchersParser(l)
	} else if f.UTF8MatchersParsing() {
		parseMatcher = utf8MatcherParser(l)
		parseMatchers = utf8MatchersParser(l)
	} else {
		parseMatcher = fallbackMatcherParser(l)
		parseMatchers = fallbackMatchersParser(l)
	}
}

// stableMatcherParser uses the old pkg/labels parser to parse the matcher in
// the input string.
func stableMatcherParser(_ log.Logger) matcherParser {
	return func(s string) (*labels.Matcher, error) {
		return labels.ParseMatcher(s)
	}
}

// stableMatchersParser uses the old pkg/labels parser to parse zero or more
// matchers in the input string. It returns an error if the input is invalid.
func stableMatchersParser(_ log.Logger) matchersParser {
	return func(s string) (labels.Matchers, error) {
		return labels.ParseMatchers(s)
	}
}

// utf8MatcherParser uses the new matchers/parse parser to parse
// the matcher in the input string. If this fails it does not fallback
// to the old pkg/labels parser.
func utf8MatcherParser(_ log.Logger) matcherParser {
	return func(s string) (*labels.Matcher, error) {
		return labels.ParseMatcher(s)
	}
}

// utf8MatchersParser uses the new matchers/parse parser to parse
// zero or more matchers in the input string. If this fails it
// does not fallback to the old pkg/labels parser.
func utf8MatchersParser(_ log.Logger) matchersParser {
	return func(s string) (labels.Matchers, error) {
		return labels.ParseMatchers(s)
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
		m, err = parse.Matcher(s)
		if err != nil {
			// The input is not valid in the old pkg/labels parser either,
			// it cannot be valid input.
			m, invalidErr = labels.ParseMatcher(s)
			if invalidErr != nil {
				return nil, invalidErr
			}
			// The input is valid in the old pkg/labels parser, but not the
			// new matchers/parse parser.
			level.Warn(l).Log(
				"msg",
				"Failed to parse input with matchers/parse, falling back to pkg/labels parser",
				"matcher",
				s,
				"err",
				err,
			)
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
		m, err = parse.Matchers(s)
		if err != nil {
			// The input is not valid in the old pkg/labels parser either,
			// it cannot be valid input.
			m, invalidErr = labels.ParseMatchers(s)
			if invalidErr != nil {
				return nil, invalidErr
			}
			// The input is valid in the old pkg/labels parser, but not the
			// new matchers/parse parser.
			level.Warn(l).Log(
				"msg",
				"Alertmanager is moving to a new parser for label matchers, and this input is incompatible. Please make sure all regular expressions and values are double-quoted. If you are still seeing this message please open an issue.",
				"matchers",
				s,
				"err",
				err,
			)
		}
		return m, nil
	}
}
