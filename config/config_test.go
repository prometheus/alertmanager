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
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
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
	_, err := Load(in)

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
		t.Fatalf("no error returned, expected:\n%q", expected)
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
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestReceiverExistsForDeepSubRoute(t *testing.T) {
	in := `
route:
    receiver: team-X
    routes:
      - match:
          foo: bar
        routes:
        - match:
            foo: bar
          receiver: nonexistent

receivers:
- name: 'team-X'
`
	_, err := Load(in)

	expected := "undefined receiver \"nonexistent\" used in route"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
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
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestMuteTimeExists(t *testing.T) {
	in := `
route:
    receiver: team-Y
    routes:
    -  match:
        severity: critical
       mute_time_intervals:
       - business_hours

receivers:
- name: 'team-Y'
`
	_, err := Load(in)

	expected := "undefined time interval \"business_hours\" used in route"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestActiveTimeExists(t *testing.T) {
	in := `
route:
    receiver: team-Y
    routes:
    -  match:
        severity: critical
       active_time_intervals:
       - business_hours

receivers:
- name: 'team-Y'
`
	_, err := Load(in)

	expected := "undefined time interval \"business_hours\" used in route"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestTimeIntervalHasName(t *testing.T) {
	in := `
time_intervals:
- name:
  time_intervals:
  - times:
     - start_time: '09:00'
       end_time: '17:00'

receivers:
- name: 'team-X-mails'

route:
  receiver: 'team-X-mails'
  routes:
  -  match:
      severity: critical
     mute_time_intervals:
     - business_hours
`
	_, err := Load(in)

	expected := "missing name in time interval"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestMuteTimeNoDuplicates(t *testing.T) {
	in := `
mute_time_intervals:
- name: duplicate
  time_intervals:
  - times:
     - start_time: '09:00'
       end_time: '17:00'
- name: duplicate
  time_intervals:
  - times:
     - start_time: '10:00'
       end_time: '14:00'

receivers:
- name: 'team-X-mails'

route:
  receiver: 'team-X-mails'
  routes:
  -  match:
      severity: critical
     mute_time_intervals:
     - business_hours
`
	_, err := Load(in)

	expected := "mute time interval \"duplicate\" is not unique"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
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
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestWildcardGroupByWithOtherGroupByLabels(t *testing.T) {
	in := `
route:
  group_by: ['alertname', 'cluster', '...']
  receiver: team-X-mails
receivers:
- name: 'team-X-mails'
`
	_, err := Load(in)

	expected := "cannot have wildcard group_by (`...`) and other other labels at the same time"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestGroupByInvalidLabel(t *testing.T) {
	in := `
route:
  group_by: ['-invalid-']
  receiver: team-X-mails
receivers:
- name: 'team-X-mails'
`
	_, err := Load(in)

	expected := "invalid label name \"-invalid-\" in group_by list"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
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
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestRootRouteNoMuteTimes(t *testing.T) {
	in := `
mute_time_intervals:
- name: my_mute_time
  time_intervals:
  - times:
     - start_time: '09:00'
       end_time: '17:00'

receivers:
- name: 'team-X-mails'

route:
  receiver: 'team-X-mails'
  mute_time_intervals:
  - my_mute_time
`
	_, err := Load(in)

	expected := "root route must not have any mute time intervals"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestRootRouteNoActiveTimes(t *testing.T) {
	in := `
time_intervals:
- name: my_active_time
  time_intervals:
  - times:
     - start_time: '09:00'
       end_time: '17:00'

receivers:
- name: 'team-X-mails'

route:
  receiver: 'team-X-mails'
  active_time_intervals:
  - my_active_time
`
	_, err := Load(in)

	expected := "root route must not have any active time intervals"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestRootRouteHasNoMatcher(t *testing.T) {
	testCases := []struct {
		name string
		in   string
	}{
		{
			name: "Test deprecated matchers on root route not allowed",
			in: `
route:
  receiver: 'team-X'
  match:
    severity: critical
receivers:
- name: 'team-X'
`,
		},
		{
			name: "Test matchers not allowed on root route",
			in: `
route:
  receiver: 'team-X'
  matchers:
    - severity=critical
receivers:
- name: 'team-X'
`,
		},
	}
	expected := "root route must not have any matchers"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(tc.in)

			if err == nil {
				t.Fatalf("no error returned, expected:\n%q", expected)
			}
			if err.Error() != expected {
				t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
			}
		})
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
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestGroupIntervalIsGreaterThanZero(t *testing.T) {
	in := `
route:
    receiver: team-X-mails
    group_interval: 0s

receivers:
- name: 'team-X-mails'
`
	_, err := Load(in)

	expected := "group_interval cannot be zero"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestRepeatIntervalIsGreaterThanZero(t *testing.T) {
	in := `
route:
    receiver: team-X-mails
    repeat_interval: 0s

receivers:
- name: 'team-X-mails'
`
	_, err := Load(in)

	expected := "repeat_interval cannot be zero"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%q", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%q\ngot:\n%q", expected, err.Error())
	}
}

func TestHideConfigSecrets(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	// String method must not reveal authentication credentials.
	s := c.String()
	if strings.Count(s, "<secret>") != 13 || strings.Contains(s, "mysecret") {
		t.Fatal("config's String method reveals authentication credentials.")
	}
}

func TestJSONMarshal(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml")
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

	require.JSONEq(t, `{"S":"<secret>"}`, string(c), "Secret not properly elided.")
}

func TestJSONUnmarshal(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	_, err = json.Marshal(c)
	if err != nil {
		t.Fatal("JSON Marshaling failed:", err)
	}
}

func TestMarshalIdempotency(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	marshaled, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal("YAML Marshaling failed:", err)
	}

	c = new(Config)
	if err := yaml.Unmarshal(marshaled, c); err != nil {
		t.Fatal("YAML Unmarshaling failed:", err)
	}
}

func TestGroupByAllNotMarshaled(t *testing.T) {
	in := `
route:
    receiver: team-X-mails
    group_by: [...]

receivers:
- name: 'team-X-mails'
`
	c, err := Load(in)
	if err != nil {
		t.Fatal("load failed:", err)
	}

	dat, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal("YAML Marshaling failed:", err)
	}

	if strings.Contains(string(dat), "groupbyall") {
		t.Fatal("groupbyall found in config file")
	}
}

func TestEmptyFieldsAndRegex(t *testing.T) {
	boolFoo := true
	var regexpFoo amcommoncfg.Regexp
	require.NoError(t, yaml.Unmarshal([]byte(`"^(foo1|foo2|baz)$"`), &regexpFoo))

	expectedConf := Config{
		Global: &GlobalConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{
				FollowRedirects: true,
				EnableHTTP2:     true,
			},
			ResolveTimeout:  model.Duration(5 * time.Minute),
			SMTPSmarthost:   HostPort{Host: "localhost", Port: "25"},
			SMTPFrom:        "alertmanager@example.org",
			SlackAPIURL:     (*amcommoncfg.SecretURL)(amcommoncfg.MustParseURL("http://slack.example.com/")),
			SMTPRequireTLS:  true,
			PagerdutyURL:    amcommoncfg.MustParseURL("https://events.pagerduty.com/v2/enqueue"),
			OpsGenieAPIURL:  amcommoncfg.MustParseURL("https://api.opsgenie.com/"),
			WeChatAPIURL:    amcommoncfg.MustParseURL("https://qyapi.weixin.qq.com/cgi-bin/"),
			VictorOpsAPIURL: amcommoncfg.MustParseURL("https://alert.victorops.com/integrations/generic/20131114/alert/"),
			TelegramAPIUrl:  amcommoncfg.MustParseURL("https://api.telegram.org"),
			WebexAPIURL:     amcommoncfg.MustParseURL("https://webexapis.com/v1/messages"),
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
			GroupByStr: []string{
				"alertname",
				"cluster",
				"service",
			},
			GroupByAll: false,
			Routes: []*Route{
				{
					Receiver: "team-X-mails",
					MatchRE: map[string]amcommoncfg.Regexp{
						"service": regexpFoo,
					},
				},
			},
		},
		Receivers: []Receiver{
			{
				Name: "team-X-mails",
				EmailConfigs: []*EmailConfig{
					{
						To:         "team-X+alerts@example.org",
						From:       "alertmanager@example.org",
						Smarthost:  HostPort{Host: "localhost", Port: "25"},
						HTML:       "{{ template \"email.default.html\" . }}",
						RequireTLS: &boolFoo,
					},
				},
			},
		},
	}

	// Load a non-empty configuration to ensure that all fields are overwritten.
	// See https://github.com/prometheus/alertmanager/issues/1649.
	_, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	config, err := LoadFile("testdata/conf.empty-fields.yml")
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

func TestGlobalAndLocalHTTPConfig(t *testing.T) {
	config, err := LoadFile("testdata/conf.http-config.good.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf-http-config.good.yml", err)
	}

	if config.Global.HTTPConfig.FollowRedirects {
		t.Fatalf("global HTTP config should not follow redirects")
	}

	if !config.Receivers[0].SlackConfigs[0].HTTPConfig.FollowRedirects {
		t.Fatalf("global HTTP config should follow redirects")
	}
}

func TestSMTPHello(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	const refValue = "host.example.org"
	hostName := c.Global.SMTPHello
	if hostName != refValue {
		t.Errorf("Invalid SMTP Hello hostname: %s\nExpected: %s", hostName, refValue)
	}
}

func TestSMTPBothPasswordAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.smtp-both-password-and-file.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.smtp-both-password-and-file.yml", err)
	}
	if err.Error() != "at most one of smtp_auth_password & smtp_auth_password_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of auth_password & auth_password_file must be configured", err.Error())
	}
}

