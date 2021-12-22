package slackV2

import (
	"fmt"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/common/model"
	"github.com/slack-go/slack"
	"strings"
)

type Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type Element struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type Field struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type Block struct {
	Type     slack.MessageBlockType `json:"type"`
	Text     *Text                  `json:"text,omitempty"`
	Fields   []*Field               `json:"fields,omitempty"`
	Elements []*Element             `json:"elements,omitempty"`
}

func (b Block) BlockType() slack.MessageBlockType {
	return b.Type
}

func (n *Notifier) formatMessage(data *template.Data) slack.Blocks {
	firing := make([]string, 0)
	resolved := make([]string, 0)
	severity := make([]string, 0)
	envs := make([]string, 0)

	blocks := make([]slack.Block, 0)

	for _, alert := range data.Alerts {
		for _, v := range alert.Labels.SortedPairs() {
			switch v.Name {
			case "host_name":
				switch model.AlertStatus(alert.Status) {
				case model.AlertFiring:
					firing = append(firing, v.Value)
				case model.AlertResolved:
					resolved = append(resolved, v.Value)
				}
			case "severity":
				severity = append(severity, v.Value)
			case "env":
				envs = append(envs, v.Value)
			}
		}
	}

	severity = UniqStr(severity)
	resolved = UniqStr(resolved)
	firing = UniqStr(firing)
	envs = UniqStr(envs)

	blocks = append(blocks, Block{Type: slack.MBTHeader, Text: &Text{Type: slack.PlainTextType, Text: getMapValue(data.CommonLabels, "alertname")}})

	{
		url := n.conf.AlertmanagerUrl.Copy()
		url.Path = "/#/silences/new"
		args := url.Query()
		filters := make([]string, 0)
		for _, v := range data.CommonLabels.SortedPairs() {
			filters = append(filters, fmt.Sprintf("%s=\"%s\"", v.Name, v.Value))
		}
		args.Add("filter", fmt.Sprintf("{%s}", strings.Join(filters, ",")))
		fmt.Printf("%s", args)
		url.RawQuery = args.Encode()

		fields := make([]*Field, 0)
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Env: %s*", strings.ToUpper(strings.Join(envs, ", ")))})
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Severety: %s*", strings.ToUpper(strings.Join(severity, ", ")))})
		if getMapValue(data.CommonLabels, "GeneratorURL") != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:chart_with_upwards_trend:Graph>*", getMapValue(data.CommonLabels, "GeneratorURL"))})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf(":chart_with_upwards_trend:~Graph~")})
		}
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:no_bell:Silence>*", url.String())})
		blocks = append(blocks, Block{Type: slack.MBTSection, Fields: fields})
	}

	if len(firing) > 0 && len(resolved) > 0 {
		fields := make([]*Field, 0)
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Firing:* `%s`", strings.Join(firing, ", "))})
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Resolved:* `%s`", strings.Join(firing, ", "))})
		blocks = append(blocks, Block{Type: slack.MBTSection, Fields: fields})
	} else {
		fields := make([]*Field, 0)
		if len(resolved) > 0 {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Resolved: *`%s`", strings.Join(resolved, ", "))})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Firing: *`%s`", strings.Join(firing, ", "))})
		}
		blocks = append(blocks, Block{Type: slack.MBTSection, Fields: fields})
	}

	{
		block := Block{Type: slack.MBTContext, Elements: make([]*Element, 0)}

		if summary := getMapValue(data.CommonAnnotations, "description"); len(summary) > 0 {
			block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Summary:* %s", summary)})
		}
		if desc := getMapValue(data.CommonAnnotations, "description"); len(desc) > 0 {
			block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Description:* %s", desc)})
		}
		blocks = append(blocks, block)
	}

	result := slack.Blocks{BlockSet: blocks}

	return result
}
