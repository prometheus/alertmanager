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
	"net/url"
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

func TestShowMarshalSecretValues(t *testing.T) {
	MarshalSecretValue = true
	defer func() { MarshalSecretValue = false }()

	c, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	// String method must reveal authentication credentials.
	s := c.String()
	if strings.Count(s, "<secret>") > 0 || !strings.Contains(s, "mysecret") {
		t.Fatal("config's String method must reveal authentication credentials when MarshalSecretValue = true.")
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

func TestJSONMarshalHideSecret(t *testing.T) {
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

func TestJSONMarshalShowSecret(t *testing.T) {
	MarshalSecretValue = true
	defer func() { MarshalSecretValue = false }()

	test := struct {
		S Secret
	}{
		S: Secret("test"),
	}

	c, err := json.Marshal(test)
	if err != nil {
		t.Fatal(err)
	}
	require.JSONEq(t, `{"S":"test"}`, string(c), "config's String method must reveal authentication credentials when MarshalSecretValue = true.")
}

func TestJSONMarshalHideSecretURL(t *testing.T) {
	urlp, err := url.Parse("http://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	u := &SecretURL{urlp}

	c, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	// u003c -> "<"
	// u003e -> ">"
	require.Equal(t, "\"\\u003csecret\\u003e\"", string(c), "SecretURL not properly elided in JSON.")
	// Check that the marshaled data can be unmarshaled again.
	out := &SecretURL{}
	err = json.Unmarshal(c, out)
	if err != nil {
		t.Fatal(err)
	}

	c, err = yaml.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "<secret>\n", string(c), "SecretURL not properly elided in YAML.")
	// Check that the marshaled data can be unmarshaled again.
	out = &SecretURL{}
	err = yaml.Unmarshal(c, &out)
	if err != nil {
		t.Fatal(err)
	}
}

func TestJSONMarshalShowSecretURL(t *testing.T) {
	MarshalSecretValue = true
	defer func() { MarshalSecretValue = false }()

	urlp, err := url.Parse("http://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	u := &SecretURL{urlp}

	c, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "\"http://example.com/\"", string(c), "config's String method must reveal authentication credentials when MarshalSecretValue = true.")
}

func TestUnmarshalSecretURL(t *testing.T) {
	b := []byte(`"http://example.com/se cret"`)
	var u SecretURL

	err := json.Unmarshal(b, &u)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "http://example.com/se%20cret", u.String(), "SecretURL not properly unmarshaled in JSON.")

	err = yaml.Unmarshal(b, &u)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, "http://example.com/se%20cret", u.String(), "SecretURL not properly unmarshaled in YAML.")
}

func TestHideSecretURL(t *testing.T) {
	b := []byte(`"://wrongurl/"`)
	var u SecretURL

	err := json.Unmarshal(b, &u)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "wrongurl")
}

func TestShowMarshalSecretURL(t *testing.T) {
	MarshalSecretValue = true
	defer func() { MarshalSecretValue = false }()

	b := []byte(`"://wrongurl/"`)
	var u SecretURL

	err := json.Unmarshal(b, &u)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrongurl")
}

func TestMarshalURL(t *testing.T) {
	for name, tc := range map[string]struct {
		input        *URL
		expectedJSON string
		expectedYAML string
	}{
		"url": {
			input:        mustParseURL("http://example.com/"),
			expectedJSON: "\"http://example.com/\"",
			expectedYAML: "http://example.com/\n",
		},

		"wrapped nil value": {
			input:        &URL{},
			expectedJSON: "null",
			expectedYAML: "null\n",
		},

		"wrapped empty URL": {
			input:        &URL{&url.URL{}},
			expectedJSON: "\"\"",
			expectedYAML: "\"\"\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			j, err := json.Marshal(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expectedJSON, string(j), "URL not properly marshaled into JSON.")

			y, err := yaml.Marshal(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expectedYAML, string(y), "URL not properly marshaled into YAML.")
		})
	}
}

func TestUnmarshalNilURL(t *testing.T) {
	b := []byte(`null`)

	{
		var u URL
		err := json.Unmarshal(b, &u)
		require.Error(t, err, "unsupported scheme \"\" for URL")
	}

	{
		var u URL
		err := yaml.Unmarshal(b, &u)
		require.NoError(t, err)
	}
}

func TestUnmarshalEmptyURL(t *testing.T) {
	b := []byte(`""`)

	{
		var u URL
		err := json.Unmarshal(b, &u)
		require.Error(t, err, "unsupported scheme \"\" for URL")
		require.Equal(t, (*url.URL)(nil), u.URL)
	}

	{
		var u URL
		err := yaml.Unmarshal(b, &u)
		require.Error(t, err, "unsupported scheme \"\" for URL")
		require.Equal(t, (*url.URL)(nil), u.URL)
	}
}

