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