func TestSMTPNoUsernameOrPassword(t *testing.T) {
	_, err := LoadFile("testdata/conf.smtp-no-username-or-password.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.smtp-no-username-or-password.yml", err)
	}
}

func TestGlobalAndLocalSMTPPassword(t *testing.T) {
	config, err := LoadFile("testdata/conf.smtp-password-global-and-local.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.smtp-password-global-and-local.yml", err)
	}

	require.Equal(t, "/tmp/globaluserpassword", config.Receivers[0].EmailConfigs[0].AuthPasswordFile, "first email should use password file /tmp/globaluserpassword")
	require.Emptyf(t, config.Receivers[0].EmailConfigs[0].AuthPassword, "password field should be empty when file provided")

	require.Equal(t, "/tmp/localuser1password", config.Receivers[0].EmailConfigs[1].AuthPasswordFile, "second email should use password file /tmp/localuser1password")
	require.Emptyf(t, config.Receivers[0].EmailConfigs[1].AuthPassword, "password field should be empty when file provided")

	require.Equal(t, Secret("mysecret"), config.Receivers[0].EmailConfigs[2].AuthPassword, "third email should use password mysecret")
	require.Emptyf(t, config.Receivers[0].EmailConfigs[2].AuthPasswordFile, "file field should be empty when password provided")
}

