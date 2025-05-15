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
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/common/model"
)

type routingTestDefinition struct {
	alert             models.LabelSet
	expectedReceivers []string
	configFile        string
}

func checkResolvedReceivers(mainRoute *dispatch.Route, ls models.LabelSet, expectedReceivers []string) error {
	resolvedReceivers, err := resolveAlertReceivers(mainRoute, &ls)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expectedReceivers, resolvedReceivers) {
		return fmt.Errorf("unexpected routing result want: `%s`, got: `%s`", strings.Join(expectedReceivers, ","), strings.Join(resolvedReceivers, ","))
	}
	return nil
}

func TestRoutingTest(t *testing.T) {
	tests := []*routingTestDefinition{
		{configFile: "testdata/conf.routing.yml", alert: models.LabelSet{"test": "1"}, expectedReceivers: []string{"test1"}},
		{configFile: "testdata/conf.routing.yml", alert: models.LabelSet{"test": "2"}, expectedReceivers: []string{"test1", "test2"}},
		{configFile: "testdata/conf.routing-reverted.yml", alert: models.LabelSet{"test": "2"}, expectedReceivers: []string{"test2", "test1"}},
		{configFile: "testdata/conf.routing.yml", alert: models.LabelSet{"test": "volovina"}, expectedReceivers: []string{"default"}},
	}

	for _, test := range tests {
		cfg, err := config.LoadFile(test.configFile)
		if err != nil {
			t.Fatalf("failed to load test configuration: %v", err)
		}
		mainRoute := dispatch.NewRoute(cfg.Route, nil)
		err = checkResolvedReceivers(mainRoute, test.alert, test.expectedReceivers)
		if err != nil {
			t.Fatalf("%v", err)
		}
		fmt.Println("  OK")
	}
}

