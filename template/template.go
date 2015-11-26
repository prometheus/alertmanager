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

package template

import (
	"bytes"
	"sort"
	"strings"

	tmplhtml "html/template"
	tmpltext "text/template"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

// Template bundles a text and a html template instance.
type Template struct {
	text *tmpltext.Template
	html *tmplhtml.Template
}

// FromGlobs calls ParseGlob on all path globs provided and returns the
// resulting Template.
func FromGlobs(paths ...string) (*Template, error) {
	t := &Template{
		text: tmpltext.New("").Option("missingkey=zero"),
		html: tmplhtml.New("").Option("missingkey=zero"),
	}
	var err error

	t.text = t.text.Funcs(tmpltext.FuncMap(DefaultFuncs))
	t.html = t.html.Funcs(tmplhtml.FuncMap(DefaultFuncs))

	for _, tp := range paths {
		if t.text, err = t.text.ParseGlob(tp); err != nil {
			return nil, err
		}
		if t.html, err = t.html.ParseGlob(tp); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// ExecuteTextString needs a meaningful doc comment (TODO(fabxc)).
func (t *Template) ExecuteTextString(text string, data interface{}) (string, error) {
	tmpl, err := t.text.Clone()
	if err != nil {
		return "", err
	}
	tmpl, err = tmpl.New("").Parse(text)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	return buf.String(), err
}

// ExecuteHTMLString needs a meaningful doc comment (TODO(fabxc)).
func (t *Template) ExecuteHTMLString(html string, data interface{}) (string, error) {
	tmpl, err := t.html.Clone()
	if err != nil {
		return "", err
	}
	tmpl, err = tmpl.New("").Parse(html)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	return buf.String(), err
}

type FuncMap map[string]interface{}

var DefaultFuncs = FuncMap{
	"toUpper": strings.ToUpper,
	"toLower": strings.ToLower,
	"toTitle": strings.ToTitle,
	// sortedPairs allows for in-order iteration of key/value pairs.
	"sortedPairs": func(m map[string]string) []Pair {
		var (
			pairs     = make([]Pair, 0, len(m))
			keys      = make([]string, 0, len(m))
			sortStart = 0
		)
		for k := range m {
			if k == string(model.AlertNameLabel) {
				keys = append([]string{k}, keys...)
				sortStart = 1
			} else {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys[sortStart:])

		for _, k := range keys {
			pairs = append(pairs, Pair{k, m[k]})
		}
		return pairs
	},
	"firing": func(alerts []Alert) []Alert {
		res := []Alert{}
		for _, a := range alerts {
			if a.Status == string(model.AlertFiring) {
				res = append(res, a)
			}
		}
		return res
	},
	"resolved": func(alerts []Alert) []Alert {
		res := []Alert{}
		for _, a := range alerts {
			if a.Status == string(model.AlertResolved) {
				res = append(res, a)
			}
		}
		return res
	},
}

// Pair is a key/value string pair.
type Pair struct {
	Name, Value string
}

// Data is the data passed to notification templates.
// End-users should not be exposed to Go's type system,
// as this will confuse them and prevent simple things like
// simple equality checks to fail. Map everything to float64/string.
type Data struct {
	Status string
	Alerts []Alert

	GroupLabels       map[string]string
	CommonLabels      map[string]string
	CommonAnnotations map[string]string

	ExternalURL string
}

// Alert holds one alert for notification templates.
type Alert struct {
	Status      string
	Labels      map[string]string
	Annotations map[string]string
}

func NewData(groupLabels model.LabelSet, as ...*types.Alert) *Data {
	alerts := types.Alerts(as...)

	data := &Data{
		Status:            string(alerts.Status()),
		Alerts:            make([]Alert, 0, len(alerts)),
		GroupLabels:       map[string]string{},
		CommonLabels:      map[string]string{},
		CommonAnnotations: map[string]string{},
		ExternalURL:       "something",
	}

	for _, a := range alerts {
		alert := Alert{
			Status:      string(a.Status()),
			Labels:      make(map[string]string, len(a.Labels)),
			Annotations: make(map[string]string, len(a.Annotations)),
		}
		for k, v := range a.Labels {
			alert.Labels[string(k)] = string(v)
		}
		for k, v := range a.Annotations {
			alert.Annotations[string(k)] = string(v)
		}
		data.Alerts = append(data.Alerts, alert)
	}

	for k, v := range groupLabels {
		data.GroupLabels[string(k)] = string(v)
	}

	if len(alerts) >= 1 {
		var (
			commonLabels      = alerts[0].Labels.Clone()
			commonAnnotations = alerts[0].Annotations.Clone()
		)
		for _, a := range alerts[1:] {
			for ln, lv := range commonLabels {
				if a.Labels[ln] != lv {
					delete(commonLabels, ln)
				}
			}
			for an, av := range commonAnnotations {
				if a.Annotations[an] != av {
					delete(commonAnnotations, an)
				}
			}
		}
		for k, v := range commonLabels {
			data.CommonLabels[string(k)] = string(v)
		}
		for k, v := range commonAnnotations {
			data.CommonAnnotations[string(k)] = string(v)
		}
	}

	return data
}