func TestGroupByAll(t *testing.T) {
	c, err := LoadFile("testdata/conf.group-by-all.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.group-by-all.yml", err)
	}

	if !c.Route.GroupByAll {
		t.Errorf("Invalid group by all param: expected to by true")
	}
}

func TestVictorOpsDefaultAPIKey(t *testing.T) {
	conf, err := LoadFile("testdata/conf.victorops-default-apikey.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.victorops-default-apikey.yml", err)
	}

	defaultKey := conf.Global.VictorOpsAPIKey
	overrideKey := Secret("qwe456")
	if defaultKey != conf.Receivers[0].VictorOpsConfigs[0].APIKey {
		t.Fatalf("Invalid victorops key: %s\nExpected: %s", conf.Receivers[0].VictorOpsConfigs[0].APIKey, defaultKey)
	}
	if overrideKey != conf.Receivers[1].VictorOpsConfigs[0].APIKey {
		t.Errorf("Invalid victorops key: %s\nExpected: %s", conf.Receivers[0].VictorOpsConfigs[0].APIKey, string(overrideKey))
	}
}

func TestVictorOpsDefaultAPIKeyFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.victorops-default-apikey-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.victorops-default-apikey-file.yml", err)
	}

	defaultKey := conf.Global.VictorOpsAPIKeyFile
	overrideKey := "/override_file"
	if defaultKey != conf.Receivers[0].VictorOpsConfigs[0].APIKeyFile {
		t.Fatalf("Invalid VictorOps key_file: %s\nExpected: %s", conf.Receivers[0].VictorOpsConfigs[0].APIKeyFile, defaultKey)
	}
	if overrideKey != conf.Receivers[1].VictorOpsConfigs[0].APIKeyFile {
		t.Errorf("Invalid VictorOps key_file: %s\nExpected: %s", conf.Receivers[0].VictorOpsConfigs[0].APIKeyFile, overrideKey)
	}
}

func TestVictorOpsBothAPIKeyAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.victorops-both-file-and-apikey.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.victorops-both-file-and-apikey.yml", err)
	}
	if err.Error() != "at most one of victorops_api_key & victorops_api_key_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of victorops_api_key & victorops_api_key_file must be configured", err.Error())
	}
}

