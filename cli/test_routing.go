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
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/xlab/treeprint"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/matcher/compat"
	"github.com/prometheus/alertmanager/pkg/labels"
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

func parseReceiversWithGrouping(input string) (map[string][]string, error) {
	result := make(map[string][][]string) // maps receiver to list of possible groupings
	// If no square brackets in input, treat it as simple receiver list
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

	receivers := strings.Split(input, ",")
	for _, r := range receivers {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}

		parts := strings.Split(r, "[")
		if len(parts) > 2 {
			return nil, fmt.Errorf("invalid receiver format: %s", r)
		}

		receiverName := strings.TrimSpace(parts[0])
		if receiverName == "" {
			return nil, fmt.Errorf("empty receiver name in: %s", r)
		}

		if len(parts) == 2 {
			if !strings.HasSuffix(parts[1], "]") {
				return nil, fmt.Errorf("missing closing bracket in: %s", r)
			}
			grouping := strings.TrimSuffix(parts[1], "]")
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
		} else {
			result[receiverName] = nil
		}
	}
	return flattenGroupingMap(result), nil
}

// flattenGroupingMap converts the internal map[string][][]string to the expected map[string][]string format
func flattenGroupingMap(input map[string][][]string) map[string][]string {
	result := make(map[string][]string)
	for receiver, groupings := range input {
		if groupings == nil {
			result[receiver] = nil
			continue
		}
		// For receivers with grouping, we'll create separate entries with suffixes
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

	// Parse labels to LabelSet.
	ls := make(models.LabelSet, len(c.labels))
	for _, l := range c.labels {
		matcher, err := compat.Matcher(l, "cli")
		if err != nil {
			kingpin.Fatalf("Failed to parse labels: %v\n", err)
		}
		if matcher.Type != labels.MatchEqual {
			kingpin.Fatalf("%s\n", "Labels must be specified as key=value pairs")
		}
		ls[matcher.Name] = matcher.Value
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

	// Verify receivers.
	if c.expectedReceivers != "" && c.expectedReceivers != receiversSlug {
		fmt.Printf("WARNING: Expected receivers did not match resolved receivers.\n")
		os.Exit(1)
	}

	// Verify receivers and their grouping.
	if len(c.receiversGrouping) > 0 {
		matchedReceivers := make(map[string]bool)

		for _, route := range finalRoutes {
			receiver := route.RouteOpts.Receiver
			actualGroups := make([]string, 0, len(route.RouteOpts.GroupBy))

			for k := range route.RouteOpts.GroupBy {
				actualGroups = append(actualGroups, string(k))
			}

			// Try to match with any of the expected groupings.
			matched := false

			for expectedReceiver, expectedGroups := range c.receiversGrouping {
				baseReceiver := strings.Split(expectedReceiver, "_")[0]
				if baseReceiver == receiver && expectedGroups != nil {
					if stringSlicesEqual(expectedGroups, actualGroups) {
						matchedReceivers[expectedReceiver] = true
						matched = true
						break
					}
				}
			}

			if !matched && c.receiversGrouping[receiver] != nil {
				fmt.Printf("WARNING: No matching grouping found for receiver %s with groups %v\n",
					receiver, actualGroups)
				os.Exit(1)
			}
		}

		// Check if all expected receivers with grouping were matched.
		for expectedReceiver, expectedGroups := range c.receiversGrouping {
			if expectedGroups != nil && !matchedReceivers[expectedReceiver] {
				fmt.Printf("WARNING: Expected receiver %s with grouping %v was not matched\n",
					expectedReceiver, expectedGroups)
				os.Exit(1)
			}
		}
	}

	var output strings.Builder

	output.WriteString(receiversSlug)

	if len(finalRoutes) > 0 {
		for _, route := range finalRoutes {
			if len(route.RouteOpts.GroupBy) > 0 {
				groupBySlice := make([]string, 0, len(route.RouteOpts.GroupBy))
				for k := range route.RouteOpts.GroupBy {
					groupBySlice = append(groupBySlice, string(k))
				}

				output.WriteString(fmt.Sprintf("[%s]", strings.Join(groupBySlice, ",")))
			}
		}
	}

	fmt.Println(output.String())
	return nil
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
