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

	for _, toParse := range strings.Split(s, ",") {
		m, err := Matcher(toParse)
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
		return "", "", labels.MatchEqual, fmt.Errorf("bad matcher format")
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
