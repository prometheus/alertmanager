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

package adapter

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/alertmanager/matchers/parse"
	"github.com/prometheus/alertmanager/pkg/labels"
)

var (
	// ParseMatcher is the default parser for parsing individual matchers.
	ParseMatcher = FallbackMatcherParser(log.NewNopLogger())

	// ParseMatchers is the default parser for parsing a series of zero or
	// more matchers.
	ParseMatchers = FallbackMatchersParser(log.NewNopLogger())
)

// MatcherParser is an interface for parsing individual matchers.
type MatcherParser func(s string) (*labels.Matcher, error)

// MatchersParser is an interface for parsing a series of zero or more matchers.
type MatchersParser func(s string) (labels.Matchers, error)

// OldMatcherParser uses the old pkg/labels parser to parse the matcher in
// the input string.
func OldMatcherParser(l log.Logger) MatcherParser {
	return func(s string) (*labels.Matcher, error) {
		level.Debug(l).Log(
			"msg",
			"Parsing matcher with old regular expressions parser",
			"matcher",
			s,
		)
		return labels.ParseMatcher(s)
	}
}

// OldMatchersParser uses the old pkg/labels parser to parse zero or more
// matchers in the string.  It returns an error if the input is invalid.
func OldMatchersParser(l log.Logger) MatchersParser {
	return func(s string) (labels.Matchers, error) {
		level.Debug(l).Log(
			"msg",
			"Parsing matchers with old regular expressions parser",
			"matchers",
			s)
		return labels.ParseMatchers(s)
	}
}

// FallbackMatchersParser uses the new matchers/parse parser to parse the
// matcher in the input string. If this fails it falls back to the old
// pkg/labels parser and emits a warning log line.
func FallbackMatchersParser(l log.Logger) MatchersParser {
	return func(s string) (labels.Matchers, error) {
		var (
			m          []*labels.Matcher
			err        error
			invalidErr error
		)
		level.Debug(l).Log(
			"msg",
			"Parsing matchers with new parser",
			"matchers",
			s,
		)
		m, err = parse.Parse(s)
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
				"Failed to parse input with matchers/parse, falling back to pkg/labels parser",
				"matchers",
				s,
				"err",
				err,
			)
		}
		return m, nil
	}
}

// FallbackMatcherParser uses the new matchers/parse parser to parse
// zero or more matchers in the string. If this fails it falls back to
// the old pkg/labels parser and emits a warning log line.
func FallbackMatcherParser(l log.Logger) MatcherParser {
	return func(s string) (*labels.Matcher, error) {
		var (
			m          *labels.Matcher
			err        error
			invalidErr error
		)
		level.Debug(l).Log(
			"msg",
			"Parsing matcher with new parser",
			"matcher",
			s,
		)
		m, err = parse.ParseMatcher(s)
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
