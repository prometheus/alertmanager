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
	result := make(map[string][][]string) // maps receiver to list of possible groupings.

	input = removeSpacesAroundCommas(input)

	// If no square brackets in input, treat it as simple receiver list.
	if !strings.Contains(input, "[") {
		receivers := strings.Split(input, ",")

		for _, r := range receivers {
			r = strings.TrimSpace(r)
			if r != "" {
				result[r] = nil
			}
		}

		return flattenGroupingMap(result), nil
	}

	// Split by comma but preserve commas within square brackets.
	var (
		receivers       []string
		currentReceiver strings.Builder
		inBrackets      = false
		bracketCount    = 0
	)

	for i := 0; i < len(input); i++ {
		char := input[i]

		if char == '[' {
			inBrackets = true
			bracketCount++
		}

		if char == ']' {
			bracketCount--

			if bracketCount == 0 {
				inBrackets = false
			}
		}

		switch {
		case char == ',' && !inBrackets:
			if currentReceiver.Len() > 0 {
				receivers = append(receivers, strings.TrimSpace(currentReceiver.String()))
				currentReceiver.Reset()
			}

		default:
			currentReceiver.WriteByte(char)
		}
	}

	if currentReceiver.Len() > 0 {
		receivers = append(receivers, strings.TrimSpace(currentReceiver.String()))
	}

	for _, r := range receivers {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}

		bracketIndex := strings.LastIndex(r, "[")
		if bracketIndex == -1 {
			// No grouping specified.
			result[r] = nil
			continue
		}

		receiverName := strings.TrimSpace(r[:bracketIndex])
		if receiverName == "" {
			return nil, fmt.Errorf("empty receiver name in: %s", r)
		}

		groupingPart := r[bracketIndex:]
		if !strings.HasPrefix(groupingPart, "[") || !strings.HasSuffix(groupingPart, "]") {
			return nil, fmt.Errorf("missing closing bracket in: %s", r)
		}

		grouping := strings.TrimSuffix(strings.TrimPrefix(groupingPart, "["), "]")
		groups := strings.Split(grouping, ",")

		// Clean up group names
		cleanGroups := make([]string, 0, len(groups))
		for _, g := range groups {
			g = strings.TrimSpace(g)
			if g != "" {
				cleanGroups = append(cleanGroups, g)
			}
		}

		if result[receiverName] == nil {
			result[receiverName] = make([][]string, 0)
		}

		result[receiverName] = append(result[receiverName], cleanGroups)
	}

	return flattenGroupingMap(result), nil
}

// removeSpacesAroundCommas removes spaces around commas.
func removeSpacesAroundCommas(input string) string {
	input = strings.ReplaceAll(input, " ,", ",")
	input = strings.ReplaceAll(input, ", ", ",")

	return input
}

// flattenGroupingMap converts the internal map[string][][]string to the expected map[string][]string format.
func flattenGroupingMap(input map[string][][]string) map[string][]string {
	result := make(map[string][]string)

	for receiver, groupings := range input {
		if groupings == nil {
			result[receiver] = nil
			continue
		}

		// For receivers with grouping, we'll create separate entries with suffixes.
		for i, groups := range groupings {
			if i == 0 {
				result[receiver] = groups
			} else {
				result[fmt.Sprintf("%s_%d", receiver, i)] = groups
			}
		}
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
			return nil, fmt.Errorf("failed to parse labels: %w", err)
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

	if !slices.Equal[[]string](expectedReceivers, actualReceivers) {
		return fmt.Errorf("expected receivers did not match resolved receivers.\nExpected: %v\nGot: %v",
			expectedReceivers, actualReceivers)
	}

	return nil
}

// verifyReceiversGrouping checks if receivers and their groupings match the expected configuration.
func verifyReceiversGrouping(receiversGrouping map[string][]string, finalRoutes []*dispatch.Route) error {
	if len(receiversGrouping) == 0 {
		return nil
	}

	// First, build a map of base receivers to their expected groupings
	expectedGroupings := make(map[string][][]string)
	for receiver, groups := range receiversGrouping {
		baseReceiver := strings.Split(receiver, "_")[0]
		if groups != nil {
			expectedGroupings[baseReceiver] = append(expectedGroupings[baseReceiver], groups)
		}
	}

	for _, route := range finalRoutes {
		receiver := route.RouteOpts.Receiver
		actualGroups := sortGroupLabels(route.RouteOpts.GroupBy)

		// Skip if no grouping expectations for this receiver
		if _, exists := expectedGroupings[receiver]; !exists {
			continue
		}

		// Try to match with any of the expected groupings
		matched := false
		for _, expectedGroups := range expectedGroupings[receiver] {
			if slices.Equal[[]string](expectedGroups, actualGroups) {
				matched = true
				break
			}
		}

		if !matched {
			var msg strings.Builder
			msg.WriteString(fmt.Sprintf("WARNING: No matching grouping found for receiver %s with groups [%s] expected groups are\n",
				receiver, strings.Join(actualGroups, ",")))

			for _, groups := range expectedGroupings[receiver] {
				msg.WriteString(fmt.Sprintf("- [%s]\n", strings.Join(groups, ",")))
			}
			return fmt.Errorf(msg.String())
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

	if err := verifyReceivers(c.expectedReceivers, receiversSlug); err != nil {
		fmt.Printf("WARNING: %v\n", err)
		os.Exit(1)
	}

	if err := verifyReceiversGrouping(c.receiversGrouping, finalRoutes); err != nil {
		fmt.Printf("WARNING: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(formatOutput(finalRoutes))

	return nil
}