func TestVictorOpsNoAPIKey(t *testing.T) {
	_, err := LoadFile("testdata/conf.victorops-no-apikey.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.victorops-no-apikey.yml", err)
	}
	if err.Error() != "no global VictorOps API Key set" {
		t.Errorf("Expected: %s\nGot: %s", "no global VictorOps API Key set", err.Error())
	}
}

func TestOpsGenieDefaultAPIKey(t *testing.T) {
	conf, err := LoadFile("testdata/conf.opsgenie-default-apikey.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.opsgenie-default-apikey.yml", err)
	}

	defaultKey := conf.Global.OpsGenieAPIKey
	if commoncfg.Secret(defaultKey) != conf.Receivers[0].OpsGenieConfigs[0].APIKey {
		t.Fatalf("Invalid OpsGenie key: %s\nExpected: %s", conf.Receivers[0].OpsGenieConfigs[0].APIKey, defaultKey)
	}
	if commoncfg.Secret(defaultKey) == conf.Receivers[1].OpsGenieConfigs[0].APIKey {
		t.Errorf("Invalid OpsGenie key: %s\nExpected: %s", conf.Receivers[0].OpsGenieConfigs[0].APIKey, "qwe456")
	}
}

func TestOpsGenieDefaultAPIKeyFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.opsgenie-default-apikey-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.opsgenie-default-apikey-file.yml", err)
	}

	defaultKey := conf.Global.OpsGenieAPIKeyFile
	if defaultKey != conf.Receivers[0].OpsGenieConfigs[0].APIKeyFile {
		t.Fatalf("Invalid OpsGenie key_file: %s\nExpected: %s", conf.Receivers[0].OpsGenieConfigs[0].APIKeyFile, defaultKey)
	}
	if defaultKey == conf.Receivers[1].OpsGenieConfigs[0].APIKeyFile {
		t.Errorf("Invalid OpsGenie key_file: %s\nExpected: %s", conf.Receivers[0].OpsGenieConfigs[0].APIKeyFile, "/override_file")
	}
}

func TestOpsGenieBothAPIKeyAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.opsgenie-both-file-and-apikey.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.opsgenie-both-file-and-apikey.yml", err)
	}
	if err.Error() != "at most one of opsgenie_api_key & opsgenie_api_key_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of opsgenie_api_key & opsgenie_api_key_file must be configured", err.Error())
	}
}

func TestOpsGenieNoAPIKey(t *testing.T) {
	_, err := LoadFile("testdata/conf.opsgenie-no-apikey.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.opsgenie-no-apikey.yml", err)
	}
	if err.Error() != "no global OpsGenie API Key set either inline or in a file" {
		t.Errorf("Expected: %s\nGot: %s", "no global OpsGenie API Key set either inline or in a file", err.Error())
	}
}

func TestOpsGenieDeprecatedTeamSpecified(t *testing.T) {
	_, err := LoadFile("testdata/conf.opsgenie-default-apikey-old-team.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.opsgenie-default-apikey-old-team.yml", err)
	}

	const expectedErr = `yaml: unmarshal errors:
  line 16: field teams not found in type opsgenie.plain`
	if err.Error() != expectedErr {
		t.Errorf("Expected: %s\nGot: %s", expectedErr, err.Error())
	}
}

func TestSlackBothAPIURLAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.slack-both-file-and-url.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.slack-both-file-and-url.yml", err)
	}
	if err.Error() != "at most one of slack_api_url & slack_api_url_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of slack_api_url & slack_api_url_file must be configured", err.Error())
	}
}

func TestSlackNoAPIURL(t *testing.T) {
	_, err := LoadFile("testdata/conf.slack-no-api-url.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.slack-no-api-url.yml", err)
	}
	if err.Error() != "no global Slack API URL set either inline or in a file" {
		t.Errorf("Expected: %s\nGot: %s", "no global Slack API URL set either inline or in a file", err.Error())
	}
}

func TestSlackGlobalAPIURLFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.slack-default-api-url-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.slack-default-api-url-file.yml", err)
	}

	// no override
	firstConfig := conf.Receivers[0].SlackConfigs[0]
	if firstConfig.APIURLFile != "/global_file" || firstConfig.APIURL != nil {
		t.Fatalf("Invalid Slack URL file: %s\nExpected: %s", firstConfig.APIURLFile, "/global_file")
	}

	// override the file
	secondConfig := conf.Receivers[0].SlackConfigs[1]
	if secondConfig.APIURLFile != "/override_file" || secondConfig.APIURL != nil {
		t.Fatalf("Invalid Slack URL file: %s\nExpected: %s", secondConfig.APIURLFile, "/override_file")
	}

	// override the global file with an inline URL
	thirdConfig := conf.Receivers[0].SlackConfigs[2]
	if thirdConfig.APIURL.String() != "http://mysecret.example.com/" || thirdConfig.APIURLFile != "" {
		t.Fatalf("Invalid Slack URL: %s\nExpected: %s", thirdConfig.APIURL.String(), "http://mysecret.example.com/")
	}
}

