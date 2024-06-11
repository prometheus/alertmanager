// Copyright 2015 Prometheus Team
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

// Imported from https://github.com/grafana/mimir/blob/main/pkg/alertmanager/alertmanager_template.go.

package template

import (
	"encoding/json"
	"fmt"
	tmplhtml "html/template"
	"net/url"
	tmpltext "text/template"
)

type grafanaDatasource struct {
	Type string `json:"type,omitempty"`
	UID  string `json:"uid,omitempty"`
}

type grafanaExploreRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type grafanaExploreQuery struct {
	Datasource grafanaDatasource `json:"datasource"`
	Expr       string            `json:"expr"`
	Instant    bool              `json:"instant"`
	Range      bool              `json:"range"`
	RefID      string            `json:"refId"`
}

type grafanaExploreParams struct {
	Range   grafanaExploreRange   `json:"range"`
	Queries []grafanaExploreQuery `json:"queries"`
}

// grafanaExploreURL is a template helper function to generate Grafana range query explore URL in the alert template.
func grafanaExploreURL(grafanaURL, datasource, from, to, expr string) (string, error) {
	res, err := json.Marshal(&grafanaExploreParams{
		Range: grafanaExploreRange{
			From: from,
			To:   to,
		},
		Queries: []grafanaExploreQuery{
			{
				Datasource: grafanaDatasource{
					Type: "prometheus",
					UID:  datasource,
				},
				Expr:    expr,
				Instant: false,
				Range:   true,
				RefID:   "A",
			},
		},
	})
	return grafanaURL + "/explore?left=" + url.QueryEscape(string(res)), err
}

// queryFromGeneratorURL returns a PromQL expression parsed out from a GeneratorURL in Prometheus alert
func queryFromGeneratorURL(generatorURL string) (string, error) {
	u, err := url.Parse(generatorURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse generator URL: %w", err)
	}
	// See https://github.com/prometheus/prometheus/blob/259bb5c69263635887541964d1bfd7acc46682c6/util/strutil/strconv.go#L28
	queryParam, ok := u.Query()["g0.expr"]
	if !ok || len(queryParam) < 1 {
		return "", fmt.Errorf("query not found in the generator URL")
	}
	query, err := url.QueryUnescape(queryParam[0])
	if err != nil {
		return "", fmt.Errorf("failed to URL decode the query: %w", err)
	}
	return query, nil
}

// WithCustomFunctions returns template.Option which adds additional template functions
// to the default ones.
func WithCustomFunctions(userID string) Option {
	funcs := tmpltext.FuncMap{
		"tenantID":              func() string { return userID },
		"grafanaExploreURL":     grafanaExploreURL,
		"queryFromGeneratorURL": queryFromGeneratorURL,
	}
	return func(text *tmpltext.Template, html *tmplhtml.Template) {
		text.Funcs(funcs)
		html.Funcs(funcs)
	}
}
