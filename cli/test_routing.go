// Copyright 2018 Prometheus Team
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

package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/common/model"
	"github.com/xlab/treeprint"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/matcher/compat"
)

const routingTestHelp = `Test alert routing

Will return receiver names which the alert with given labels resolves to.
If the labelset resolves to multiple receivers, they are printed out in order as defined in the routing tree.

Routing is loaded from a local configuration file or a running Alertmanager configuration.
Specifying --config.file takes precedence over --alertmanager.url.

Example:

./amtool config routes test --config.file=doc/examples/simple.yml --verify.receivers=team-DB-pager service=database

`

func configureRoutingTestCmd(cc *kingpin.CmdClause, c *routingShow) {
	routingTestCmd := cc.Command("test", routingTestHelp)

	routingTestCmd.Flag("verify.receivers", "Checks if specified receivers matches resolved receivers.").StringVar(&c.expectedReceivers)
	routingTestCmd.Flag("verify.receivers-grouping", "Checks if specified receivers and their grouping match resolved values. Format: receiver1[group1,group2],receiver2[group3]").StringVar(&c.expectedReceiversGroup)
	routingTestCmd.Flag("tree", "Prints out matching routes tree.").BoolVar(&c.debugTree)
	routingTestCmd.Arg("labels", "List of labels to be tested against the configured routes.").StringsVar(&c.labels)
	routingTestCmd.Action(execWithTimeout(c.routingTestAction))
}

// resolveAlertReceivers returns list of receiver names which given LabelSet resolves to.
func resolveAlertReceivers(mainRoute *dispatch.Route, labels *models.LabelSet) ([]string, error) {
	var (
		finalRoutes []*dispatch.Route
		receivers   []string
	)

	finalRoutes = mainRoute.Match(convertClientToCommonLabelSet(*labels))
	for _, r := range finalRoutes {
		receivers = append(receivers, r.RouteOpts.Receiver)
	}

	return receivers, nil
}

func printMatchingTree(mainRoute *dispatch.Route, ls models.LabelSet) {
	tree := treeprint.New()
	getMatchingTree(mainRoute, tree, ls)
	fmt.Println("Matching routes:")
	fmt.Println(tree.String())
	fmt.Print("\n")
}

// parseReceiversWithGrouping parses the input string and returns a map of receiver names to their expected groupings.
func parseReceiversWithGrouping(input string) (map[string][]string, error) {
	result := make(map[string][]string)
	receiverCounts := make(map[string]int)

	// Split by comma, but respect brackets
	receivers := splitWithBrackets(input)

	for _, r := range receivers {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}

		// Check if this receiver has explicit grouping
		parts := strings.SplitN(r, "=", 2)
		receiverName := strings.TrimSpace(parts[0])
		var cleanGroups []string

		if len(parts) == 1 {
			// No explicit grouping specified - treat as [...].
			cleanGroups = []string{"..."}
		} else {
			groupingPart := strings.TrimSpace(parts[1])
			if !strings.HasPrefix(groupingPart, "[") || !strings.HasSuffix(groupingPart, "]") {
				return nil, fmt.Errorf("invalid grouping format in %q, expected [group1,group2] or [...] after =", r)
			}

			// Extract and clean up group names
			grouping := strings.TrimSuffix(strings.TrimPrefix(groupingPart, "["), "]")
			if grouping == "" || grouping == "..." {
				cleanGroups = []string{"..."}
			} else {
				groups := strings.Split(grouping, ",")
				cleanGroups = make([]string, 0, len(groups))
				for _, g := range groups {
					g = strings.TrimSpace(g)
					if g != "" {
						cleanGroups = append(cleanGroups, g)
					}
				}
			}
		}

		// Handle duplicate receivers by adding a suffix
		count := receiverCounts[receiverName]
		receiverCounts[receiverName]++

		if count > 0 {
			receiverName = fmt.Sprintf("%s_%d", receiverName, count)
		}

		result[receiverName] = cleanGroups
	}

	return result, nil
}

