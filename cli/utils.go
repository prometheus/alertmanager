package cli

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/viper"

	"github.com/prometheus/alertmanager/types"
)

func GetAlertmanagerURL() (*url.URL, error) {
	u, err := url.ParseRequestURI(viper.GetString("alertmanager.url"))
	if err != nil {
		return nil, errors.New("Invalid alertmanager url")
	}
	return u, nil
}

// Parse a list of labels (cli arguments)
func parseMatchers(labels []string) (types.Matchers, error) {
	matchers := make([]*types.Matcher, 0)

	for _, v := range labels {
		var sep string
		isRegex, err := regexp.MatchString(".+=~.+", v)
		if err != nil {
			return types.Matchers{}, err
		}
		if isRegex {
			sep = "=~"
		} else {
			sep = "="
		}
		labelVec := strings.SplitN(v, sep, 2)
		// Assume that no = was given and just use alertname
		var label, value string
		if len(labelVec) < 2 {
			label, value = "alertname", labelVec[0]
		} else {
			label, value = labelVec[0], labelVec[1]
		}

		var matcher types.Matcher
		if isRegex {
			matcher = types.Matcher{
				Name:    label,
				Value:   cleanupRegex(value),
				IsRegex: isRegex,
			}
		} else {
			matcher = types.Matcher{
				Name:    label,
				Value:   value,
				IsRegex: isRegex,
			}
		}

		err = matcher.Validate()
		if err != nil {
			return nil, err
		}

		// Init needs to be called before running match. Does NOT need to be run before validate
		err = matcher.Init()
		if err != nil {
			return nil, err
		}

		matchers = append(matchers, &matcher)
	}

	return types.NewMatchers(matchers...), nil
}

// Expand a list of matchers into a list of all combinations of matchers for each matcher that has the same Name
// ```
// alertname=foo instance=bar instance=baz
// ```
// Goes to
// ```
// alertname=foo instance=bar
// alertname=foo instance=baz
// ```
func parseMatcherGroups(matchers types.Matchers) []types.Matchers {
	keyMap := make(map[string]types.Matchers)

	for i, kv := range matchers {
		if _, ok := keyMap[kv.Name]; !ok {
			keyMap[kv.Name] = make(types.Matchers, 0)
		}
		keyMap[kv.Name] = append(keyMap[kv.Name], matchers[i])
	}

	var output []types.Matchers
	for _, v := range keyMap {
		output = merge(output, v)
	}

	for _, group := range output {
		sort.Sort(group)
	}

	return output
}

// Determine if two matcher groups match
func groupMatch(silence, query types.Matchers) bool {
	for _, groupMatcher := range query {
		matches := false
		for _, silenceMatcher := range silence {
			if match(*groupMatcher, *silenceMatcher) {
				matches = true
				break
			}
		}
		if !matches {
			return false
		}
	}
	return true
}

// Determine if two matchers match the same things (roughly)
func match(this, that types.Matcher) bool {
	if this.Name != that.Name {
		return false
	}

	if this.IsRegex {
		matches, err := regexp.MatchString(this.Value, that.Value)
		if err != nil {
			return false
		}

		return matches
	} else if that.IsRegex {
		matches, err := regexp.MatchString(that.Value, this.Value)

		if err != nil {
			return false
		}
		return matches
	} else {
		return this.Value == that.Value
	}
}

// Thanks @vendemiat for help with this one
func merge(dst []types.Matchers, source types.Matchers) []types.Matchers {
	if len(dst) == 0 {
		for i := range source {
			dst = append(dst, types.Matchers{source[i]})
		}
		return dst
	}

	output := make([]types.Matchers, len(dst)*len(source))
	j := 0
	for i := range dst {
		for k := range source {
			output[j] = make(types.Matchers, len(dst[i]))
			if n := copy(output[j], dst[i]); n == 0 {
				fmt.Printf("copy failure\n")
			}
			output[j] = append(output[j], source[k])
			j += 1
		}
	}
	return output
}

// Provide sanity ^ and $ for all regex matchers if they are not already given
func cleanupRegex(regex string) string {
	var output []rune
	rune_slice := []rune(regex)
	first_rune := rune_slice[0]
	last_rune := rune_slice[len(rune_slice)-1]

	if first_rune != '^' {
		output = append(output, '^')
	}

	output = append(output, rune_slice...)

	if last_rune != '$' {
		output = append(output, '$')
	}
	return string(output)
}
