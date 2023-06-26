// Like ParseMatchers, its
// purpose is to allow the Alertmanager project to transition from the
// old regular expression parser to the new LL(1) parser while giving users
// the choice to disable it if required.

package adapter

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/alertmanager/matchers"
	"github.com/prometheus/alertmanager/matchers/old_parse"
	"github.com/prometheus/alertmanager/matchers/parse"
)

var (
	// ParseMatcher is the default parser for parsing individual matchers.
	ParseMatcher = OldMatcherParser(log.NewNopLogger())

	// ParseMatchers is the default parser for parsing a series of zero or
	// more matchers.
	ParseMatchers = OldMatchersParser(log.NewNopLogger())
)

// MatcherParser is an interface for parsing individual matchers.
type MatcherParser func(s string) (*matchers.Matcher, error)

// MatchersParser is an interface for parsing a series of zero or more matchers.
type MatchersParser func(s string) (matchers.Matchers, error)

func OldMatcherParser(l log.Logger) MatcherParser {
	return func(s string) (*matchers.Matcher, error) {
		level.Debug(l).Log(
			"msg",
			"Parsing matcher with old regular expressions parser",
			"matcher",
			s,
		)
		return old_parse.ParseMatcher(s)
	}
}

func OldMatchersParser(l log.Logger) MatchersParser {
	return func(s string) (matchers.Matchers, error) {
		level.Debug(l).Log(
			"msg",
			"Parsing matchers with old regular expressions parser",
			"matchers",
			s)
		return old_parse.ParseMatchers(s)
	}
}

func FallbackMatchersParser(l log.Logger) MatchersParser {
	return func(s string) (matchers.Matchers, error) {
		level.Debug(l).Log(
			"msg",
			"Parsing matchers with new parser",
			"matchers",
			s,
		)
		m, err := parse.Parse(s)
		if err != nil {
			level.Warn(l).Log(
				"msg",
				"Failed to parse matchers, falling back to old regular expressions parser",
				"matchers",
				s,
				"err",
				err,
			)
			return old_parse.ParseMatchers(s)
		}
		return m, nil
	}
}

func FallbackMatcherParser(l log.Logger) MatcherParser {
	return func(s string) (*matchers.Matcher, error) {
		level.Debug(l).Log(
			"msg",
			"Parsing matcher with new parser",
			"matcher",
			s,
		)
		m, err := parse.ParseMatcher(s)
		if err != nil {
			level.Warn(l).Log(
				"msg",
				"Failed to parse matcher, falling back to old regular expressions parser",
				"matcher",
				s,
				"err",
				err,
			)
			return old_parse.ParseMatcher(s)
		}
		return m, nil
	}
}