// splitWithBrackets splits a string by commas while respecting brackets.
func splitWithBrackets(input string) []string {
	var result []string
	var current strings.Builder
	bracketCount := 0

	for _, char := range input {
		switch char {
		case '[':
			bracketCount++
			current.WriteRune(char)

		case ']':
			bracketCount--
			current.WriteRune(char)

		case ',':
			if bracketCount == 0 {
				// Only split on commas outside brackets
				if current.Len() > 0 {
					result = append(result, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(char)
			}

		default:
			current.WriteRune(char)
		}
	}

	// Add the last part if there is one
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// sortGroupLabels returns a sorted slice of group labels.
func sortGroupLabels(groupBy map[model.LabelName]struct{}) []string {
	result := make([]string, 0, len(groupBy))

	for k := range groupBy {
		result = append(result, string(k))
	}

	slices.Sort[[]string](result)

	return result
}

// parseLabelSet parses command line labels into a LabelSet.
func parseLabelSet(labels []string) (models.LabelSet, error) {
	ls := make(models.LabelSet, len(labels))
	for _, l := range labels {
		matcher, err := compat.Matcher(l, "cli")
		if err != nil {
			return nil, fmt.Errorf("Failed to parse labels: %w", err)
		}

		if matcher.Type != 0 { // 0 is labels.MatchEqual
			return nil, fmt.Errorf("labels must be specified as key=value pairs")
		}

		ls[matcher.Name] = matcher.Value
	}
	return ls, nil
}

// verifyReceivers checks if the actual receivers match the expected ones.
func verifyReceivers(expected, actual string) error {
	if expected == "" {
		return nil
	}
	expectedReceivers := strings.Split(expected, ",")
	actualReceivers := strings.Split(actual, ",")

	// Create maps to count occurrences
	expectedCounts := make(map[string]int)
	actualCounts := make(map[string]int)

	for _, r := range expectedReceivers {
		expectedCounts[r]++
	}
	for _, r := range actualReceivers {
		actualCounts[r]++
	}

	// Compare counts for each receiver
	for receiver, expectedCount := range expectedCounts {
		actualCount := actualCounts[receiver]
		if actualCount != expectedCount {
			return fmt.Errorf("expected %d occurrence(s) of receiver %q, but got %d.\nExpected: %v\nGot: %v",
				expectedCount, receiver, actualCount, expectedReceivers, actualReceivers)
		}
	}

	// Check for extra receivers in actual that weren't expected
	for receiver, actualCount := range actualCounts {
		if expectedCounts[receiver] == 0 {
			return fmt.Errorf("got unexpected receiver %q %d time(s).\nExpected: %v\nGot: %v",
				receiver, actualCount, expectedReceivers, actualReceivers)
		}
	}

	return nil
}

// verifyReceiversGrouping checks if receivers and their groupings match the expected configuration.
func verifyReceiversGrouping(receiversGrouping map[string][]string, finalRoutes []*dispatch.Route) error {
	if len(receiversGrouping) == 0 {
		return nil
	}

	// Build slice of actual receivers and their groupings to handle multiple occurrences
	type receiverInfo struct {
		name     string
		grouping []string
		matched  bool
	}
	var actualReceivers []receiverInfo
	for _, route := range finalRoutes {
		actualReceivers = append(actualReceivers, receiverInfo{
			name:     route.RouteOpts.Receiver,
			grouping: sortGroupLabels(route.RouteOpts.GroupBy),
		})
	}

	// Check each expected receiver and its grouping
	for expectedReceiver, expectedGroups := range receiversGrouping {
		baseReceiver := strings.Split(expectedReceiver, "_")[0]

		// Find a matching receiver that hasn't been matched yet
		matched := false
		for i := range actualReceivers {
			if actualReceivers[i].matched || actualReceivers[i].name != baseReceiver {
				continue
			}

			if expectedGroups == nil {
				// For receivers where we expect no grouping
				if len(actualReceivers[i].grouping) > 0 {
					continue // Try next occurrence if this one has grouping
				}
			} else {
				// Special case: if expectedGroups contains "..." it means group by all labels
				if len(expectedGroups) == 1 && expectedGroups[0] == "..." {
					// Any grouping is acceptable
					actualReceivers[i].matched = true
					matched = true
					break
				}

				// For receivers where we expect specific grouping
				sortedExpected := slices.Clone(expectedGroups)
				sortedActual := slices.Clone(actualReceivers[i].grouping)
				slices.Sort(sortedExpected)
				slices.Sort(sortedActual)

				if !slices.Equal(sortedExpected, sortedActual) {
					continue // Try next occurrence if grouping doesn't match
				}
			}

			actualReceivers[i].matched = true
			matched = true
			break
		}

		if !matched {
			// If we couldn't find a matching receiver instance
			if expectedGroups == nil {
				return fmt.Errorf("expected receiver %q without grouping not found", baseReceiver)
			} else if len(expectedGroups) == 1 && expectedGroups[0] == "..." {
				return fmt.Errorf("expected receiver %q with any grouping not found", baseReceiver)
			} else {
				return fmt.Errorf("expected receiver %q with grouping [%s] not found",
					baseReceiver, strings.Join(expectedGroups, ","))
			}
		}
	}

	// Check for unexpected extra receivers
	for _, r := range actualReceivers {
		if !r.matched {
			groupingStr := ""
			if len(r.grouping) > 0 {
				groupingStr = fmt.Sprintf(" with grouping [%s]", strings.Join(r.grouping, ","))
			}
			return fmt.Errorf("found unexpected receiver %q%s", r.name, groupingStr)
		}
	}

	return nil
}

// formatOutput generates the output string showing receivers and their groupings.
func formatOutput(finalRoutes []*dispatch.Route) string {
	var (
		sb    strings.Builder
		first = true
	)

	for _, route := range finalRoutes {
		if !first {
			sb.WriteString(",")
		}

		first = false
		sb.WriteString(route.RouteOpts.Receiver)

		if len(route.RouteOpts.GroupBy) > 0 {
			groupBySlice := sortGroupLabels(route.RouteOpts.GroupBy)
			sb.WriteString(fmt.Sprintf("[%s]", strings.Join(groupBySlice, ",")))
		}
	}

	return sb.String()
}

func (c *routingShow) routingTestAction(ctx context.Context, _ *kingpin.ParseContext) error {
	cfg, err := loadAlertmanagerConfig(ctx, alertmanagerURL, c.configFile)
	if err != nil {
		kingpin.Fatalf("%v\n", err)
		return err
	}

	if c.expectedReceiversGroup != "" {
		c.receiversGrouping, err = parseReceiversWithGrouping(c.expectedReceiversGroup)
		if err != nil {
			kingpin.Fatalf("Failed to parse receivers with grouping: %v\n", err)
		}
	}

	mainRoute := dispatch.NewRoute(cfg.Route, nil)

	ls, err := parseLabelSet(c.labels)
	if err != nil {
		kingpin.Fatalf("%v\n", err)
	}

	if c.debugTree {
		printMatchingTree(mainRoute, ls)
	}

	receivers, err := resolveAlertReceivers(mainRoute, &ls)
	if err != nil {
		return err
	}

	receiversSlug := strings.Join(receivers, ",")
	finalRoutes := mainRoute.Match(convertClientToCommonLabelSet(ls))

	// If we have expected receivers with grouping, verify both count and grouping.
	if c.expectedReceiversGroup != "" {
		if err := verifyReceiversGrouping(c.receiversGrouping, finalRoutes); err != nil {
			fmt.Printf("WARNING: %v\n", err)
			os.Exit(1)
		}
	} else if c.expectedReceivers != "" {
		if err := verifyReceivers(c.expectedReceivers, receiversSlug); err != nil {
			fmt.Printf("WARNING: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println(formatOutput(finalRoutes))
	return nil
}
