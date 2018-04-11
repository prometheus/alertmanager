package cli

import (
	"fmt"
	"net/url"
	"path"

	"github.com/prometheus/alertmanager/pkg/parse"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
)

type ByAlphabetical []labels.Matcher

func (s ByAlphabetical) Len() int      { return len(s) }
func (s ByAlphabetical) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s ByAlphabetical) Less(i, j int) bool {
	if s[i].Name != s[j].Name {
		return s[i].Name < s[j].Name
	} else if s[i].Type != s[j].Type {
		return s[i].Type < s[j].Type
	} else if s[i].Value != s[j].Value {
		return s[i].Value < s[j].Value
	}
	return false
}

func GetAlertmanagerURL(p string) url.URL {
	amURL := *alertmanagerURL
	amURL.Path = path.Join(alertmanagerURL.Path, p)
	return amURL
}

// Parse a list of labels (cli arguments)
func parseMatchers(inputLabels []string) ([]labels.Matcher, error) {
	matchers := make([]labels.Matcher, 0)

	for _, v := range inputLabels {
		name, value, matchType, err := parse.Input(v)
		if err != nil {
			return []labels.Matcher{}, err
		}

		matchers = append(matchers, labels.Matcher{
			Type:  matchType,
			Name:  name,
			Value: value,
		})
	}

	return matchers, nil
}

// Only valid for when you are going to add a silence
func TypeMatchers(matchers []labels.Matcher) (types.Matchers, error) {
	typeMatchers := types.Matchers{}
	for _, matcher := range matchers {
		typeMatcher, err := TypeMatcher(matcher)
		if err != nil {
			return types.Matchers{}, err
		}
		typeMatchers = append(typeMatchers, &typeMatcher)
	}
	return typeMatchers, nil
}

// Only valid for when you are going to add a silence
// Doesn't allow negative operators
func TypeMatcher(matcher labels.Matcher) (types.Matcher, error) {
	typeMatcher := types.NewMatcher(model.LabelName(matcher.Name), matcher.Value)

	switch matcher.Type {
	case labels.MatchEqual:
		typeMatcher.IsRegex = false
	case labels.MatchRegexp:
		typeMatcher.IsRegex = true
	default:
		return types.Matcher{}, fmt.Errorf("invalid match type for creation operation: %s", matcher.Type)
	}
	return *typeMatcher, nil
}
