package slackV2

import (
	"fmt"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/common/model"
	"github.com/satori/go.uuid"
	"github.com/slack-go/slack"
	"net/http"
	url2 "net/url"
	"strconv"
	"strings"
	"time"
)

func genGrafanaRenderUrl(dash string, panel string, org string) string {

	const unixMinute = 1000 * 60
	const unixSec = 1000

	url := ""

	if u, err := url2.Parse("https://grafana.adsrv.wtf/"); err == nil {
		u.Scheme = "https"
		u.Host = "grafana.adsrv.wtf"
		u.Path = "/render/d-solo/" + dash + "/"
		q := u.Query()
		q.Set("orgId", org)
		q.Set("from", strconv.FormatInt(time.Now().UnixMilli()-(unixMinute*60), 10))
		q.Set("to", strconv.FormatInt(time.Now().UnixMilli()-(unixSec*10), 10))
		q.Set("panelId", panel)
		q.Set("width", "1000")
		q.Set("height", "300")
		q.Set("tz", "Europe/Moscow")
		u.RawQuery = q.Encode()
		url = u.String()
	}
	return url
}
func genGrafanaUrl(dash string, panel string) string {
	DashUrl := ""

	if u, err := url2.Parse(""); err == nil {
		u.Scheme = "https"
		u.Host = "grafana.adsrv.wtf"
		u.Path = "d/" + dash
		if panel != "" {
			q := u.Query()
			q.Set("viewPanel", panel)
			u.RawQuery = q.Encode()
		}
		DashUrl = u.String()
	}
	return DashUrl
}
func urlMerger(kUrl string, pUrl string) string {
	imageLink := ""
	key := ""
	if u, err := url2.Parse(kUrl); err == nil {
		key = u.Path
	}

	trunc := []rune(key)
	key = string(trunc[len(trunc)-10:])

	if u2, err := url2.Parse(pUrl); err == nil {
		q := u2.Query()
		q.Set("pub_secret", key)
		u2.RawQuery = q.Encode()
		imageLink = u2.String()
	}
	return imageLink
}

func getUploadedImageUrl(url string, token config.Secret, grafanaToken config.Secret) string {

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+string(grafanaToken))
	response, err := client.Do(req)

	defer response.Body.Close()

	if err != nil {
		fmt.Printf("invalid req")
		return ""
	}

	if response.StatusCode != 200 {
		fmt.Printf("received non 200 response code")
		return ""
	}

	uuid := uuid.NewV4()
	fileName := strings.Replace(uuid.String(), "-", "", -1)
	api := slack.New(string(token))
	params := slack.FileUploadParameters{
		Reader:   response.Body,
		Filetype: "jpg",
		Filename: fileName + ".jpg",
	}
	image, err := api.UploadFile(params)

	if err != nil {
		fmt.Printf("UPLOAD ERROR - 1\nName: %s", image.Name)
		return ""
	}
	sharedUrl, _, _, err := api.ShareFilePublicURL(image.ID)

	if err != nil {
		fmt.Printf("SharedError :%v", sharedUrl)
		return ""
	}

	imageUrl := urlMerger(sharedUrl.PermalinkPublic, sharedUrl.URLPrivate)

	return imageUrl

}

func (n *Notifier) formatGrafanaMessage(data *template.Data) slack.Blocks {

	dashboardUid := ""
	panelId := ""
	orgId := ""
	grafanaValues := ""
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
		for _, v := range alert.Annotations.SortedPairs() {
			switch v.Name {

			case "__dashboardUid__":
				dashboardUid = v.Value
			case "__panelId__":
				panelId = v.Value
			case "orgid":
				orgId = v.Value
			case "__value_string__":
				grafanaValues = v.Value
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

		alertEditUrl := ""
		for _, alert := range data.Alerts {
			if alert.GeneratorURL != "" {
				alertEditUrl = alert.GeneratorURL
				break
			}
		}

		grafanaDashUrl := genGrafanaUrl(dashboardUid, "")
		grafanaPanelUrl := genGrafanaUrl(dashboardUid, panelId)

		fields := make([]*Field, 0)
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Env: %s*", strings.ToUpper(strings.Join(envs, ", ")))})
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Severety: %s*", strings.ToUpper(strings.Join(severity, ", ")))})
		if grafanaPanelUrl != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:design:Graph>*", grafanaPanelUrl)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf(":design:~Graph~")})
		}
		if grafanaDashUrl != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:chart_with_upwards_trend:Dash>*", grafanaDashUrl)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf(":chart_with_upwards_trend:~Dash~")})
		}

		if url != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:no_bell:Silence>*", url)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:no_bell:Silence>*", url)})
		}
		if alertEditUrl != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:gear:Edit>*", alertEditUrl)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*:gear:~Edit~")})
		}

		blocks = append(blocks, Block{Type: slack.MBTSection, Fields: fields})
	}

	if len(firing) > 0 && len(resolved) > 0 {
		fields := make([]*Field, 0)
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Firing:* `%s`", strings.Join(firing, ", "))})
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Resolved:* `%s`", strings.Join(resolved, ", "))})
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
		grafanaImageUrl := genGrafanaRenderUrl(dashboardUid, panelId, orgId)
		slackImageUrl := getUploadedImageUrl(grafanaImageUrl, n.conf.UserToken, n.conf.GrafanaToken)
		blocks = append(blocks, Block{Type: slack.MBTImage, ImageURL: slackImageUrl, AltText: "inspiration"})

	}

	{
		block := Block{Type: slack.MBTContext, Elements: make([]*Element, 0)}

		if val := getMapValue(data.CommonAnnotations, "summary"); len(val) > 0 {
			block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Summary:* %s", val)})
		} else {
			summary := make([]string, 0)
			for _, al := range data.Alerts {
				if val, ok := al.Annotations["summary"]; ok && len(val) > 0 {
					summary = append(summary, val)
				}
			}
			summary = mergeSameMessages(summary)
			if len(summary) > 0 {
				block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Summary:* %s", cut(strings.Join(summary, ";\n"), 500))})
			}
		}

		if val := getMapValue(data.CommonAnnotations, "description"); len(val) > 0 {
			block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Description:* %s Value: %v", val, grafanaValues)})
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
