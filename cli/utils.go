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
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/prometheus/alertmanager/client"
	amconfig "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/client_golang/api"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

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

// Parse a list of matchers (cli arguments)
func parseMatchers(inputMatchers []string) ([]labels.Matcher, error) {
	matchers := make([]labels.Matcher, 0)

	for _, v := range inputMatchers {
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

// getRemoteAlertmanagerConfigStatus returns status responsecontaining configuration from remote Alertmanager
func getRemoteAlertmanagerConfigStatus(ctx context.Context, alertmanagerURL *url.URL) (*client.ServerStatus, error) {
	c, err := api.NewClient(api.Config{Address: alertmanagerURL.String()})
	if err != nil {
		return nil, err
	}
	statusAPI := client.NewStatusAPI(c)
	status, err := statusAPI.Get(ctx)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func checkRoutingConfigInputFlags(alertmanagerURL *url.URL, configFile string) {
	if alertmanagerURL != nil && configFile != "" {
		fmt.Fprintln(os.Stderr, "Warning: --config.file flag overrides the --alertmanager.url.")
	}
	if alertmanagerURL == nil && configFile == "" {
		kingpin.Fatalf("You have to specify one of --config.file or --alertmanager.url flags.")
	}
}

func loadAlertmanagerConfig(ctx context.Context, alertmanagerURL *url.URL, configFile string) (*amconfig.Config, error) {
	checkRoutingConfigInputFlags(alertmanagerURL, configFile)
	if configFile != "" {
		cfg, _, err := amconfig.LoadFile(configFile)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if alertmanagerURL != nil {
		status, err := getRemoteAlertmanagerConfigStatus(ctx, alertmanagerURL)
		if err != nil {
			return nil, err
		}
		return status.ConfigJSON, nil
	}
	return nil, errors.New("failed to get Alertmanager configuration")
}

// convertClientToCommonLabelSet converts client.LabelSet to model.Labelset
func convertClientToCommonLabelSet(cls client.LabelSet) model.LabelSet {
	mls := make(model.LabelSet, len(cls))
	for ln, lv := range cls {
		mls[model.LabelName(ln)] = model.LabelValue(lv)
	}
	return mls
}

// Parse a list of labels (cli arguments)
func parseLabels(inputLabels []string) (client.LabelSet, error) {
	labelSet := make(client.LabelSet, len(inputLabels))

	for _, l := range inputLabels {
		name, value, matchType, err := parse.Input(l)
		if err != nil {
			return client.LabelSet{}, err
		}
		if matchType != labels.MatchEqual {
			return client.LabelSet{}, errors.New("labels must be specified as key=value pairs")
		}

		labelSet[client.LabelName(name)] = client.LabelValue(value)
	}

	return labelSet, nil
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

// Helper function for adding the ctx with timeout into an action.
func execWithTimeout(fn func(context.Context, *kingpin.ParseContext) error) func(*kingpin.ParseContext) error {
	return func(x *kingpin.ParseContext) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return fn(ctx, x)
	}
}
