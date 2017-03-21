package cli

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/viper"

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
	} else {
		return false
	}
}

func GetAlertmanagerURL() (*url.URL, error) {
	u, err := url.ParseRequestURI(viper.GetString("alertmanager.url"))
	if err != nil {
		return nil, errors.New("Invalid alertmanager url")
	}
	return u, nil
}

// Parse a list of labels (cli arguments)
func parseMatchers(inputLabels []string) ([]labels.Matcher, error) {
	matchers := make([]labels.Matcher, 0)

	for _, v := range inputLabels {
		fmt.Println(v)
		matcher, err := parse.Matchers(v)
		if err != nil {
			return []labels.Matcher{}, err
		}
		for _, item := range matcher {
			matchers = append(matchers, *item)
		}
	}

	return matchers, nil
}

// Expand a list of matchers into a list of all combinations of matchers for each matcher that has the same Name
// ```
// alertname=foo instance=bar instance=baz
// ```
// Goes t
// ```
// alertname=foo instance=bar
// alertname=foo instance=baz
// ```
func parseMatcherGroups(matchers []labels.Matcher) [][]labels.Matcher {
	keyMap := make(map[string][]labels.Matcher)

	for i, kv := range matchers {
		if _, ok := keyMap[kv.Name]; !ok {
			keyMap[kv.Name] = make([]labels.Matcher, 0)
		}
		keyMap[kv.Name] = append(keyMap[kv.Name], matchers[i])
	}

	var output [][]labels.Matcher
	for _, v := range keyMap {
		output = merge(output, v)
	}

	for _, group := range output {
		sort.Sort(ByAlphabetical(group))
	}

	return output
}

// Thanks @vendemiat for help with this one
func merge(dst [][]labels.Matcher, source []labels.Matcher) [][]labels.Matcher {
	if len(dst) == 0 {
		for i := range source {
			dst = append(dst, []labels.Matcher{source[i]})
		}
		return dst
	}

	output := make([][]labels.Matcher, len(dst)*len(source))
	j := 0
	for i := range dst {
		for k := range source {
			output[j] = make([]labels.Matcher, len(dst[i]))
			if n := copy(output[j], dst[i]); n == 0 {
				fmt.Printf("copy failure\n")
			}
			output[j] = append(output[j], source[k])
			j += 1
		}
	}
	return output
}

func MatchersToString(matchers []labels.Matcher) string {
	stringMatchers := make([]string, len(matchers))
	for i, v := range matchers {
		stringMatchers[i] = v.String()
		fmt.Println(v.String())
	}
	return fmt.Sprintf("{%s}", strings.Join(stringMatchers, ", "))
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
	var typeMatcher types.Matcher

	switch matcher.Type {
	case labels.MatchEqual:
		typeMatcher = *types.NewMatcher(model.LabelName(matcher.Name), matcher.Value)
	case labels.MatchRegexp:
		typeMatcher = *types.NewRegexMatcher(model.LabelName(matcher.Name), matcher.Regexp())
	default:
		return types.Matcher{}, errors.New("Invalid match type for creation operation")
	}
	return typeMatcher, nil
}
