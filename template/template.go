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

	html_tmpl "html/template"
	text_tmpl "text/template"
)

type Template struct {
	text *text_tmpl.Template
	html *html_tmpl.Template
}

func FromGlobs(paths ...string) (*Template, error) {
	t := &Template{
		text: text_tmpl.New("").Option("missingkey=zero"),
		html: html_tmpl.New("").Option("missingkey=zero"),
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

	return t, nil
}

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
