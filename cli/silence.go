package cli

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/prometheus/alertmanager/types"
	"github.com/spf13/cobra"
)

var labels []string

type alertmanagerSilenceResponse struct {
	Status    string          `json:"status"`
	Data      []types.Silence `json:"data,omitempty"`
	ErrorType string          `json:"errorType,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// silenceCmd represents the silence command
var silenceCmd = &cobra.Command{
	Use:   "silence",
	Short: "Manage silences",
	Long:  `Add, expire or view silences For more information and additional flags see query help`,
	RunE:  query,
}

func init() {
	RootCmd.AddCommand(silenceCmd)
	silenceCmd.AddCommand(addCmd)
	silenceCmd.AddCommand(expireCmd)
	silenceCmd.AddCommand(queryCmd)
}

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

// Determine of two matchers match the same things (roughly)
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

// Thanks @vicky for help with this one
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

func parseMatchers(labels []string) (types.Matchers, error) {
	matchers := make([]*types.Matcher, 0)

	for _, v := range labels {
		var sep string
		isRegex, err := regexp.MatchString(".+~=.+", v)
		if err != nil {
			return types.Matchers{}, err
		}
		if isRegex {
			sep = "~="
		} else {
			sep = "="
		}
		labelVec := strings.SplitN(v, sep, 2)
		if len(labelVec) != 2 {
			return nil, errors.New("Unable to parse match groups")
		}
		label, value := labelVec[0], labelVec[1]
		matcher := types.Matcher{
			Name:    label,
			Value:   value,
			IsRegex: isRegex,
		}
		err = matcher.Validate()
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, &matcher)
	}

	return types.NewMatchers(matchers...), nil
}
