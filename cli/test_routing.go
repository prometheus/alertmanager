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

	routingTestCmd.Flag("verify.receivers", "Checks if specified receivers matches resolved receivers. The command fails if the labelset does not route to the specified receivers.").StringVar(&c.expectedReceivers)
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

func (c *routingShow) routingTestAction(ctx context.Context, _ *kingpin.ParseContext) error {
	cfg, err := loadAlertmanagerConfig(ctx, alertmanagerURL, c.configFile)
	if err != nil {
		kingpin.Fatalf("%v\n", err)
		return err
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
	receiversSlug := strings.Join(receivers, ",")
	fmt.Printf("%s\n", receiversSlug)

	if c.expectedReceivers != "" && c.expectedReceivers != receiversSlug {
		fmt.Printf("WARNING: Expected receivers did not match resolved receivers.\n")
		os.Exit(1)
	}

	return err
}
