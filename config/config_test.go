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
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
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

	expected := "cannot have wildcard group_by (`...`) and other labels at the same time"

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

func TestMarshalRegexpWithNilValue(t *testing.T) {
	r := &Regexp{}

	out, err := json.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "null", string(out))

	out, err = yaml.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "null\n", string(out))
}

func TestUnmarshalEmptyRegexp(t *testing.T) {
	b := []byte(`""`)

	{
		var re Regexp
		err := json.Unmarshal(b, &re)
		require.NoError(t, err)
		require.Equal(t, regexp.MustCompile("^(?:)$"), re.Regexp)
		require.Empty(t, re.original)
	}

	{
		var re Regexp
		err := yaml.Unmarshal(b, &re)
		require.NoError(t, err)
		require.Equal(t, regexp.MustCompile("^(?:)$"), re.Regexp)
		require.Empty(t, re.original)
	}
}

func TestUnmarshalNullRegexp(t *testing.T) {
	input := []byte(`null`)

	{
		var re Regexp
		err := json.Unmarshal(input, &re)
		require.NoError(t, err)
		require.Empty(t, re.original)
	}

	{
		var re Regexp
		err := yaml.Unmarshal(input, &re) // Interestingly enough, unmarshalling `null` in YAML doesn't even call UnmarshalYAML.
		require.NoError(t, err)
		require.Nil(t, re.Regexp)
		require.Empty(t, re.original)
	}
}

