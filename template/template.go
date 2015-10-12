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
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"time"

	html_tmpl "html/template"
	text_tmpl "text/template"

	"github.com/prometheus/common/model"
)

type Template struct {
	text *text_tmpl.Template
	html *html_tmpl.Template
}

func FromGlobs(paths ...string) (*Template, error) {
	t := &Template{
		text: text_tmpl.New(""),
		html: html_tmpl.New(""),
	}
	var err error

	for _, tp := range paths {
		if t.text, err = t.text.ParseGlob(tp); err != nil {
			return nil, err
		}
		if t.html, err = t.html.ParseGlob(tp); err != nil {
			return nil, err
		}
	}

	t.funcs(DefaultFuncs)
	return t, nil
}

type FuncMap map[string]interface{}

func (t *Template) funcs(fm FuncMap) *Template {
	t.text.Funcs(text_tmpl.FuncMap(fm))
	t.html.Funcs(html_tmpl.FuncMap(fm))
	return t
}

func (t *Template) ExecuteTextString(name string, data interface{}) (string, error) {
	var buf bytes.Buffer
	err := t.ExecuteText(&buf, name, data)
	return buf.String(), err
}

func (t *Template) ExecuteText(w io.Writer, name string, data interface{}) error {
	return t.text.ExecuteTemplate(w, name, data)
}

func (t *Template) ExecuteHTMLString(name string, data interface{}) (string, error) {
	var buf bytes.Buffer
	err := t.ExecuteHTML(&buf, name, data)
	return buf.String(), err
}

func (t *Template) ExecuteHTML(w io.Writer, name string, data interface{}) error {
	return t.html.ExecuteTemplate(w, name, data)
}

var DefaultFuncs = FuncMap{
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"title": strings.Title,
	"match": regexp.MatchString,
	// replace performs regular expression replace-all on the input string.
	"replace": func(pattern, repl, text string) string {
		re := regexp.MustCompile(pattern)
		return re.ReplaceAllString(text, repl)
	},
	// safe marks a string as safe to be not HTML-escaped.
	"safe": func(s string) html_tmpl.HTML {
		return html_tmpl.HTML(s)
	},
	// commonLabels returns the labels that are equal across
	// all given alerts.
	"commonLabels": func(alerts model.Alerts) model.LabelSet {
		if len(alerts) < 1 {
			return model.LabelSet{}
		}
		common := alerts[0].Labels.Clone()

		for _, a := range alerts[1:] {
			for ln, lv := range common {
				if a.Labels[ln] != lv {
					delete(common, ln)
				}
			}
		}

		return common
	},
	// commonAnnotations returns the annotations that are equal across
	// all given alerts.
	"commonAnnotations": func(alerts model.Alerts) model.LabelSet {
		if len(alerts) < 1 {
			return model.LabelSet{}
		}
		common := alerts[0].Annotations.Clone()

		for _, a := range alerts[1:] {
			for ln, lv := range common {
				if a.Annotations[ln] != lv {
					delete(common, ln)
				}
			}
		}

		return common
	},
	// humanize returns a human-readable string representation of the value.
	"humanize": func(v float64) string {
		if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%.4g", v)
		}
		if math.Abs(v) >= 1 {
			prefix := ""
			for _, p := range []string{"k", "M", "G", "T", "P", "E", "Z", "Y"} {
				if math.Abs(v) < 1000 {
					break
				}
				prefix = p
				v /= 1000
			}
			return fmt.Sprintf("%.4g%s", v, prefix)
		}
		prefix := ""
		for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
			if math.Abs(v) >= 1 {
				break
			}
			prefix = p
			v *= 1000
		}
		return fmt.Sprintf("%.4g%s", v, prefix)
	},
	"humanize1024": func(v float64) string {
		if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%.4g", v)
		}
		prefix := ""
		for _, p := range []string{"ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"} {
			if math.Abs(v) < 1024 {
				break
			}
			prefix = p
			v /= 1024
		}
		return fmt.Sprintf("%.4g%s", v, prefix)
	},
	// duration returns a duration for the second value.
	"duration": func(v float64) time.Duration {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0
		}
		return time.Duration(v) * time.Nanosecond
	},
	// time returns a time representation of the second value.
	"time": func(v float64) time.Time {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return time.Time{}
		}
		return time.Unix(int64(v), 0)
	},
}
