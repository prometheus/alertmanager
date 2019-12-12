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

	"github.com/prometheus/common/model"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/alertmanager/api/v2/client/general"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
)

// GetAlertmanagerURL appends the given path to the alertmanager base URL
func GetAlertmanagerURL(p string) url.URL {
	amURL := *alertmanagerURL
	amURL.Path = path.Join(alertmanagerURL.Path, p)
	return amURL
}

// parseMatchers parses a list of matchers (cli arguments).
func parseMatchers(inputMatchers []string) ([]labels.Matcher, error) {
	matchers := make([]labels.Matcher, 0, len(inputMatchers))

	for _, v := range inputMatchers {
		matcher, err := labels.ParseMatcher(v)
		if err != nil {
			return []labels.Matcher{}, err
		}

		matchers = append(matchers, *matcher)
	}

	return matchers, nil
}

// getRemoteAlertmanagerConfigStatus returns status responsecontaining configuration from remote Alertmanager
func getRemoteAlertmanagerConfigStatus(ctx context.Context, alertmanagerURL *url.URL) (*models.AlertmanagerStatus, error) {
	amclient := NewAlertmanagerClient(alertmanagerURL)
	params := general.NewGetStatusParams().WithContext(ctx)
	getOk, err := amclient.General.GetStatus(params)
	if err != nil {
		return nil, err
	}

	return getOk.Payload, nil
}

func checkRoutingConfigInputFlags(alertmanagerURL *url.URL, configFile string) {
	if alertmanagerURL != nil && configFile != "" {
		fmt.Fprintln(os.Stderr, "Warning: --config.file flag overrides the --alertmanager.url.")
	}
	if alertmanagerURL == nil && configFile == "" {
		kingpin.Fatalf("You have to specify one of --config.file or --alertmanager.url flags.")
	}
}

func loadAlertmanagerConfig(ctx context.Context, alertmanagerURL *url.URL, configFile string) (*config.Config, error) {
	checkRoutingConfigInputFlags(alertmanagerURL, configFile)
	if configFile != "" {
		cfg, err := config.LoadFile(configFile)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if alertmanagerURL == nil {
		return nil, errors.New("failed to get Alertmanager configuration")
	}
	configStatus, err := getRemoteAlertmanagerConfigStatus(ctx, alertmanagerURL)
	if err != nil {
		return nil, err
	}
	return config.Load(*configStatus.Config.Original)
}

// convertClientToCommonLabelSet converts client.LabelSet to model.Labelset
func convertClientToCommonLabelSet(cls models.LabelSet) model.LabelSet {
	mls := make(model.LabelSet, len(cls))
	for ln, lv := range cls {
		mls[model.LabelName(ln)] = model.LabelValue(lv)
	}
	return mls
}

// parseLabels parses a list of labels (cli arguments).
func parseLabels(inputLabels []string) (models.LabelSet, error) {
	labelSet := make(models.LabelSet, len(inputLabels))

	for _, l := range inputLabels {
		matcher, err := labels.ParseMatcher(l)
		if err != nil {
			return models.LabelSet{}, err
		}
		if matcher.Type != labels.MatchEqual {
			return models.LabelSet{}, errors.New("labels must be specified as key=value pairs")
		}

		labelSet[matcher.Name] = matcher.Value
	}

	return labelSet, nil
}

// TypeMatchers only valid for when you are going to add a silence
func TypeMatchers(matchers []labels.Matcher) (models.Matchers, error) {
	typeMatchers := models.Matchers{}
	for _, matcher := range matchers {
		typeMatcher, err := TypeMatcher(matcher)
		if err != nil {
			return models.Matchers{}, err
		}
		typeMatchers = append(typeMatchers, &typeMatcher)
	}
	return typeMatchers, nil
}

// TypeMatcher only valid for when you are going to add a silence
// Doesn't allow negative operators
func TypeMatcher(matcher labels.Matcher) (models.Matcher, error) {
	name := matcher.Name
	value := matcher.Value
	typeMatcher := models.Matcher{
		Name:  &name,
		Value: &value,
	}

	isRegex := false
	switch matcher.Type {
	case labels.MatchEqual:
		isRegex = false
	case labels.MatchRegexp:
		isRegex = true
	default:
		return models.Matcher{}, fmt.Errorf("invalid match type for creation operation: %s", matcher.Type)
	}
	typeMatcher.IsRegex = &isRegex
	return typeMatcher, nil
}

// Helper function for adding the ctx with timeout into an action.
func execWithTimeout(fn func(context.Context, *kingpin.ParseContext) error) func(*kingpin.ParseContext) error {
	return func(x *kingpin.ParseContext) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return fn(ctx, x)
	}
}
