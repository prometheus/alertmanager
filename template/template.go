package template

import (
	text_tmpl "html/template"
	html_tmpl "text/template"

	"bytes"
	"io"
	"strings"

	"github.com/prometheus/alertmanager/config"
)

type Template struct {
	text *text_tmpl.Template
	html *html_tmpl.Template
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

func (t *Template) ApplyConfig(conf *config.Config) {
	var (
		tt = text_tmpl.New("")
		ht = html_tmpl.New("")
	)

	for _, tf := range conf.Templates {
		tt = text_tmpl.Must(tt.ParseGlob(tf))
		ht = html_tmpl.Must(ht.ParseGlob(tf))
	}

	t.text = tt
	t.html = ht

	t.funcs(DefaultFuncs)
}
