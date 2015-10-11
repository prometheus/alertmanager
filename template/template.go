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
	"io"
	"strings"

	text_tmpl "html/template"
	html_tmpl "text/template"
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

var DefaultFuncs = FuncMap{
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
}

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