func TestParseReceiversWithGrouping(t *testing.T) {
	tests := []struct {
		input    string
		expected map[string][]string
		wantErr  bool
	}{
		{
			input: "infrasec-team-opsgenie=[product],infrasec-team-opsgenie=[...]",
			expected: map[string][]string{
				"infrasec-team-opsgenie":   {"product"},
				"infrasec-team-opsgenie_1": {"..."},
			},
		},
		{
			input: "team1=[group1,group2],team2",
			expected: map[string][]string{
				"team1": {"group1", "group2"},
				"team2": {"..."},
			},
		},
		{
			input: "team1=[...],team1=[group2],team1",
			expected: map[string][]string{
				"team1":   {"..."},
				"team1_1": {"group2"},
				"team1_2": {"..."},
			},
		},
		{
			input: "team1,team1=[group1]",
			expected: map[string][]string{
				"team1":   {"..."},
				"team1_1": {"group1"},
			},
		},
		{
			input: "team1=[],team2=[...],team3",
			expected: map[string][]string{
				"team1": {"..."},
				"team2": {"..."},
				"team3": {"..."},
			},
		},
	}

	for _, tt := range tests {
		got, err := parseReceiversWithGrouping(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseReceiversWithGrouping(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !reflect.DeepEqual(got, tt.expected) {
			t.Errorf("parseReceiversWithGrouping(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func makeRoute(receiver string, groupByLabels ...string) *dispatch.Route {
	gbMap := make(map[model.LabelName]struct{})
	for _, l := range groupByLabels {
		gbMap[model.LabelName(l)] = struct{}{}
	}

	dispatchOpts := dispatch.RouteOpts{
		Receiver: receiver,
		GroupBy:  gbMap,
	}

	return &dispatch.Route{
		RouteOpts: dispatchOpts,
		// Note: Other fields of dispatch.Route like Matchers, Continue, Routes are not populated
		// as they are not directly used by verifyReceiversGrouping. verifyReceiversGrouping
		// primarily uses route.RouteOpts.Receiver and the result of sortGroupLabels(route.RouteOpts.GroupBy).
	}
}

func TestVerifyReceiversGrouping(t *testing.T) {
	tests := []struct {
		name              string
		receiversGrouping map[string][]string
		finalRoutes       []*dispatch.Route
		wantErr           bool
		errContains       string
	}{
		{
			name: "exact match",
			receiversGrouping: map[string][]string{
				"rec-A": {"g1", "g2"},
			},
			finalRoutes: []*dispatch.Route{makeRoute("rec-A", "g1", "g2")},
			wantErr:     false,
		},
		{
			name: "exact match - different order in expectation",
			receiversGrouping: map[string][]string{
				"rec-A": {"g2", "g1"},
			},
			finalRoutes: []*dispatch.Route{makeRoute("rec-A", "g1", "g2")},
			wantErr:     false,
		},
		{
			name: "no grouping match",
			receiversGrouping: map[string][]string{
				"rec-B": {},
			},
			finalRoutes: []*dispatch.Route{makeRoute("rec-B")},
			wantErr:     false,
		},
		{
			name: "any grouping match",
			receiversGrouping: map[string][]string{
				"rec-C": {"..."},
			},
			finalRoutes: []*dispatch.Route{makeRoute("rec-C", "g1")},
			wantErr:     false,
		},
		{
			name: "mismatch - wrong group label",
			receiversGrouping: map[string][]string{
				"rec-A": {"g1", "g3"},
			},
			finalRoutes: []*dispatch.Route{makeRoute("rec-A", "g1", "g2")},
			wantErr:     true,
			errContains: `expected receiver "rec-A" with grouping [g1,g3] not found`,
		},
		{
			name: "mismatch - expected no group, but has group",
			receiversGrouping: map[string][]string{
				"rec-A": {},
			},
			finalRoutes: []*dispatch.Route{makeRoute("rec-A", "g1")},
			wantErr:     true,
			errContains: `expected receiver "rec-A" with no grouping not found`,
		},
		{
			name: "mismatch - expected group, but has no group",
			receiversGrouping: map[string][]string{
				"rec-B": {"g1"},
			},
			finalRoutes: []*dispatch.Route{makeRoute("rec-B")},
			wantErr:     true,
			errContains: `expected receiver "rec-B" with grouping [g1] not found`,
		},
		{
			name: "mismatch - expected receiver not found",
			receiversGrouping: map[string][]string{
				"rec-Z": {"g1"},
			},
			finalRoutes: []*dispatch.Route{makeRoute("rec-A", "g1")},
			wantErr:     true,
			errContains: `expected receiver "rec-Z" with grouping [g1] not found`,
		},
		{
			name: "mismatch - unexpected receiver present",
			receiversGrouping: map[string][]string{
				"rec-A": {"g1"},
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "g1"),
				makeRoute("rec-B"), // Unexpected
			},
			wantErr:     true,
			errContains: `found unexpected receiver "rec-B" with no grouping`,
		},
		{
			name: "multiple expectations, all match",
			receiversGrouping: map[string][]string{
				"rec-A": {"g1"},
				"rec-B": {},
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "g1"),
				makeRoute("rec-B"),
			},
			wantErr: false,
		},
		{
			name: "multiple expectations, one fails (wrong group)",
			receiversGrouping: map[string][]string{
				"rec-A": {"g1"},
				"rec-B": {"g2"},
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "g1"),
				makeRoute("rec-B"), // rec-B has no group, expected [g2]
			},
			wantErr:     true,
			errContains: `expected receiver "rec-B" with grouping [g2] not found`,
		},
		{
			name: "multiple expectations, one fails (missing expected)",
			receiversGrouping: map[string][]string{
				"rec-A": {"g1"},
				"rec-D": {"g3"}, // rec-D not in finalRoutes
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "g1"),
				makeRoute("rec-B"),
			},
			wantErr:     true,
			errContains: `expected receiver "rec-D" with grouping [g3] not found`,
		},
		{
			name: "suffixed receiver expectation - match",
			receiversGrouping: map[string][]string{
				"rec-A":   {"app"},
				"rec-A_1": {"db"},
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "app"),
				makeRoute("rec-A", "db"),
			},
			wantErr: false,
		},
		{
			name: "suffixed receiver expectation - second one not found",
			receiversGrouping: map[string][]string{
				"rec-A":   {"app"},
				"rec-A_1": {"db"},
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "app"),
			},
			wantErr:     true,
			errContains: `expected receiver "rec-A" with grouping [db] not found`,
		},
		{
			name: "suffixed receiver expectation - second one wrong group",
			receiversGrouping: map[string][]string{
				"rec-A":   {"app"},
				"rec-A_1": {"db"},
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "app"),
				makeRoute("rec-A", "log"), // Actual group [log] != expected [db]
			},
			wantErr:     true,
			errContains: `expected receiver "rec-A" with grouping [db] not found`,
		},
		{
			name: "any grouping for one, specific for another",
			receiversGrouping: map[string][]string{
				"rec-A": {"..."},
				"rec-B": {"specific"},
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "anything"),
				makeRoute("rec-B", "specific"),
			},
			wantErr: false,
		},
		{
			name: "any grouping for one, specific for another - B fails",
			receiversGrouping: map[string][]string{
				"rec-A": {"..."},
				"rec-B": {"specific"},
			},
			finalRoutes: []*dispatch.Route{
				makeRoute("rec-A", "anything"),
				makeRoute("rec-B", "other"),
			},
			wantErr:     true,
			errContains: `expected receiver "rec-B" with grouping [specific] not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyReceiversGrouping(tt.receiversGrouping, tt.finalRoutes)
			if tt.wantErr {
				if err == nil {
					t.Errorf("verifyReceiversGrouping() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("verifyReceiversGrouping() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("verifyReceiversGrouping() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}
