package slackV2

import (
	"fmt"
	"net/http"
	url2 "net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/satori/go.uuid"
	"github.com/slack-go/slack"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
)

func genGrafanaRenderUrl(grafanaUrl string, grafanaTZ string, org string, dash string, panel string) (string, error) {

	const fromShift = -time.Hour
	const toShift = -time.Second * 10
	const imageWidth = "999"
	const imageHeight = "333"
	const urlPath = "/render/d-solo/"

	if grafanaUrl == "" {
		return "", fmt.Errorf("grafanaUrl is empty")
	}

	u, err := url2.Parse(grafanaUrl)
	if err != nil {
		return "", err
	}

	u.Path = path.Join(u.Path, urlPath, dash)
	q := u.Query()
	q.Set("orgId", org)
	q.Set("from", strconv.Itoa(int(time.Now().Add(fromShift).UnixMilli())))
	q.Set("to", strconv.Itoa(int(time.Now().Add(toShift).UnixMilli())))
	q.Set("panelId", panel)
	q.Set("width", imageWidth)
	q.Set("height", imageHeight)
	q.Set("tz", grafanaTZ)
	u.RawQuery = EncodeUrlArgs(q)
	return u.String(), nil

}

func genGrafanaUrl(grafanaUrl string, org string, dash string, panel string) (string, error) {

	if grafanaUrl == "" {
		return "", fmt.Errorf("grafanaUrl is empty")
	}

	u, err := url2.Parse(grafanaUrl)
	if err != nil {
		return "", err
	}

	u.Path = path.Join(u.Path, "/d/"+dash)
	q := u.Query()
	q.Set("orgId", org)
	if panel != "" {
		q.Set("viewPanel", panel)
	}
	u.RawQuery = EncodeUrlArgs(q)
	return u.String(), nil
}

func urlMerger(publicUrl string, privateUrl string) (string, error) {
	u, err := url2.Parse(publicUrl)
	if err != nil {
		return "", err
	}

	trunc := []rune(u.Path)
	key := string(trunc[len(trunc)-10:])

	u, err = url2.Parse(privateUrl)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("pub_secret", key)
	u.RawQuery = EncodeUrlArgs(q)

	return u.String(), nil
}

func getUploadedImageUrl(url string, token config.Secret, grafanaToken config.Secret) (string, error) {
	const imageExtension = "jpg"
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+string(grafanaToken))

	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request status code %d != %d", response.StatusCode, http.StatusOK)
	}

	fileName := fmt.Sprintf("%s.%s", strings.Replace(uuid.NewV4().String(), "-", "", -1), imageExtension)
	api := slack.New(string(token))
	params := slack.FileUploadParameters{
		Reader:   response.Body,
		Filetype: "jpg",
		Filename: fileName,
	}

	image, err := api.UploadFile(params)
	if err != nil {
		return "", fmt.Errorf("upload error, image: %s, error: %w", image.Name, err)
	}

	sharedUrl, _, _, err := api.ShareFilePublicURL(image.ID)
	if err != nil {
		return "", fmt.Errorf("share error: %w", err)
	}

	imageUrl, err := urlMerger(sharedUrl.PermalinkPublic, sharedUrl.URLPrivate)
	if err != nil {
		return "", fmt.Errorf("url merge error: %w", err)
	}

	return imageUrl, nil

}

