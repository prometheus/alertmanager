// Copyright 2016 Prometheus Team
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

package config

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestLoadEmptyString(t *testing.T) {

	var in string
	_, err := Load(in)

	expected := "no route provided in config"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestDefaultReceiverExists(t *testing.T) {
	in := `
route:
   group_wait: 30s
`
	conf := &Config{}
	err := yaml.Unmarshal([]byte(in), conf)

	expected := "root route must specify a default receiver"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
	}
}

func TestReceiverNameIsUnique(t *testing.T) {
	in := `
route:
    receiver: team-X

receivers:
- name: 'team-X'
- name: 'team-X'
`
	_, err := Load(in)

	expected := "notification config name \"team-X\" is not unique"

	if err == nil {
		t.Fatalf("no error returned, expeceted:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}

}

func TestReceiverExists(t *testing.T) {
	in := `
route:
    receiver: team-X

receivers:
- name: 'team-Y'
`
	_, err := Load(in)

	expected := "undefined receiver \"team-X\" used in route"

	if err == nil {
		t.Fatalf("no error returned, expeceted:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}

}

func TestReceiverHasName(t *testing.T) {
	in := `
route:

receivers:
- name: ''
`
	_, err := Load(in)

	expected := "missing name in receiver"

	if err == nil {
		t.Fatalf("no error returned, expeceted:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}

}

func TestGroupByHasNoDuplicatedLabels(t *testing.T) {
	in := `
route:
  group_by: ['alertname', 'cluster', 'service', 'cluster']

receivers:
- name: 'team-X-mails'
`
	_, err := Load(in)

	expected := "duplicated label \"cluster\" in group_by"

	if err == nil {
		t.Fatalf("no error returned, expeceted:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}

}

func TestRootRouteExists(t *testing.T) {
	in := `
receivers:
- name: 'team-X-mails'
`
	_, err := Load(in)

	expected := "no routes provided"

	if err == nil {
		t.Fatalf("no error returned, expeceted:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}

}

func TestRootRouteHasNoMatcher(t *testing.T) {
	in := `
route:
  receiver: 'team-X'
  match:
    severity: critical

receivers:
- name: 'team-X'
`
	_, err := Load(in)

	expected := "root route must not have any matchers"

	if err == nil {
		t.Fatalf("no error returned, expeceted:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}

}

func TestContinueErrorInRouteRoot(t *testing.T) {
	in := `
route:
    receiver: team-X-mails
    continue: true

receivers:
- name: 'team-X-mails'
`
	_, err := Load(in)

	expected := "cannot have continue in root route"

	if err == nil {
		t.Fatalf("no error returned, expeceted:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}

}

func TestHideConfigSecrets(t *testing.T) {
	c, _, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	// String method must not reveal authentication credentials.
	s := c.String()
	secretRe := regexp.MustCompile("<secret>")
	matches := secretRe.FindAllStringIndex(s, -1)
	if len(matches) != 14 || strings.Contains(s, "mysecret") {
		t.Fatal("config's String method reveals authentication credentials.")
	}
}

func TestJSONMarshal(t *testing.T) {
	c, _, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	_, err = json.Marshal(c)
	if err != nil {
		t.Fatal("JSON Marshaling failed:", err)
	}
}

func TestJSONMarshalSecret(t *testing.T) {
	test := struct {
		S Secret
	}{
		S: Secret("test"),
	}

	c, err := json.Marshal(test)
	if err != nil {
		t.Fatal(err)
	}

	// u003c -> "<"
	// u003e -> ">"
	require.Equal(t, "{\"S\":\"\\u003csecret\\u003e\"}", string(c), "Secret not properly elided.")
}

func TestJSONUnmarshalMarshaled(t *testing.T) {
	c, _, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	plainCfg, err := json.Marshal(c)
	if err != nil {
		t.Fatal("JSON Marshaling failed:", err)
	}

	cfg := Config{}
	err = json.Unmarshal(plainCfg, &cfg)
	if err != nil {
		t.Fatal("JSON Unmarshaling failed:", err)
	}
}

func TestEmptyFieldsAndRegex(t *testing.T) {
	boolFoo := true
	var regexpFoo Regexp
	regexpFoo.Regexp, _ = regexp.Compile("^(?:^(foo1|foo2|baz)$)$")

	var expectedConf = Config{

		Global: &GlobalConfig{
			ResolveTimeout:   model.Duration(5 * time.Minute),
			SMTPSmarthost:    "localhost:25",
			SMTPFrom:         "alertmanager@example.org",
			HipchatAuthToken: "mysecret",
			HipchatAPIURL:    "https://hipchat.foobar.org/",
			SlackAPIURL:      "mysecret",
			SMTPRequireTLS:   true,
			PagerdutyURL:     "https://events.pagerduty.com/v2/enqueue",
			OpsGenieAPIURL:   "https://api.opsgenie.com/",
			WeChatAPIURL:     "https://qyapi.weixin.qq.com/cgi-bin/",
			VictorOpsAPIURL:  "https://alert.victorops.com/integrations/generic/20131114/alert/",
		},

		Templates: []string{
			"/etc/alertmanager/template/*.tmpl",
		},
		Route: &Route{
			Receiver: "team-X-mails",
			GroupBy: []model.LabelName{
				"alertname",
				"cluster",
				"service",
			},
			Routes: []*Route{
				{
					Receiver: "team-X-mails",
					MatchRE: map[string]Regexp{
						"service": regexpFoo,
					},
				},
			},
		},
		Receivers: []*Receiver{
			{
				Name: "team-X-mails",
				EmailConfigs: []*EmailConfig{
					{
						To:         "team-X+alerts@example.org",
						From:       "alertmanager@example.org",
						Smarthost:  "localhost:25",
						HTML:       "{{ template \"email.default.html\" . }}",
						RequireTLS: &boolFoo,
					},
				},
			},
		},
	}

	config, _, err := LoadFile("testdata/conf.empty-fields.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.empty-fields.yml", err)
	}

	configGot, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal("YAML Marshaling failed:", err)
	}

	configExp, err := yaml.Marshal(expectedConf)
	if err != nil {
		t.Fatalf("%s", err)
	}

	if !reflect.DeepEqual(configGot, configExp) {
		t.Fatalf("%s: unexpected config result: \n\n%s\n expected\n\n%s", "testdata/conf.empty-fields.yml", configGot, configExp)
	}
}

func TestSMTPHello(t *testing.T) {
	c, _, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	const refValue = "host.example.org"
	var hostName = c.Global.SMTPHello
	if hostName != refValue {
		t.Errorf("Invalid SMTP Hello hostname: %s\nExpected: %s", hostName, refValue)
	}
}

func TestVictorOpsDefaultAPIKey(t *testing.T) {
	conf, _, err := LoadFile("testdata/conf.victorops-default-apikey.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.victorops-default-apikey.yml", err)
	}

	var defaultKey = conf.Global.VictorOpsAPIKey
	if defaultKey != conf.Receivers[0].VictorOpsConfigs[0].APIKey {
		t.Errorf("Invalid victorops key: %s\nExpected: %s", conf.Receivers[0].VictorOpsConfigs[0].APIKey, defaultKey)
	}
	if defaultKey == conf.Receivers[1].VictorOpsConfigs[0].APIKey {
		t.Errorf("Invalid victorops key: %s\nExpected: %s", conf.Receivers[0].VictorOpsConfigs[0].APIKey, "qwe456")
	}
}

func TestVictorOpsNoAPIKey(t *testing.T) {
	_, _, err := LoadFile("testdata/conf.victorops-no-apikey.yml")
	if err == nil {
		t.Errorf("Expected an error parsing %s: %s", "testdata/conf.victorops-no-apikey.yml", err)
	}
	if err.Error() != "no global VictorOps API Key set" {
		t.Errorf("Expected: %s\nGot: %s", "no global VictorOps API Key set", err.Error())
	}
}