func TestUnmarshalURL(t *testing.T) {
	b := []byte(`"http://example.com/a b"`)
	var u URL

	err := json.Unmarshal(b, &u)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "http://example.com/a%20b", u.String(), "URL not properly unmarshaled in JSON.")

	err = yaml.Unmarshal(b, &u)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "http://example.com/a%20b", u.String(), "URL not properly unmarshaled in YAML.")
}

func TestUnmarshalInvalidURL(t *testing.T) {
	for _, b := range [][]byte{
		[]byte(`"://example.com"`),
		[]byte(`"http:example.com"`),
		[]byte(`"telnet://example.com"`),
	} {
		var u URL

		err := json.Unmarshal(b, &u)
		if err == nil {
			t.Errorf("Expected an error unmarshaling %q from JSON", string(b))
		}

		err = yaml.Unmarshal(b, &u)
		if err == nil {
			t.Errorf("Expected an error unmarshaling %q from YAML", string(b))
		}
		t.Logf("%s", err)
	}
}

func TestUnmarshalRelativeURL(t *testing.T) {
	b := []byte(`"/home"`)
	var u URL

	err := json.Unmarshal(b, &u)
	if err == nil {
		t.Errorf("Expected an error parsing URL")
	}

	err = yaml.Unmarshal(b, &u)
	if err == nil {
		t.Errorf("Expected an error parsing URL")
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
			SlackAPIURL:      (*SecretURL)(mustParseURL("http://slack.example.com/")),
			SMTPRequireTLS:   true,
			PagerdutyURL:     mustParseURL("https://events.pagerduty.com/v2/enqueue"),
			OpsGenieAPIURL:   mustParseURL("https://api.opsgenie.com/"),
			WeChatAPIURL:     mustParseURL("https://qyapi.weixin.qq.com/cgi-bin/"),
			VictorOpsAPIURL:  mustParseURL("https://alert.victorops.com/integrations/generic/20131114/alert/"),
			TelegramAPIUrl:   mustParseURL("https://api.telegram.org"),
			WebexAPIURL:      mustParseURL("https://webexapis.com/v1/messages"),
			RocketchatAPIURL: mustParseURL("https://open.rocket.chat/"),
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
	if defaultKey != conf.Receivers[0].OpsGenieConfigs[0].APIKey {
		t.Fatalf("Invalid OpsGenie key: %s\nExpected: %s", conf.Receivers[0].OpsGenieConfigs[0].APIKey, defaultKey)
	}
	if defaultKey == conf.Receivers[1].OpsGenieConfigs[0].APIKey {
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

func TestRocketchatDefaultToken(t *testing.T) {
	conf, err := LoadFile("testdata/conf.rocketchat-default-token.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.rocketchat-default-token.yml", err)
	}

	defaultToken := conf.Global.RocketchatToken
	overrideToken := Secret("token456")
	if defaultToken != conf.Receivers[0].RocketchatConfigs[0].Token {
		t.Fatalf("Invalid rocketchat key: %s\nExpected: %s", string(*conf.Receivers[0].RocketchatConfigs[0].Token), string(*defaultToken))
	}
	if overrideToken != *conf.Receivers[1].RocketchatConfigs[0].Token {
		t.Errorf("Invalid rocketchat key: %s\nExpected: %s", string(*conf.Receivers[0].RocketchatConfigs[0].Token), string(overrideToken))
	}
}

func TestRocketchatDefaultTokenID(t *testing.T) {
	conf, err := LoadFile("testdata/conf.rocketchat-default-token.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.rocketchat-default-token.yml", err)
	}

	defaultTokenID := conf.Global.RocketchatTokenID
	overrideTokenID := Secret("id456")
	if defaultTokenID != conf.Receivers[0].RocketchatConfigs[0].TokenID {
		t.Fatalf("Invalid rocketchat key: %s\nExpected: %s", string(*conf.Receivers[0].RocketchatConfigs[0].TokenID), string(*defaultTokenID))
	}
	if overrideTokenID != *conf.Receivers[1].RocketchatConfigs[0].TokenID {
		t.Errorf("Invalid rocketchat key: %s\nExpected: %s", string(*conf.Receivers[0].RocketchatConfigs[0].TokenID), string(overrideTokenID))
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
		t.Errorf("Invalid Rocketchat key_file: %s\nExpected: %s", conf.Receivers[0].RocketchatConfigs[0].TokenFile, overrideTokenFile)
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
		t.Errorf("Invalid Rocketchat key_file: %s\nExpected: %s", conf.Receivers[0].RocketchatConfigs[0].TokenIDFile, overrideTokenIDFile)
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