func (n *Notifier) formatGrafanaMessage(data *template.Data) slack.Blocks {
	dashboardUid := ""
	panelId := ""
	orgId := ""
	grafanaValues := ""
	runBook := ""
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
			switch strings.ToLower(v.Name) {
			case strings.ToLower("__dashboardUid__"):
				dashboardUid = v.Value
			case strings.ToLower("__panelId__"):
				panelId = v.Value
			case strings.ToLower("OrgID"):
				orgId = v.Value
			case strings.ToLower("__value_string__"):
				grafanaValues = v.Value
			case strings.ToLower("runbook_url"):
				runBook = v.Value
			}
		}
	}

	severity = UniqStr(severity)
	resolved = UniqStr(resolved)
	firing = UniqStr(firing)
	envs = UniqStr(envs)

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
			urlParsed.RawQuery = EncodeUrlArgs(args)
			url = urlParsed.String()
			url = strings.Replace(url, "%23", "#", 1)
		}

		alertEditUrl := ""
		for _, alert := range data.Alerts {
			if alert.GeneratorURL != "" {
				if urlParsed, err := url2.Parse(alert.GeneratorURL); err == nil {
					args := urlParsed.Query()
					args.Add("orgId", orgId)
					urlParsed.RawQuery = EncodeUrlArgs(args)
					alertEditUrl = urlParsed.String()
					break
				}
			}
		}

		//Header
		blocks = append(blocks, Block{Type: slack.MBTHeader, Text: &Text{Type: slack.PlainTextType, Text: getMapValue(data.CommonLabels, "alertname")}})

		//Divider
		//blocks = append(blocks, Block{Type: slack.MBTDivider})

		//Env and severity
		fields := make([]*Field, 0)
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Env: %s*", strings.ToUpper(strings.Join(envs, ", ")))})
		fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*Severety: %s*", strings.ToUpper(strings.Join(severity, ", ")))})

		//Buttons
		if url, err := genGrafanaUrl(n.conf.GrafanaUrl, orgId, dashboardUid, panelId); err == nil {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:chart_with_upwards_trend:Panel>*", url)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf(":chart_with_upwards_trend:~Panel~")})
		}

		if url != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:no_bell:Silence>*", url)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:no_bell:Silence>*", url)})
		}
		if url, err := genGrafanaUrl(n.conf.GrafanaUrl, orgId, dashboardUid, ""); err == nil {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:dashboard:Dash>*", url)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf(":dashboard:~Dash~")})
		}
		if alertEditUrl != "" {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|:gear:Edit>*", alertEditUrl)})
		} else {
			fields = append(fields, &Field{Type: slack.MarkdownType, Text: fmt.Sprintf("*:gear:~Edit~")})
		}

		blocks = append(blocks, Block{Type: slack.MBTSection, Fields: fields})
	}

	//Firing > Resolved
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

	//GrafanaImage
	if imageUrl, err := genGrafanaRenderUrl(n.conf.GrafanaUrl, n.conf.GrafanaTZ, orgId, dashboardUid, panelId); err == nil {
		if slackImageUrl, err := getUploadedImageUrl(imageUrl, n.conf.UserToken, n.conf.GrafanaToken); err == nil {
			blocks = append(blocks, Block{Type: slack.MBTImage, ImageURL: slackImageUrl, AltText: "inspiration"})
		}
	}

	//Summary Description and Metrics
	{
		block := Block{Type: slack.MBTContext, Elements: make([]*Element, 0)}

		if (grafanaValues != "[no value]") || (grafanaValues != "") {
			regexpForParseMetric := regexp.MustCompile(`(?m) labels={[a-zA-z0-9=:,_@{ -.]+} value=`)
			valueStringCollection := regexpForParseMetric.ReplaceAllString(grafanaValues, ", value=")
			regexpForParseParams := regexp.MustCompile(`(?m)metric='(?P<name>.*)', value=(?P<value>.*)`)

			grafanaMapParams := make(map[string]string)
			for _, parsedCollection := range strings.Split(valueStringCollection, "], [ ") {
				match := regexpForParseParams.FindStringSubmatch(parsedCollection)
				if len(match) >= 3 {
					grafanaMapParams[match[1]] = match[2]
				}
			}
			if valueStringCollection != "" {
				block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Metric:* %s\n", valueStringCollection)})
			}
		}

		if val := getMapValue(data.CommonAnnotations, "description"); len(val) > 0 {
			block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Description:* %s\n", val)})
		} else {
			for _, al := range data.Alerts {
				if val, ok := al.Annotations["description"]; ok && len(val) > 0 {
					block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Description:* %s\n", val)})
					break
				}
			}
		}

		if val := getMapValue(data.CommonAnnotations, "summary"); len(val) > 0 {
			if runBook != "" {
				block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|Summary:>* %s", runBook, val)})
			} else {
				block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Summary:* %s", val)})
			}
		} else {
			summary := make([]string, 0)
			for _, al := range data.Alerts {
				if val, ok := al.Annotations["summary"]; ok && len(val) > 0 {
					summary = append(summary, val)
				}
			}
			summary = mergeSameMessages(summary)
			if len(summary) > 0 {
				if runBook != "" {
					block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*<%s|Summary:>* %s", runBook, cut(strings.Join(summary, ";\n"), 500))})
				} else {
					block.Elements = append(block.Elements, &Element{Type: slack.MarkdownType, Text: fmt.Sprintf("*Summary:* %s", cut(strings.Join(summary, ";\n"), 500))})
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