func TestValidSNSConfig(t *testing.T) {
	_, err := LoadFile("testdata/conf.sns-topic-arn.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.sns-topic-arn.yml\"", err)
	}
}

func TestInvalidSNSConfig(t *testing.T) {
	_, err := LoadFile("testdata/conf.sns-invalid.yml")
	if err == nil {
		t.Fatalf("expected error with missing fields on SNS config")
	}
	const expectedErr = `must provide either a Target ARN, Topic ARN, or Phone Number for SNS config`
	if err.Error() != expectedErr {
		t.Errorf("Expected: %s\nGot: %s", expectedErr, err.Error())
	}
}

func TestUnmarshalHostPort(t *testing.T) {
	for _, tc := range []struct {
		in string

		exp     HostPort
		jsonOut string
		yamlOut string
		err     bool
	}{
		{
			in:  `""`,
			exp: HostPort{},
			yamlOut: `""
`,
			jsonOut: `""`,
		},
		{
			in:  `"localhost:25"`,
			exp: HostPort{Host: "localhost", Port: "25"},
			yamlOut: `localhost:25
`,
			jsonOut: `"localhost:25"`,
		},
		{
			in:  `":25"`,
			exp: HostPort{Host: "", Port: "25"},
			yamlOut: `:25
`,
			jsonOut: `":25"`,
		},
		{
			in:  `"localhost"`,
			err: true,
		},
		{
			in:  `"localhost:"`,
			err: true,
		},
	} {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			hp := HostPort{}
			err := yaml.Unmarshal([]byte(tc.in), &hp)
			if tc.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, hp)

			b, err := yaml.Marshal(&hp)
			require.NoError(t, err)
			require.Equal(t, tc.yamlOut, string(b))

			b, err = json.Marshal(&hp)
			require.NoError(t, err)
			require.Equal(t, tc.jsonOut, string(b))
		})
	}
}

