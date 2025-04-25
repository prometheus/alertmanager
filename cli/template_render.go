// Copyright 2021 Prometheus Team
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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/template"
)

var defaultData = template.Data{
	Receiver: "receiver",
	Status:   "alertstatus",
	Alerts: template.Alerts{
		template.Alert{
			Status: string(model.AlertFiring),
			Labels: template.KV{
				"label1":          "value1",
				"label2":          "value2",
				"instance":        "foo.bar:1234",
				"commonlabelkey1": "commonlabelvalue1",
				"commonlabelkey2": "commonlabelvalue2",
			},
			Annotations: template.KV{
				"annotation1":          "value1",
				"annotation2":          "value2",
				"commonannotationkey1": "commonannotationvalue1",
				"commonannotationkey2": "commonannotationvalue2",
			},
			StartsAt:     time.Now().Add(-5 * time.Minute),
			EndsAt:       time.Now(),
			GeneratorURL: "https://generatorurl.com",
			Fingerprint:  "fingerprint1",
		},
		template.Alert{
			Status: string(model.AlertResolved),
			Labels: template.KV{
				"foo":             "bar",
				"baz":             "qux",
				"commonlabelkey1": "commonlabelvalue1",
				"commonlabelkey2": "commonlabelvalue2",
			},
			Annotations: template.KV{
				"aaa":                  "bbb",
				"ccc":                  "ddd",
				"commonannotationkey1": "commonannotationvalue1",
				"commonannotationkey2": "commonannotationvalue2",
			},
			StartsAt:     time.Now().Add(-10 * time.Minute),
			EndsAt:       time.Now(),
			GeneratorURL: "https://generatorurl.com",
			Fingerprint:  "fingerprint2",
		},
	},
	GroupLabels: template.KV{
		"grouplabelkey1": "grouplabelvalue1",
		"grouplabelkey2": "grouplabelvalue2",
	},
	CommonLabels: template.KV{
		"commonlabelkey1": "commonlabelvalue1",
		"commonlabelkey2": "commonlabelvalue2",
	},
	CommonAnnotations: template.KV{
		"commonannotationkey1": "commonannotationvalue1",
		"commonannotationkey2": "commonannotationvalue2",
	},
	ExternalURL: "https://example.com",
}

type templateRenderCmd struct {
	templateFilesGlobs []string
	templateType       string
	templateText       string
	templateData       *os.File
}

func configureTemplateRenderCmd(cc *kingpin.CmdClause) {
	var (
		c         = &templateRenderCmd{}
		renderCmd = cc.Command("render", "Render a given definition in a template file to standard output.")
	)

	renderCmd.Flag("template.glob", "Glob of paths that will be expanded and used for rendering.").Required().StringsVar(&c.templateFilesGlobs)
	renderCmd.Flag("template.text", "The template that will be rendered.").Required().StringVar(&c.templateText)
	renderCmd.Flag("template.type", "The type of the template. Can be either text (default) or html.").EnumVar(&c.templateType, "html", "text")
	renderCmd.Flag("template.data", "Full path to a file which contains the data of the alert(-s) with which the --template.text will be rendered. Must be in JSON. File must be formatted according to the following layout: https://pkg.go.dev/github.com/prometheus/alertmanager/template#Data. If none has been specified then a predefined, simple alert will be used for rendering.").FileVar(&c.templateData)

	renderCmd.Action(execWithTimeout(c.render))
}

func (c *templateRenderCmd) render(ctx context.Context, _ *kingpin.ParseContext) error {
	tmpl, err := template.FromGlobs(c.templateFilesGlobs)
	if err != nil {
		return err
	}

	f := tmpl.ExecuteTextString
	if c.templateType == "html" {
		f = tmpl.ExecuteHTMLString
	}

	var data template.Data
	if c.templateData == nil {
		data = defaultData
	} else {
		content, err := io.ReadAll(c.templateData)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(content, &data); err != nil {
			return err
		}
	}

	rendered, err := f(c.templateText, data)
	if err != nil {
		return err
	}

	fmt.Print(rendered)
	return nil
}
