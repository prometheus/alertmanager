### Alert
```json
[
    {
        "labels": {
            "alertname": "Test Alert Cvvtintokg.",
            "env": "Test",
            "group": "Ops",
            "name": "Hbcqqq dzrvxi dxvftrprop dotm.",
            "severity": "critical"
        },
        "annotations": {
            "message": "Yqlvc qdxjqdw kmedspse adfbai rmw klqeosrv fgcronx usjp embhqt.\"\nNpctkymw mkezjf iynfo qae wdvfw fmxev.Jyhbt kcfepqp dhesgstrt vpvashto yfv wvpd gadptu hbzgjwi ctt rbg."
        },
        "startsAt": "2023-05-04T06:42:15.904Z",
        "endsAt": "2023-05-04T07:42:15.904Z",
        "generatorURL": "https://www.github.com"
    }
]
```
### Template 
```gotemplate
{{ define "dingTalk.json" }}
    {{ $key_map := (`{"alertname":"Alert Name","name":"Service Name","group":"Group","service":"Service","severity":"Severity","message":"Message","instance":"Instance Name"}`|toMap) }}
    {{ $alert_name := "" }}
    {{ $alert_status := "" }}
    {{ if eq .Status "firing" }}
        {{ $alert_status = printf "[Firing: %d]" (.Alerts.Firing | len)  }}
    {{ else if eq .Status "resolved" }}
        {{ $alert_status = printf "[Resolved: %d]" (.Alerts.Resolved | len)  }}
    {{ else }}
        {{ $alert_status =(.Status | toUpper) }}
    {{ end }}
    {{ range .GroupLabels.SortedPairs }} {{ if eq .Name "alertname" }}{{ $alert_name = (.Value) }}{{end}} {{ end }}
    {{ $alert_title := (printf "%s %s"  $alert_status $alert_name ) }}
    {{ $alert_msg := (printf "### %s %s"  $alert_status $alert_name ) }}
    {{ range .Alerts.Firing }}
        {{ range .Labels.SortedPairs }}
            {{ if (ne (index $key_map .Name) "") }}
                {{ $alert_msg = (printf "%s\n\n> %s: %s" $alert_msg (index $key_map .Name) .Value ) }}
            {{else}}
                {{ $alert_msg = (printf "%s\n\n> %s: %s" $alert_msg .Name .Value ) }}
            {{end}}
        {{ end }}
        {{ range .Annotations.SortedPairs }}
            {{ if (ne (index $key_map .Name) "") }}
                {{ $alert_msg = (printf "%s\n\n> %s: %s" $alert_msg (index $key_map .Name) .Value ) }}
            {{else}}
                {{ $alert_msg = (printf "%s\n\n> %s: %s" $alert_msg .Name .Value ) }}
            {{end}}
        {{ end }}
        {{ $alert_msg = (printf "%s\n\n> [View Data](%s)" $alert_msg .GeneratorURL ) }}
    {{ end }}
    {
         "msgtype": "markdown",
         "markdown": {
             "title":{{ $alert_title|toJson }},
             "text": {{ $alert_msg|toJson }}
         },
        "at": {
            "atMobiles": [
            ],
            "isAtAll": false
        }
     }
{{ end }}
```
### Alertmanager Config
```yaml
receivers:
  - name: 'web.hook'
    webhook_configs:
      - url: 'https://oapi.dingtalk.com/robot/send?access_token=<token>'
        json: '{{ template "dingTalk.json" . }}'
```

### Webhook Content
```
{
  "msgtype": "markdown",
  "markdown": {
    "title": "[Firing: 1] Test Alert Eijwywi wtuplkvno uix.",
    "text": "### [Firing: 1] Test Alert Eijwywi wtuplkvno uix.\n\n\u003e Alert Name: Test Alert Eijwywi wtuplkvno uix.\n\n\u003e env: Test\n\n\u003e Group: Ops\n\n\u003e Service Name: Tdjafh qbhhpmyt aeekyhrhr tdjdbbap.\n\n\u003e Severity: critical\n\n\u003e Message: Bstg aenfwxmvk qbihjaxo.\"\nEoog xzrgupeeel yqyndiw appvaw wkkbre dhemrscsg egvofxxvs xqxj.Egwmievu xysgytr idwufuw.\n\n\u003e [View Data](https://www.github.com)"
  },
  "at": {
    "atMobiles": [],
    "isAtAll": false
  }
}
```

### Feishu message
![image](./dingTalk_message.png)