func TestNilRegexp(t *testing.T) {
	for _, tc := range []struct {
		file   string
		errMsg string
	}{
		{
			file:   "testdata/conf.nil-match_re-route.yml",
			errMsg: "invalid_label",
		},
		{
			file:   "testdata/conf.nil-source_match_re-inhibition.yml",
			errMsg: "invalid_source_label",
		},
		{
			file:   "testdata/conf.nil-target_match_re-inhibition.yml",
			errMsg: "invalid_target_label",
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			_, err := os.Stat(tc.file)
			require.NoError(t, err)

			_, err = LoadFile(tc.file)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

func TestTelegramUnmarshal(t *testing.T) {
	in := `
route:
  receiver: test
receivers:
- name: test
  telegram_configs:
  - chat_id: 1234
    bot_token: secret
`
	var c Config
	err := yaml.Unmarshal([]byte(in), &c)
	require.NoError(t, err)

	require.Len(t, c.Receivers, 1)
	require.Len(t, c.Receivers[0].TelegramConfigs, 1)

	require.Equal(t, "https://api.telegram.org", c.Receivers[0].TelegramConfigs[0].APIUrl.String())
	require.Equal(t, commoncfg.Secret("secret"), c.Receivers[0].TelegramConfigs[0].BotToken)
	require.Equal(t, int64(1234), c.Receivers[0].TelegramConfigs[0].ChatID)
	require.Equal(t, "HTML", c.Receivers[0].TelegramConfigs[0].ParseMode)
}

func TestMSTeamsV2RejectsMissingConfig(t *testing.T) {
	in := `
route:
  receiver: teams
receivers:
- name: teams
  msteamsv2_configs:
  -
`
	_, err := Load(in)
	require.ErrorContains(t, err, "missing msteamsv2 config")
}

// TestMSTeamsV2GlobalProxyInheritedWhenNoLocalHTTPConfig checks that the global
// proxy_url is used when the receiver has no http_config set.
func TestMSTeamsV2GlobalProxyInheritedWhenNoLocalHTTPConfig(t *testing.T) {
	in := `
global:
  http_config:
    proxy_url: "http://proxy.example.com:3128"

route:
  receiver: teams

receivers:
- name: teams
  msteamsv2_configs:
  - webhook_url: "https://example.webhook.office.com/webhookb2/test"
`
	cfg, err := Load(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rc := cfg.Receivers[0].MSTeamsV2Configs[0]
	if rc.HTTPConfig == nil {
		t.Fatal("expected HTTPConfig to be non-nil")
	}
	if rc.HTTPConfig.ProxyURL.URL == nil {
		t.Fatal("expected global proxy_url to be inherited")
	}
	if got := rc.HTTPConfig.ProxyURL.String(); got != "http://proxy.example.com:3128" {
		t.Errorf("expected proxy_url %q, got %q", "http://proxy.example.com:3128", got)
	}
}

// TestMSTeamsV2GlobalProxyInheritedWhenPartialHTTPConfig checks that the global
// proxy_url is used when the receiver sets a partial http_config with no proxy.
func TestMSTeamsV2GlobalProxyInheritedWhenPartialHTTPConfig(t *testing.T) {
	in := `
global:
  http_config:
    proxy_url: "http://proxy.example.com:3128"

route:
  receiver: teams

receivers:
- name: teams
  msteamsv2_configs:
  - webhook_url: "https://example.webhook.office.com/webhookb2/test"
    http_config:
      follow_redirects: true
`
	cfg, err := Load(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rc := cfg.Receivers[0].MSTeamsV2Configs[0]
	if rc.HTTPConfig == nil {
		t.Fatal("expected HTTPConfig to be non-nil")
	}
	if rc.HTTPConfig.ProxyURL.URL == nil {
		t.Fatal("expected global proxy_url to be inherited into partial http_config")
	}
	if got := rc.HTTPConfig.ProxyURL.String(); got != "http://proxy.example.com:3128" {
		t.Errorf("expected proxy_url %q, got %q", "http://proxy.example.com:3128", got)
	}
}

// TestMSTeamsV2LocalProxyTakesPrecedence checks that a receiver-level proxy_url
// is not overwritten by the global one.
func TestMSTeamsV2LocalProxyTakesPrecedence(t *testing.T) {
	in := `
global:
  http_config:
    proxy_url: "http://global-proxy.example.com:3128"

route:
  receiver: teams

receivers:
- name: teams
  msteamsv2_configs:
  - webhook_url: "https://example.webhook.office.com/webhookb2/test"
    http_config:
      proxy_url: "http://local-proxy.example.com:8080"
`
	cfg, err := Load(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rc := cfg.Receivers[0].MSTeamsV2Configs[0]
	if rc.HTTPConfig.ProxyURL.URL == nil {
		t.Fatal("expected proxy_url to be set")
	}
	if got := rc.HTTPConfig.ProxyURL.String(); got != "http://local-proxy.example.com:8080" {
		t.Errorf("expected local proxy_url %q, got %q", "http://local-proxy.example.com:8080", got)
	}
}

func TestMSTeamsV2PreservesReceiverHTTPSettingsWithoutGlobal(t *testing.T) {
	const in = `
global:
  http_config: null
route:
  receiver: teams
receivers:
- name: teams
  msteamsv2_configs:
  - webhook_url: "https://example.com/teams"
  - webhook_url: "https://example.com/teams"
    http_config:
      follow_redirects: false
`
	cfg, err := Load(in)
	require.NoError(t, err)
	require.Nil(t, cfg.Global.HTTPConfig)

	configs := cfg.Receivers[0].MSTeamsV2Configs
	require.Len(t, configs, 2)
	require.Nil(t, configs[0].HTTPConfig)
	require.NotNil(t, configs[1].HTTPConfig)
	require.False(t, configs[1].HTTPConfig.FollowRedirects)
	require.Nil(t, configs[1].HTTPConfig.ProxyURL.URL)
}

func TestMSTeamsV2EnvironmentProxyTakesPrecedence(t *testing.T) {
	const in = `
global:
  http_config:
    proxy_url: "http://proxy.example.com:3128"
route:
  receiver: teams
receivers:
- name: teams
  msteamsv2_configs:
  - webhook_url: "https://example.com/teams"
    http_config:
      proxy_from_environment: true
`
	cfg, err := Load(in)
	require.NoError(t, err)

	httpConfig := cfg.Receivers[0].MSTeamsV2Configs[0].HTTPConfig
	require.NotNil(t, httpConfig)
	require.True(t, httpConfig.ProxyFromEnvironment)
	require.Nil(t, httpConfig.ProxyURL.URL)
	require.NoError(t, httpConfig.Validate())
}
