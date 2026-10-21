// Copyright The Prometheus Authors
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

package template

import (
	tmplhtml "html/template"
	"net/url"
	"testing"
	tmpltext "text/template"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/alert"
)

// TestSplitRouteLabels checks the static/dynamic classification of route
// label templates. For every case classified static it also renders the
// template against two different alert sets and checks the output is the
// same, so a wrong "static" verdict would be caught.
func TestSplitRouteLabels(t *testing.T) {
	t.Parallel()

	tmpl, err := New(func(text *tmpltext.Template, html *tmplhtml.Template) {
		text.Funcs(tmpltext.FuncMap{"custom": func() string { return "c" }})
		html.Funcs(tmplhtml.FuncMap{"custom": func() string { return "c" }})
	})
	require.NoError(t, err)
	tmpl.ExternalURL = &url.URL{Scheme: "http", Host: "am.example.com"}

	for _, tc := range []struct {
		name   string
		labels model.LabelSet
		static []model.LabelName
	}{
		{
			name:   "constants",
			labels: model.LabelSet{"a": "plain", "b": "", "c": `{{ "quoted" }} {{ 1 }} {{ true }}`},
			static: []model.LabelName{"a", "b", "c"},
		},
		{
			name: "group fields",
			labels: model.LabelSet{
				"receiver": "{{ .Receiver }}",
				"team":     "{{ .GroupLabels.team }}",
				"url":      "{{ .ExternalURL }}",
				"raw":      "{{ .RouteLabels.team }}",
				"root":     "{{ $.GroupLabels.team }}",
			},
			static: []model.LabelName{"receiver", "team", "url", "raw", "root"},
		},
		{
			name: "methods and pure funcs on static values",
			labels: model.LabelSet{
				"pairs":  `{{ range .GroupLabels.SortedPairs }}{{ .Name }}={{ .Value }};{{ end }}`,
				"names":  `{{ .GroupLabels.Names | join "," }}`,
				"remove": `{{ (.GroupLabels.Remove (stringSlice "team")).Names | join "," }}`,
				"upper":  `{{ .GroupLabels.team | toUpper }}`,
				"printf": `{{ printf "%s-%s" .Receiver .GroupLabels.team }}`,
				"index":  `{{ index .GroupLabels "team" }}`,
				"re":     `{{ reReplaceAll "a" "b" .GroupLabels.team }}`,
				"date":   `{{ (toDate "2006" "2020") | date "2006-01" }}`,
				"json":   `{{ .GroupLabels | toJson }}`,
			},
			static: []model.LabelName{"pairs", "names", "remove", "upper", "printf", "index", "re", "date", "json"},
		},
		{
			name: "control structures over static pipelines",
			labels: model.LabelSet{
				"if":     `{{ if eq .GroupLabels.team "x" }}yes{{ else }}no{{ end }}`,
				"with":   `{{ with .GroupLabels.team }}[{{ . }}]{{ else }}none{{ end }}`,
				"var":    `{{ $t := .GroupLabels.team }}{{ $t | toUpper }}`,
				"assign": `{{ $t := "" }}{{ $t = .GroupLabels.team }}{{ $t }}`,
				"shadow": `{{ $t := .GroupLabels.team }}{{ with .GroupLabels.team }}{{ $t := $.Status }}{{ end }}{{ $t }}`,
				"ifvar":  `{{ if $t := .GroupLabels.team }}{{ $t }}{{ end }}`,
				"range":  `{{ range $k, $v := .GroupLabels }}{{ $k }}:{{ $v }},{{ end }}`,
			},
			static: []model.LabelName{"if", "with", "var", "assign", "shadow", "ifvar", "range"},
		},
		{
			name: "routeLabels references",
			labels: model.LabelSet{
				"team":    "{{ .GroupLabels.team }}",
				"desc":    `team is {{ routeLabels "team" }}`,
				"deep":    `{{ routeLabels "desc" | toUpper }}`,
				"none":    `{{ routeLabels "missing" }}`,
				"count":   `{{ len .Alerts }}`,
				"viadyn":  `{{ routeLabels "count" }}`,
				"cyc1":    `{{ routeLabels "cyc2" }}`,
				"cyc2":    `{{ routeLabels "cyc1" }}`,
				"dynname": `{{ routeLabels .GroupLabels.team }}`,
			},
			static: []model.LabelName{"team", "desc", "deep", "none"},
		},
		{
			name: "alert-derived data is dynamic",
			labels: model.LabelSet{
				"status":      "{{ .Status }}",
				"alerts":      "{{ len .Alerts }}",
				"first":       "{{ (index .Alerts 0).Labels.alertname }}",
				"common":      "{{ .CommonLabels.instance }}",
				"annot":       "{{ .CommonAnnotations.summary }}",
				"reason":      "{{ .NotificationReason }}",
				"dot":         "{{ . }}",
				"root":        "{{ $ }}",
				"firing":      `{{ range .Alerts.Firing }}{{ .Labels.alertname }}{{ end }}`,
				"rangeel":     `{{ range .Alerts }}{{ .Status }}{{ end }}`,
				"withdyn":     `{{ with .Status }}{{ . }}{{ end }}`,
				"ifdyn":       `{{ if .Alerts }}some{{ end }}`,
				"vardyn":      `{{ $s := .Status }}{{ $s }}`,
				"ifassign":    `{{ $x := "" }}{{ if .GroupLabels.team }}{{ $x = $.CommonLabels.severity }}{{ end }}{{ $x }}`,
				"withassign":  `{{ $x := "" }}{{ with .GroupLabels.team }}{{ $x = $.CommonLabels.severity }}{{ end }}{{ $x }}`,
				"rangeassign": `{{ $x := "" }}{{ range .GroupLabels }}{{ $x = $.CommonLabels.severity }}{{ end }}{{ $x }}`,
			},
			static: nil,
		},
		{
			name: "unknown, impure and unparseable",
			labels: model.LabelSet{
				"now":    `{{ now | date "2006" }}`,
				"since":  `{{ since (toDate "2006" "2020") }}`,
				"custom": `{{ custom }}`,
				"call":   `{{ call .GroupLabels.Names }}`,
				"tmpl":   `{{ template "x" . }}`,
				"block":  `{{ block "b" .GroupLabels }}{{ end }}`,
				"broken": `{{ .GroupLabels.team `,
				"nofunc": `{{ nosuchfunc }}`,
				"mixed":  `{{ .GroupLabels.team }}{{ .Status }}`,
			},
			static: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			static, dynamic := tmpl.SplitRouteLabels(tc.labels)

			wantStatic := model.LabelSet{}
			wantDynamic := tc.labels.Clone()
			for _, n := range tc.static {
				wantStatic[n] = tc.labels[n]
				delete(wantDynamic, n)
			}
			require.Equal(t, wantStatic, static)
			require.Equal(t, wantDynamic, dynamic)

			// Static labels must render identically whatever the alerts are.
			groupLabels := model.LabelSet{"team": "x"}
			render := func(alerts ...*alert.Alert) map[model.LabelName]string {
				data := tmpl.Data("recv", groupLabels, tc.labels, "unknown", alerts...)
				r := tmpl.RouteLabelRenderer(data)
				out := map[model.LabelName]string{}
				for name := range static {
					v, err := r(string(name))
					require.NoError(t, err, "static label %q", name)
					out[name] = v
				}
				return out
			}
			now := time.Now()
			a1 := alert.New(model.Alert{
				Labels:      model.LabelSet{"alertname": "A", "team": "x", "instance": "i1"},
				Annotations: model.LabelSet{"summary": "s1"},
				StartsAt:    now.Add(-time.Hour),
				EndsAt:      now.Add(time.Hour),
			}, now, false)
			a2 := alert.New(model.Alert{
				Labels:      model.LabelSet{"alertname": "B", "team": "x", "instance": "i2"},
				Annotations: model.LabelSet{"summary": "s2"},
				StartsAt:    now.Add(-2 * time.Hour),
				EndsAt:      now.Add(-time.Minute),
			}, now, false)
			require.Equal(t, render(), render(a1), "static render must not depend on alerts")
			require.Equal(t, render(a1), render(a1, a2), "static render must not depend on alerts")
		})
	}
}

// TestPureTemplateFuncsCoverDefaultFuncs forces a purity decision for every
// function in DefaultFuncs, so that a newly added function is classified
// explicitly instead of silently.
func TestPureTemplateFuncsCoverDefaultFuncs(t *testing.T) {
	t.Parallel()
	for name := range DefaultFuncs {
		pure, impure := pureTemplateFuncs[name], impureTemplateFuncs[name]
		require.True(t, pure || impure, "DefaultFuncs %q is in neither pureTemplateFuncs nor impureTemplateFuncs", name)
		require.False(t, pure && impure, "DefaultFuncs %q is in both pureTemplateFuncs and impureTemplateFuncs", name)
	}
	for name := range pureTemplateFuncs {
		if _, ok := DefaultFuncs[name]; ok {
			continue
		}
		// Must be a text/template builtin: it has to parse.
		_, err := tmpltext.New("").Parse("{{ " + name + " }}")
		require.NoError(t, err, "pureTemplateFuncs %q is neither a DefaultFunc nor a builtin", name)
	}
}
