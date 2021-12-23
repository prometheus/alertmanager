package slackV2

import (
	"fmt"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/common/model"
	"github.com/slack-go/slack"
	url2 "net/url"
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
	blocks = append(blocks, Block{Type: slack.MBTDivider})

	{
		url := ""
		if urlParsed, err := url2.Parse(data.ExternalURL); err == nil {
			urlParsed.Path = "/#/silences/new"
			args := urlParsed.Query()
			filters := make([]string, 0)
			for _, v := range data.CommonLabels.SortedPairs() {
				filters = append(filters, fmt.Sprintf("%s=\"%s\"", v.Name, v.Value))
			}
			args.Add("filter", fmt.Sprintf("{%s}", strings.Join(filters, ",")))
			urlParsed.RawQuery = args.Encode()
			url = urlParsed.String()
			url = strings.Replace(url, "%23", "#", 1)
		}

		graphUrl := ""
		for _, alert := range data.Alerts {
			if alert.GeneratorURL != "" {
				graphUrl = alert.GeneratorURL
				break
			}
		}

		fields := make([]*Field, 0)
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Env: %s*", strings.ToUpper(strings.Join(envs, ", ")))})
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Severety: %s*", strings.ToUpper(strings.Join(severity, ", ")))})
		if graphUrl != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:chart_with_upwards_trend:Graph>*", graphUrl)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf(":chart_with_upwards_trend:~Graph~")})
		}
		if url != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:no_bell:Silence>*", url)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*:no_bell:~Silence~")})
		}

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

		if val := getMapValue(data.CommonAnnotations, "summary"); len(val) > 0 {
			block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Summary:* %s", val)})
		} else {
			for _, al := range data.Alerts {
				if val, ok := al.Annotations["summary"]; ok && len(val) > 0 {
					block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Summary:* %s", val)})
					break
				}
			}
		}

		if val := getMapValue(data.CommonAnnotations, "description"); len(val) > 0 {
			block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Description:* %s", val)})
		} else {
			for _, al := range data.Alerts {
				if val, ok := al.Annotations["description"]; ok && len(val) > 0 {
					block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Description:* %s", val)})
					break
				}
			}
		}

		if len(block.Elements) > 0 {
			blocks = append(blocks, block)
		}
	}

	result := slack.Blocks{BlockSet: blocks}

	return result
}