func TestMarshalEmptyMatchers(t *testing.T) {
	r := Matchers{}

	out, err := json.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "[]", string(out))

	out, err = yaml.Marshal(r)
	require.NoError(t, err)
	require.Equal(t, "[]\n", string(out))
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
	regexpFoo := Regexp{
		Regexp:   regexp.MustCompile("^(?:^(foo1|foo2|baz)$)$"),
		original: "^(foo1|foo2|baz)$",
	}

	expectedConf := Config{
		Global: &GlobalConfig{
			HTTPConfig: &commoncfg.HTTPClientConfig{
				FollowRedirects: true,
				EnableHTTP2:     true,
			},
			ResolveTimeout: model.Duration(5 * time.Minute),
			SMTPSmarthost:  HostPort{Host: "localhost", Port: "25"},
			SMTPFrom:       "alertmanager@example.org",
			SMTPTLSConfig: &commoncfg.TLSConfig{
				InsecureSkipVerify: false,
			},
			SlackAPIURL:      (*amcommoncfg.SecretURL)(amcommoncfg.MustParseURL("http://slack.example.com/")),
			SlackAppURL:      amcommoncfg.MustParseURL("https://slack.com/api/chat.postMessage"),
			SMTPRequireTLS:   true,
			PagerdutyURL:     amcommoncfg.MustParseURL("https://events.pagerduty.com/v2/enqueue"),
			OpsGenieAPIURL:   amcommoncfg.MustParseURL("https://api.opsgenie.com/"),
			WeChatAPIURL:     amcommoncfg.MustParseURL("https://qyapi.weixin.qq.com/cgi-bin/"),
			VictorOpsAPIURL:  amcommoncfg.MustParseURL("https://alert.victorops.com/integrations/generic/20131114/alert/"),
			TelegramAPIUrl:   amcommoncfg.MustParseURL("https://api.telegram.org"),
			WebexAPIURL:      amcommoncfg.MustParseURL("https://webexapis.com/v1/messages"),
			RocketchatAPIURL: amcommoncfg.MustParseURL("https://open.rocket.chat/"),
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
					MatchRE: map[string]Regexp{
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
						TLSConfig: &commoncfg.TLSConfig{
							InsecureSkipVerify: false,
						},
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

func TestEmptyConfigOfIntegration(t *testing.T) {
	baseConfigTmpl := `
global:
route:
  receiver: 'test-receiver'
receivers:
- name: 'test-receiver'
  %s:
  -
`

	tests := []struct {
		integration string // The key name in YAML (e.g., webhook_configs)
		expectedErr string // The unique error message expected for this integration
	}{
		{
			integration: "discord_configs",
			expectedErr: "missing discord config",
		},
		{
			integration: "email_configs",
			expectedErr: "missing email config",
		},
		{
			integration: "incidentio_configs",
			expectedErr: "missing incidentio config",
		},
		{
			integration: "pagerduty_configs",
			expectedErr: "missing pagerduty config",
		},
		{
			integration: "webhook_configs",
			expectedErr: "missing webhook config",
		},
		{
			integration: "pushover_configs",
			expectedErr: "missing pushover config",
		},
		{
			integration: "victorops_configs",
			expectedErr: "missing victorops config",
		},
		{
			integration: "sns_configs",
			expectedErr: "missing sns config",
		},
		{
			integration: "telegram_configs",
			expectedErr: "missing telegram config",
		},
		{
			integration: "webex_configs",
			expectedErr: "missing webex config",
		},
		{
			integration: "msteams_configs",
			expectedErr: "missing msteams config",
		},
		{
			integration: "msteamsv2_configs",
			expectedErr: "missing msteamsv2 config",
		},
		{
			integration: "jira_configs",
			expectedErr: "missing jira config",
		},
		{
			integration: "mattermost_configs",
			expectedErr: "missing mattermost config",
		},
		{
			integration: "slack_configs",
			expectedErr: "no Slack API URL nor App token set either inline or in a file",
		},
		{
			integration: "opsgenie_configs",
			expectedErr: "no global OpsGenie API Key set either inline or in a file",
		},
		{
			integration: "wechat_configs",
			expectedErr: "no global Wechat Api Secret set either inline or in a file",
		},
		{
			integration: "rocketchat_configs",
			expectedErr: "no global Rocketchat TokenID set either inline or in a file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.integration, func(t *testing.T) {
			in := fmt.Sprintf(baseConfigTmpl, tc.integration)
			_, err := Load(in)
			require.Error(t, err, "Expected empty configuration to be an error for %s", tc.integration)
			require.ErrorContains(t, err, tc.expectedErr)
		})
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

	require.Equal(t, commoncfg.Secret("mysecret"), config.Receivers[0].EmailConfigs[2].AuthPassword, "third email should use password mysecret")
	require.Emptyf(t, config.Receivers[0].EmailConfigs[2].AuthPasswordFile, "file field should be empty when password provided")

	require.Equal(t, commoncfg.Secret("myprecious"), config.Receivers[0].EmailConfigs[3].AuthSecret, "fourth email should use secret myprecious")

	require.Equal(t, "/tmp/localuser4secret", config.Receivers[0].EmailConfigs[4].AuthSecretFile, "fifth email should use secret file /tmp/localuser4secret")
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
	overrideKey := commoncfg.Secret("qwe456")
	if defaultKey != conf.Receivers[0].VictorOpsConfigs[0].APIKey {
		t.Fatalf("Invalid victorops key: %s\nExpected: %s", conf.Receivers[0].VictorOpsConfigs[0].APIKey, defaultKey)
	}
	if overrideKey != conf.Receivers[1].VictorOpsConfigs[0].APIKey {
		t.Errorf("Invalid victorops key: %s\nExpected: %s", conf.Receivers[1].VictorOpsConfigs[0].APIKey, string(overrideKey))
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
		t.Errorf("Invalid VictorOps key_file: %s\nExpected: %s", conf.Receivers[1].VictorOpsConfigs[0].APIKeyFile, overrideKey)
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

func TestTelegramDefaultBotToken(t *testing.T) {
	conf, err := LoadFile("testdata/conf.telegram-default-bot-token.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.telegram-default-bot-token.yml", err)
	}

	defaultBotToken := conf.Global.TelegramBotToken
	overrideBotToken := commoncfg.Secret("qwe456")
	if defaultBotToken != conf.Receivers[0].TelegramConfigs[0].BotToken {
		t.Fatalf("Invalid telegram bot token: %s\nExpected: %s", conf.Receivers[0].TelegramConfigs[0].BotToken, defaultBotToken)
	}
	if overrideBotToken != conf.Receivers[1].TelegramConfigs[0].BotToken {
		t.Errorf("Invalid telegram bot token: %s\nExpected: %s", conf.Receivers[1].TelegramConfigs[0].BotToken, string(overrideBotToken))
	}
}

func TestTelegramDefaultBotTokenFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.telegram-default-bot-token-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.telegram-default-bot-token-file.yml", err)
	}

	defaultBotToken := conf.Global.TelegramBotTokenFile
	overrideBotToken := "/override_file"
	if defaultBotToken != conf.Receivers[0].TelegramConfigs[0].BotTokenFile {
		t.Fatalf("Invalid telegram bot token file: %s\nExpected: %s", conf.Receivers[0].TelegramConfigs[0].BotTokenFile, defaultBotToken)
	}
	if overrideBotToken != conf.Receivers[1].TelegramConfigs[0].BotTokenFile {
		t.Errorf("Invalid telegram bot token file: %s\nExpected: %s", conf.Receivers[1].TelegramConfigs[0].BotTokenFile, overrideBotToken)
	}
}

func TestTelegramBothBotTokenAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.telegram-both-bot-token-and-file.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.telegram-both-bot-token-and-file.yml", err)
	}
	if err.Error() != "at most one of telegram_bot_token & telegram_bot_token_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of telegram_bot_token & telegram_bot_token_file must be configured", err.Error())
	}
}

func TestTelegramValidReceiverBothBotTokenAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.telegram-valid-receiver-both-bot-token-and-file.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.telegram-valid-receiver-both-bot-token-and-file.yml", err)
	}
	if err.Error() != "at most one of telegram_bot_token & telegram_bot_token_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of telegram_bot_token & telegram_bot_token_file must be configured", err.Error())
	}
}

func TestTelegramNoBotToken(t *testing.T) {
	_, err := LoadFile("testdata/conf.telegram-no-bot-token.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.telegram-no-bot-token.yml", err)
	}
	if err.Error() != "missing bot_token or bot_token_file on telegram_config" {
		t.Errorf("Expected: %s\nGot: %s", "missing bot_token or bot_token_file on telegram_config", err.Error())
	}
}

func TestOpsGenieDefaultAPIKey(t *testing.T) {
	conf, err := LoadFile("testdata/conf.opsgenie-default-apikey.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.opsgenie-default-apikey.yml", err)
	}

	defaultKey := conf.Global.OpsGenieAPIKey
	if defaultKey != conf.Receivers[0].OpsGenieConfigs[0].APIKey {
		t.Fatalf("Invalid OpsGenie key: %s\nExpected: %s", conf.Receivers[0].OpsGenieConfigs[0].APIKey, defaultKey)
	}
	if defaultKey == conf.Receivers[1].OpsGenieConfigs[0].APIKey {
		t.Errorf("Invalid OpsGenie key: %s\nExpected: %s", conf.Receivers[1].OpsGenieConfigs[0].APIKey, "qwe456")
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
		t.Errorf("Invalid OpsGenie key_file: %s\nExpected: %s", conf.Receivers[1].OpsGenieConfigs[0].APIKeyFile, "/override_file")
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
  line 16: field teams not found in type config.plain`
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

func TestSlackBothAppTokenAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.slack-both-file-and-token.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.slack-both-file-and-token.yml", err)
	}
	if err.Error() != "at most one of slack_app_token & slack_app_token_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of slack_app_token & slack_app_token_file must be configured", err.Error())
	}
}

func TestSlackBothAppTokenAndAPIURL(t *testing.T) {
	_, err := LoadFile("testdata/conf.slack-both-url-and-token.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.slack-both-url-and-token.yml", err)
	}
	if err.Error() != "at most one of slack_app_token/slack_app_token_file & slack_api_url/slack_api_url_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of slack_app_token/slack_app_token_file & slack_api_url/slack_api_url_file must be configured", err.Error())
	}
}

func TestSlackUpdateMessageWebhookURL(t *testing.T) {
	_, err := LoadFile("testdata/conf.slack-update-message-and-webhook.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.slack-update-message-and-webhook", err)
	}
	if err.Error() != "update_message can only be used with bot tokens. api_url must be set to https://slack.com/api/chat.postMessage" {
		t.Errorf("Expected: %s\nGot: %s", "update_message can only be used with bot tokens. api_url must be set to https://slack.com/api/chat.postMessage", err.Error())
	}
}

func TestSlackGlobalAppToken(t *testing.T) {
	conf, err := LoadFile("testdata/conf.slack-default-app-token.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.slack-default-app-token.yml", err)
	}

	// no override
	defaultToken := conf.Global.SlackAppToken
	firstAuth := commoncfg.Authorization{
		Type:        "Bearer",
		Credentials: commoncfg.Secret(defaultToken),
	}
	firstConfig := conf.Receivers[0].SlackConfigs[0]
	if firstConfig.AppToken != defaultToken {
		t.Fatalf("Invalid Slack App token: %s\nExpected: %s", firstConfig.AppToken, defaultToken)
	}
	if firstConfig.APIURL.String() != conf.Global.SlackAppURL.String() {
		t.Fatalf("Expected API URL: %s\nGot: %s", conf.Global.SlackAppURL.String(), firstConfig.APIURL.String())
	}
	if firstConfig.HTTPConfig == nil || firstConfig.HTTPConfig.Authorization == nil {
		t.Fatalf("Error configuring Slack App authorization: %s", firstConfig.HTTPConfig)
	}
	if firstConfig.HTTPConfig.Authorization.Type != firstAuth.Type {
		t.Fatalf("Error configuring Slack App authorization type: %s\nExpected: %s", firstConfig.HTTPConfig.Authorization.Type, firstAuth.Type)
	}
	if firstConfig.HTTPConfig.Authorization.Credentials != firstAuth.Credentials {
		t.Fatalf("Error configuring Slack App authorization credentials: %s\nExpected: %s", firstConfig.HTTPConfig.Authorization.Credentials, firstAuth.Credentials)
	}

	// inline override
	inlineToken := "xoxb-1234-xxxxxx"
	secondAuth := commoncfg.Authorization{
		Type:        "Bearer",
		Credentials: commoncfg.Secret(inlineToken),
	}
	secondConfig := conf.Receivers[0].SlackConfigs[1]
	if secondConfig.AppToken != commoncfg.Secret(inlineToken) {
		t.Fatalf("Invalid Slack App token: %s\nExpected: %s", secondConfig.AppToken, inlineToken)
	}
	if secondConfig.HTTPConfig == nil || secondConfig.HTTPConfig.Authorization == nil {
		t.Fatalf("Error configuring Slack App authorization: %s", secondConfig.HTTPConfig)
	}
	if secondConfig.HTTPConfig.Authorization.Type != secondAuth.Type {
		t.Fatalf("Error configuring Slack App authorization type: %s\nExpected: %s", secondConfig.HTTPConfig.Authorization.Type, secondAuth.Type)
	}
	if secondConfig.HTTPConfig.Authorization.Credentials != secondAuth.Credentials {
		t.Fatalf("Error configuring Slack App authorization credentials: %s\nExpected: %s", secondConfig.HTTPConfig.Authorization.Credentials, secondAuth.Credentials)
	}

	// custom app url
	thirdConfig := conf.Receivers[0].SlackConfigs[2]
	if thirdConfig.AppURL.String() != "http://api.fakeslack.example/" {
		t.Fatalf("Invalid Slack URL: %s\nExpected: %s", thirdConfig.APIURL.String(), "http://mysecret.example.com/")
	}

	// workaround override
	workaroundToken := "xoxb-my-bot-token"
	fourthAuth := commoncfg.Authorization{
		Type:        "Bearer",
		Credentials: commoncfg.Secret(workaroundToken),
	}
	fourthConfig := conf.Receivers[0].SlackConfigs[3]
	if fourthConfig.AppToken != "" {
		t.Fatalf("Invalid Slack App token: %q\nExpected: %q", fourthConfig.AppToken, "")
	}
	if fourthConfig.HTTPConfig == nil || fourthConfig.HTTPConfig.Authorization == nil {
		t.Fatalf("Error configuring Slack App authorization: %s", fourthConfig.HTTPConfig)
	}
	if fourthConfig.HTTPConfig.Authorization.Type != fourthAuth.Type {
		t.Fatalf("Error configuring Slack App authorization type: %s\nExpected: %s", fourthConfig.HTTPConfig.Authorization.Type, fourthAuth.Type)
	}
	if fourthConfig.HTTPConfig.Authorization.Credentials != fourthAuth.Credentials {
		t.Fatalf("Error configuring Slack App authorization credentials: %s\nExpected: %s", fourthConfig.HTTPConfig.Authorization.Credentials, fourthAuth.Credentials)
	}

	// override the global file with an inline webhook URL
	apiURL := "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX"
	fifthConfig := conf.Receivers[0].SlackConfigs[4]
	if fifthConfig.APIURL.String() != apiURL || fifthConfig.APIURLFile != "" {
		t.Fatalf("Invalid Slack URL: %s\nExpected: %s", fifthConfig.APIURL.String(), apiURL)
	}
}

func TestSlackNoAPIURL(t *testing.T) {
	_, err := LoadFile("testdata/conf.slack-no-api-url-or-token.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.slack-no-api-url-or-token.yml", err)
	}
	if err.Error() != "no Slack API URL nor App token set either inline or in a file" {
		t.Errorf("Expected: %s\nGot: %s", "no Slack API URL nor App token set either inline or in a file", err.Error())
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

func TestRocketchatDefaultToken(t *testing.T) {
	conf, err := LoadFile("testdata/conf.rocketchat-default-token.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.rocketchat-default-token.yml", err)
	}

	defaultToken := conf.Global.RocketchatToken
	overrideToken := commoncfg.Secret("token456")
	if defaultToken != conf.Receivers[0].RocketchatConfigs[0].Token {
		t.Fatalf("Invalid rocketchat key: %s\nExpected: %s", string(*conf.Receivers[0].RocketchatConfigs[0].Token), string(*defaultToken))
	}
	if overrideToken != *conf.Receivers[1].RocketchatConfigs[0].Token {
		t.Errorf("Invalid rocketchat key: %s\nExpected: %s", string(*conf.Receivers[1].RocketchatConfigs[0].Token), string(overrideToken))
	}
}

func TestRocketchatDefaultTokenID(t *testing.T) {
	conf, err := LoadFile("testdata/conf.rocketchat-default-token.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.rocketchat-default-token.yml", err)
	}

	defaultTokenID := conf.Global.RocketchatTokenID
	overrideTokenID := commoncfg.Secret("id456")
	if defaultTokenID != conf.Receivers[0].RocketchatConfigs[0].TokenID {
		t.Fatalf("Invalid rocketchat key: %s\nExpected: %s", string(*conf.Receivers[0].RocketchatConfigs[0].TokenID), string(*defaultTokenID))
	}
	if overrideTokenID != *conf.Receivers[1].RocketchatConfigs[0].TokenID {
		t.Errorf("Invalid rocketchat key: %s\nExpected: %s", string(*conf.Receivers[1].RocketchatConfigs[0].TokenID), string(overrideTokenID))
	}
}

func TestRocketchatDefaultTokenFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.rocketchat-default-token-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.rocketchat-default-token-file.yml", err)
	}

	defaultTokenFile := conf.Global.RocketchatTokenFile
	overrideTokenFile := "/override_file"
	if defaultTokenFile != conf.Receivers[0].RocketchatConfigs[0].TokenFile {
		t.Fatalf("Invalid Rocketchat key_file: %s\nExpected: %s", conf.Receivers[0].RocketchatConfigs[0].TokenFile, defaultTokenFile)
	}
	if overrideTokenFile != conf.Receivers[1].RocketchatConfigs[0].TokenFile {
		t.Errorf("Invalid Rocketchat key_file: %s\nExpected: %s", conf.Receivers[1].RocketchatConfigs[0].TokenFile, overrideTokenFile)
	}
}

func TestRocketchatDefaultIDTokenFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.rocketchat-default-token-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.rocketchat-default-token-file.yml", err)
	}

	defaultTokenIDFile := conf.Global.RocketchatTokenIDFile
	overrideTokenIDFile := "/override_file"
	if defaultTokenIDFile != conf.Receivers[0].RocketchatConfigs[0].TokenIDFile {
		t.Fatalf("Invalid Rocketchat key_file: %s\nExpected: %s", conf.Receivers[0].RocketchatConfigs[0].TokenIDFile, defaultTokenIDFile)
	}
	if overrideTokenIDFile != conf.Receivers[1].RocketchatConfigs[0].TokenIDFile {
		t.Errorf("Invalid Rocketchat key_file: %s\nExpected: %s", conf.Receivers[1].RocketchatConfigs[0].TokenIDFile, overrideTokenIDFile)
	}
}

func TestRocketchatBothTokenAndTokenFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.rocketchat-both-token-and-tokenfile.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.rocketchat-both-token-and-tokenfile.yml", err)
	}
	if err.Error() != "at most one of rocketchat_token & rocketchat_token_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of rocketchat_token & rocketchat_token_file must be configured", err.Error())
	}
}

func TestRocketchatBothTokenIDAndTokenIDFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.rocketchat-both-tokenid-and-tokenidfile.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.rocketchat-both-tokenid-and-tokenidfile.yml", err)
	}
	if err.Error() != "at most one of rocketchat_token_id & rocketchat_token_id_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of rocketchat_token_id & rocketchat_token_id_file must be configured", err.Error())
	}
}

func TestRocketchatNoToken(t *testing.T) {
	_, err := LoadFile("testdata/conf.rocketchat-no-token.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.rocketchat-no-token.yml", err)
	}
	if err.Error() != "no global Rocketchat Token set either inline or in a file" {
		t.Errorf("Expected: %s\nGot: %s", "no global Rocketchat Token set either inline or in a file", err.Error())
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
		{
			in:  `"[fd12:3456:789a::1]:25"`,
			exp: HostPort{Host: "fd12:3456:789a::1", Port: "25"},
			yamlOut: `'[fd12:3456:789a::1]:25'
`,
			jsonOut: `"[fd12:3456:789a::1]:25"`,
		},
	} {
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

func TestSecretTemplURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid http URL",
			input:       `"http://example.com/webhook"`,
			expectError: false,
		},
		{
			name:        "invalid URL missing scheme",
			input:       `"example.com/webhook"`,
			expectError: true,
			errorMsg:    "unsupported scheme",
		},
		{
			name:        "invalid URL unsupported scheme",
			input:       `"ftp://example.com/webhook"`,
			expectError: true,
			errorMsg:    "unsupported scheme",
		},
		{
			name:        "templated URL is not validated",
			input:       `"http://example.com/{{ .GroupLabels.alertname }}"`,
			expectError: false,
		},
		{
			name:        "invalid URL with template is not validated",
			input:       `"not-a-url-{{ .GroupLabels.alertname }}"`,
			expectError: false,
		},
		{
			name:        "invalid template syntax",
			input:       `"http://example.com/{{ .Invalid"`,
			expectError: true,
			errorMsg:    "invalid template syntax",
		},
		{
			name:        "empty string",
			input:       `""`,
			expectError: false,
		},
		{
			name:        "secret token",
			input:       `"<secret>"`,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var u SecretTemplateURL
			err := yaml.Unmarshal([]byte(tc.input), &u)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSecretTemplURLMarshaling(t *testing.T) {
	t.Run("marshals to secret token by default", func(t *testing.T) {
		u := SecretTemplateURL("http://example.com/secret")

		yamlOut, err := yaml.Marshal(&u)
		require.NoError(t, err)
		require.YAMLEq(t, "<secret>\n", string(yamlOut))

		jsonOut, err := json.Marshal(&u)
		require.NoError(t, err)
		require.JSONEq(t, `"<secret>"`, string(jsonOut))
	})

	t.Run("marshals actual value when MarshalSecretValue is true", func(t *testing.T) {
		commoncfg.MarshalSecretValue = true
		defer func() { commoncfg.MarshalSecretValue = false }()

		u := SecretTemplateURL("http://example.com/secret")

		yamlOut, err := yaml.Marshal(&u)
		require.NoError(t, err)
		require.YAMLEq(t, "http://example.com/secret\n", string(yamlOut))

		jsonOut, err := json.Marshal(&u)
		require.NoError(t, err)
		require.JSONEq(t, `"http://example.com/secret"`, string(jsonOut))
	})

	t.Run("empty URL marshals to empty", func(t *testing.T) {
		u := SecretTemplateURL("")

		yamlOut, err := yaml.Marshal(&u)
		require.NoError(t, err)
		require.YAMLEq(t, "null\n", string(yamlOut))

		jsonOut, err := json.Marshal(&u)
		require.NoError(t, err)
		require.JSONEq(t, `""`, string(jsonOut))
	})
}

func TestInhibitRuleEqual(t *testing.T) {
	c, err := LoadFile("testdata/conf.inhibit-equal.yml")
	require.NoError(t, err)

	// The inhibition rule should have the expected equal labels.
	require.Len(t, c.InhibitRules, 1)
	r := c.InhibitRules[0]
	require.Equal(t, []string{"qux", "corge"}, r.Equal)

	// Should not be able to load configuration with UTF-8 in equals list.
	_, err = LoadFile("testdata/conf.inhibit-equal-utf8.yml")
	require.Error(t, err)
	require.Equal(t, "invalid label name \"qux🙂\" in equal list", err.Error())

	// Change the mode to UTF-8 mode.
	ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureUTF8StrictMode)
	require.NoError(t, err)
	compat.InitFromFlags(promslog.NewNopLogger(), ff)

	// Restore the mode to classic at the end of the test.
	ff, err = featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureClassicMode)
	require.NoError(t, err)
	defer compat.InitFromFlags(promslog.NewNopLogger(), ff)

	c, err = LoadFile("testdata/conf.inhibit-equal.yml")
	require.NoError(t, err)

	// The inhibition rule should have the expected equal labels.
	require.Len(t, c.InhibitRules, 1)
	r = c.InhibitRules[0]
	require.Equal(t, []string{"qux", "corge"}, r.Equal)

	// Should also be able to load configuration with UTF-8 in equals list.
	c, err = LoadFile("testdata/conf.inhibit-equal-utf8.yml")
	require.NoError(t, err)

	// The inhibition rule should have the expected equal labels.
	require.Len(t, c.InhibitRules, 1)
	r = c.InhibitRules[0]
	require.Equal(t, []string{"qux🙂", "corge"}, r.Equal)
}

func TestGroupByEmptyOverride(t *testing.T) {
	in := `
route:
  receiver: 'default'
  group_by: ['alertname', 'cluster']
  routes:
    - group_by: []

receivers:
  - name: 'default'
`
	cfg, err := Load(in)
	require.NoError(t, err)
	require.Len(t, cfg.Route.GroupBy, 2)
	require.NotNil(t, cfg.Route.Routes[0].GroupBy)
	require.Empty(t, cfg.Route.Routes[0].GroupBy)
}

func TestWechatNoAPIURL(t *testing.T) {
	_, err := LoadFile("testdata/conf.wechat-no-api-secret.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.wechat-no-api-url.yml", err)
	}
	if err.Error() != "no global Wechat Api Secret set either inline or in a file" {
		t.Errorf("Expected: %s\nGot: %s", "no global Wechat Api Secret set either inline or in a file", err.Error())
	}
}

func TestWechatBothAPIURLAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.wechat-both-file-and-secret.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.wechat-both-file-and-secret.yml", err)
	}
	if err.Error() != "at most one of wechat_api_secret & wechat_api_secret_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of wechat_api_secret & wechat_api_secret_file must be configured", err.Error())
	}
}

func TestWechatGlobalAPISecretFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.wechat-default-api-secret-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.wechat-default-api-secret-file.yml", err)
	}

	// no override
	firstConfig := conf.Receivers[0].WechatConfigs[0]
	if firstConfig.APISecretFile != "/global_file" || string(firstConfig.APISecret) != "" {
		t.Fatalf("Invalid Wechat API Secret file: %s\nExpected: %s", firstConfig.APISecretFile, "/global_file")
	}

	// override the file
	secondConfig := conf.Receivers[0].WechatConfigs[1]
	if secondConfig.APISecretFile != "/override_file" || string(secondConfig.APISecret) != "" {
		t.Fatalf("Invalid Wechat API Secret file: %s\nExpected: %s", secondConfig.APISecretFile, "/override_file")
	}

	// override the global file with an inline URL
	thirdConfig := conf.Receivers[0].WechatConfigs[2]
	if string(thirdConfig.APISecret) != "my_inline_secret" || thirdConfig.APISecretFile != "" {
		t.Fatalf("Invalid Wechat API Secret: %s\nExpected: %s", string(thirdConfig.APISecret), "my_inline_secret")
	}
}

func TestMattermostDefaultWebhookURL(t *testing.T) {
	conf, err := LoadFile("testdata/conf.mattermost-default-webhook-url.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.mattermost-default-webhook-url.yml", err)
	}

	defaultWebhookURL := conf.Global.MattermostWebhookURL
	overrideWebhookURL := "https://fakemattermost.example.com/hooks/xxxxxxxxxxxxxxxxxxxxxxxxxx"
	if defaultWebhookURL != conf.Receivers[0].MattermostConfigs[0].WebhookURL {
		t.Fatalf("Invalid mattermost webhook url: %s\nExpected: %s", conf.Receivers[0].MattermostConfigs[0].WebhookURL, defaultWebhookURL)
	}
	if overrideWebhookURL != conf.Receivers[1].MattermostConfigs[0].WebhookURL.String() {
		t.Errorf("Invalid mattermost webhook url: %s\nExpected: %s", conf.Receivers[1].MattermostConfigs[0].WebhookURL, overrideWebhookURL)
	}
}

func TestMattermostDefaultWebhookURLFile(t *testing.T) {
	conf, err := LoadFile("testdata/conf.mattermost-default-webhook-url-file.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.mattermost-default-webhook-url-file.yml", err)
	}

	defaultWebhookURLFile := conf.Global.MattermostWebhookURLFile
	overrideWebhookURLFile := "/override_file"
	if defaultWebhookURLFile != conf.Receivers[0].MattermostConfigs[0].WebhookURLFile {
		t.Fatalf("Invalid mattermost webhook url file: %s\nExpected: %s", conf.Receivers[0].MattermostConfigs[0].WebhookURLFile, defaultWebhookURLFile)
	}
	if overrideWebhookURLFile != conf.Receivers[1].MattermostConfigs[0].WebhookURLFile {
		t.Errorf("Invalid mattermost webhook url file: %s\nExpected: %s", conf.Receivers[1].MattermostConfigs[0].WebhookURLFile, overrideWebhookURLFile)
	}
}

func TestMattermostBothWebhookURLAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.mattermost-both-webhook-url-and-file.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.mattermost-both-webhook-url-and-file.yml", err)
	}
	if err.Error() != "at most one of mattermost_webhook_url & mattermost_webhook_url_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of mattermost_webhook_url & mattermost_webhook_url_file must be configured", err.Error())
	}
}

func TestMattermostValidReceiverBothWebhookURLAndFile(t *testing.T) {
	_, err := LoadFile("testdata/conf.mattermost-valid-receiver-both-webhook-url-and-file.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.mattermost-valid-receiver-both-webhook-url-and-file.yml", err)
	}
	if err.Error() != "at most one of mattermost_webhook_url & mattermost_webhook_url_file must be configured" {
		t.Errorf("Expected: %s\nGot: %s", "at most one of mattermost_webhook_url & mattermost_webhook_url_file must be configured", err.Error())
	}
}

func TestMattermostNoWebhookURL(t *testing.T) {
	_, err := LoadFile("testdata/conf.mattermost-no-webhook-url.yml")
	if err == nil {
		t.Fatalf("Expected an error parsing %s: %s", "testdata/conf.mattermost-no-webhook-url.yml", err)
	}
	if err.Error() != "missing webhook_url or webhook_url_file on mattermost_config" {
		t.Errorf("Expected: %s\nGot: %s", "missing webhook_url or webhook_url_file on mattermost_config", err.Error())
	}
}
