// Copyright 2018 Prometheus Team
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

package template

import (
	tmplhtml "html/template"
	"net/url"
	"sync"
	"testing"
	tmpltext "text/template"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/types"
)

func TestPairNames(t *testing.T) {
	pairs := Pairs{
		{"name1", "value1"},
		{"name2", "value2"},
		{"name3", "value3"},
	}

	expected := []string{"name1", "name2", "name3"}
	require.Equal(t, expected, pairs.Names())
}

func TestPairValues(t *testing.T) {
	pairs := Pairs{
		{"name1", "value1"},
		{"name2", "value2"},
		{"name3", "value3"},
	}

	expected := []string{"value1", "value2", "value3"}
	require.Equal(t, expected, pairs.Values())
}

func TestPairsString(t *testing.T) {
	pairs := Pairs{{"name1", "value1"}}
	require.Equal(t, "name1=value1", pairs.String())
	pairs = append(pairs, Pair{"name2", "value2"})
	require.Equal(t, "name1=value1, name2=value2", pairs.String())
}

func TestKVSortedPairs(t *testing.T) {
	kv := KV{"d": "dVal", "b": "bVal", "c": "cVal"}

	expectedPairs := Pairs{
		{"b", "bVal"},
		{"c", "cVal"},
		{"d", "dVal"},
	}

	for i, p := range kv.SortedPairs() {
		require.Equal(t, p.Name, expectedPairs[i].Name)
		require.Equal(t, p.Value, expectedPairs[i].Value)
	}

	// validates alertname always comes first
	kv = KV{"d": "dVal", "b": "bVal", "c": "cVal", "alertname": "alert", "a": "aVal"}

	expectedPairs = Pairs{
		{"alertname", "alert"},
		{"a", "aVal"},
		{"b", "bVal"},
		{"c", "cVal"},
		{"d", "dVal"},
	}

	for i, p := range kv.SortedPairs() {
		require.Equal(t, p.Name, expectedPairs[i].Name)
		require.Equal(t, p.Value, expectedPairs[i].Value)
	}
}

func TestKVRemove(t *testing.T) {
	kv := KV{
		"key1": "val1",
		"key2": "val2",
		"key3": "val3",
		"key4": "val4",
	}

	kv = kv.Remove([]string{"key2", "key4"})

	expected := []string{"key1", "key3"}
	require.Equal(t, expected, kv.Names())
}

func TestAlertsFiring(t *testing.T) {
	alerts := Alerts{
		{Status: string(model.AlertFiring)},
		{Status: string(model.AlertResolved)},
		{Status: string(model.AlertFiring)},
		{Status: string(model.AlertResolved)},
		{Status: string(model.AlertResolved)},
	}

	for _, alert := range alerts.Firing() {
		if alert.Status != string(model.AlertFiring) {
			t.Errorf("unexpected status %q", alert.Status)
		}
	}
}

func TestAlertsResolved(t *testing.T) {
	alerts := Alerts{
		{Status: string(model.AlertFiring)},
		{Status: string(model.AlertResolved)},
		{Status: string(model.AlertFiring)},
		{Status: string(model.AlertResolved)},
		{Status: string(model.AlertResolved)},
	}

	for _, alert := range alerts.Resolved() {
		if alert.Status != string(model.AlertResolved) {
			t.Errorf("unexpected status %q", alert.Status)
		}
	}
}

func TestData(t *testing.T) {
	u, err := url.Parse("http://example.com/")
	require.NoError(t, err)
	tmpl := &Template{ExternalURL: u}
	startTime := time.Time{}.Add(1 * time.Second)
	endTime := time.Time{}.Add(2 * time.Second)

	for _, tc := range []struct {
		receiver    string
		groupLabels model.LabelSet
		alerts      []*types.Alert

		exp *Data
	}{
		{
			receiver: "webhook",
			exp: &Data{
				Receiver:           "webhook",
				Status:             "resolved",
				Alerts:             Alerts{},
				NotificationReason: "first notification",
				GroupLabels:        KV{},
				CommonLabels:       KV{},
				CommonAnnotations:  KV{},
				ExternalURL:        u.String(),
			},
		},
		{
			receiver: "webhook",
			groupLabels: model.LabelSet{
				model.LabelName("job"): model.LabelValue("foo"),
			},
			alerts: []*types.Alert{
				{
					Alert: model.Alert{
						StartsAt: startTime,
						Labels: model.LabelSet{
							model.LabelName("severity"): model.LabelValue("warning"),
							model.LabelName("job"):      model.LabelValue("foo"),
						},
						Annotations: model.LabelSet{
							model.LabelName("description"): model.LabelValue("something happened"),
							model.LabelName("runbook"):     model.LabelValue("foo"),
						},
					},
				},
				{
					Alert: model.Alert{
						StartsAt: startTime,
						EndsAt:   endTime,
						Labels: model.LabelSet{
							model.LabelName("severity"): model.LabelValue("critical"),
							model.LabelName("job"):      model.LabelValue("foo"),
						},
						Annotations: model.LabelSet{
							model.LabelName("description"): model.LabelValue("something else happened"),
							model.LabelName("runbook"):     model.LabelValue("foo"),
						},
					},
				},
			},
			exp: &Data{
				Receiver: "webhook",
				Status:   "firing",
				Alerts: Alerts{
					{
						Status:      "firing",
						Labels:      KV{"severity": "warning", "job": "foo"},
						Annotations: KV{"description": "something happened", "runbook": "foo"},
						StartsAt:    startTime,
						Fingerprint: "9266ef3da838ad95",
					},
					{
						Status:      "resolved",
						Labels:      KV{"severity": "critical", "job": "foo"},
						Annotations: KV{"description": "something else happened", "runbook": "foo"},
						StartsAt:    startTime,
						EndsAt:      endTime,
						Fingerprint: "3b15fd163d36582e",
					},
				},
				NotificationReason: "first notification",
				GroupLabels:        KV{"job": "foo"},
				CommonLabels:       KV{"job": "foo"},
				CommonAnnotations:  KV{"runbook": "foo"},
				ExternalURL:        u.String(),
			},
		},
		{
			receiver:    "webhook",
			groupLabels: model.LabelSet{},
			alerts: []*types.Alert{
				{
					Alert: model.Alert{
						StartsAt: startTime,
						Labels: model.LabelSet{
							model.LabelName("severity"): model.LabelValue("warning"),
							model.LabelName("job"):      model.LabelValue("foo"),
						},
						Annotations: model.LabelSet{
							model.LabelName("description"): model.LabelValue("something happened"),
							model.LabelName("runbook"):     model.LabelValue("foo"),
						},
					},
				},
				{
					Alert: model.Alert{
						StartsAt: startTime,
						EndsAt:   endTime,
						Labels: model.LabelSet{
							model.LabelName("severity"): model.LabelValue("critical"),
							model.LabelName("job"):      model.LabelValue("bar"),
						},
						Annotations: model.LabelSet{
							model.LabelName("description"): model.LabelValue("something else happened"),
							model.LabelName("runbook"):     model.LabelValue("bar"),
						},
					},
				},
			},
			exp: &Data{
				Receiver: "webhook",
				Status:   "firing",
				Alerts: Alerts{
					{
						Status:      "firing",
						Labels:      KV{"severity": "warning", "job": "foo"},
						Annotations: KV{"description": "something happened", "runbook": "foo"},
						StartsAt:    startTime,
						Fingerprint: "9266ef3da838ad95",
					},
					{
						Status:      "resolved",
						Labels:      KV{"severity": "critical", "job": "bar"},
						Annotations: KV{"description": "something else happened", "runbook": "bar"},
						StartsAt:    startTime,
						EndsAt:      endTime,
						Fingerprint: "c7e68cb08e3e67f9",
					},
				},
				NotificationReason: "first notification",
				GroupLabels:        KV{},
				CommonLabels:       KV{},
				CommonAnnotations:  KV{},
				ExternalURL:        u.String(),
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			got := tmpl.Data(tc.receiver, tc.groupLabels, "first notification", tc.alerts...)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestTemplateExpansion(t *testing.T) {
	tmpl, err := FromGlobs([]string{})
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  any
		html  bool

		exp  string
		fail bool
	}{
		{
			title: "Template without action",
			in:    `abc`,
			exp:   "abc",
		},
		{
			title: "Template with simple action",
			in:    `{{ "abc" }}`,
			exp:   "abc",
		},
		{
			title: "Template with invalid syntax",
			in:    `{{ `,
			fail:  true,
		},
		{
			title: "Template using toUpper",
			in:    `{{ "abc" | toUpper }}`,
			exp:   "ABC",
		},
		{
			title: "Template using toLower",
			in:    `{{ "ABC" | toLower }}`,
			exp:   "abc",
		},
		{
			title: "Template using title",
			in:    `{{ "abc" | title }}`,
			exp:   "Abc",
		},
		{
			title: "Template using TrimSpace",
			in:    `{{ " a b c " | trimSpace }}`,
			exp:   "a b c",
		},
		{
			title: "Template using positive match",
			in:    `{{ if match "^a" "abc"}}abc{{ end }}`,
			exp:   "abc",
		},
		{
			title: "Template using negative match",
			in:    `{{ if match "abcd" "abc" }}abc{{ end }}`,
			exp:   "",
		},
		{
			title: "Template using join",
			in:    `{{ . | join "," }}`,
			data:  []string{"a", "b", "c"},
			exp:   "a,b,c",
		},
		{
			title: "Text template without HTML escaping",
			in:    `{{ "<b>" }}`,
			exp:   "<b>",
		},
		{
			title: "HTML template with escaping",
			in:    `{{ "<b>" }}`,
			html:  true,
			exp:   "&lt;b&gt;",
		},
		{
			title: "HTML template using safeHTML",
			in:    `{{ "<b>" | safeHtml }}`,
			html:  true,
			exp:   "<b>",
		},
		{
			title: "URL template with escaping",
			in:    `<a href="/search?{{ "q=test%20foo" }}"></a>`,
			html:  true,
			exp:   `<a href="/search?q%3dtest%2520foo"></a>`,
		},
		{
			title: "URL template using safeUrl",
			in:    `<a href="/search?{{ "q=test%20foo" | safeUrl }}"></a>`,
			html:  true,
			exp:   `<a href="/search?q=test%20foo"></a>`,
		},
		{
			title: "Template using reReplaceAll",
			in:    `{{ reReplaceAll "ab" "AB" "abcdabcda"}}`,
			exp:   "ABcdABcda",
		},
		{
			title: "Template using urlUnescape",
			in:    `{{ "search?q=test%20foo" | urlUnescape }}`,
			exp:   "search?q=test foo",
		},
		{
			title: "Template using stringSlice",
			in:    `{{ with .GroupLabels }}{{ with .Remove (stringSlice "key1" "key3") }}{{ .SortedPairs.Values }}{{ end }}{{ end }}`,
			data: Data{
				GroupLabels: KV{
					"key1": "key1",
					"key2": "key2",
					"key3": "key3",
					"key4": "key4",
				},
			},
			exp: "[key2 key4]",
		},
		{
			title: "Template using toJson with string",
			in:    `{{ "test" | toJson }}`,
			exp:   `"test"`,
		},
		{
			title: "Template using toJson with number",
			in:    `{{ 42 | toJson }}`,
			exp:   `42`,
		},
		{
			title: "Template using toJson with boolean",
			in:    `{{ true | toJson }}`,
			exp:   `true`,
		},
		{
			title: "Template using toJson with map",
			in:    `{{ . | toJson }}`,
			data:  map[string]any{"key": "value", "number": 123},
			exp:   `{"key":"value","number":123}`,
		},
		{
			title: "Template using toJson with slice",
			in:    `{{ . | toJson }}`,
			data:  []string{"a", "b", "c"},
			exp:   `["a","b","c"]`,
		},
		{
			title: "Template using toJson with KV",
			in:    `{{ .CommonLabels | toJson }}`,
			data: Data{
				CommonLabels: KV{"severity": "critical", "job": "foo"},
			},
			exp: `{"job":"foo","severity":"critical"}`,
		},
		{
			title: "Template using toJson with Alerts",
			in:    `{{ .Alerts | toJson }}`,
			data: Data{
				Alerts: Alerts{
					{
						Status: "firing",
						Labels: KV{"alertname": "test"},
					},
				},
			},
			exp: `[{"status":"firing","labels":{"alertname":"test"},"annotations":null,"startsAt":"0001-01-01T00:00:00Z","endsAt":"0001-01-01T00:00:00Z","generatorURL":"","fingerprint":""}]`,
		},
		{
			title: "Template using toJson with Alerts.Firing()",
			in:    `{{ .Alerts.Firing | toJson }}`,
			data: Data{
				Alerts: Alerts{
					{Status: "firing"},
					{Status: "resolved"},
				},
			},
			exp: `[{"status":"firing","labels":null,"annotations":null,"startsAt":"0001-01-01T00:00:00Z","endsAt":"0001-01-01T00:00:00Z","generatorURL":"","fingerprint":""}]`,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			if tc.html {
				f = tmpl.ExecuteHTMLString
			}
			got, err := f(tc.in, tc.data)
			if tc.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestTemplateExpansionWithOptions(t *testing.T) {
	testOptionWithAdditionalFuncs := func(funcs FuncMap) Option {
		return func(text *tmpltext.Template, html *tmplhtml.Template) {
			text.Funcs(tmpltext.FuncMap(funcs))
			html.Funcs(tmplhtml.FuncMap(funcs))
		}
	}
	for _, tc := range []struct {
		options []Option
		title   string
		in      string
		data    any
		html    bool

		exp  string
		fail bool
	}{
		{
			title:   "Test custom function",
			options: []Option{testOptionWithAdditionalFuncs(FuncMap{"printFoo": func() string { return "foo" }})},
			in:      `{{ printFoo }}`,
			exp:     "foo",
		},
		{
			title:   "Test Default function with additional function added",
			options: []Option{testOptionWithAdditionalFuncs(FuncMap{"printFoo": func() string { return "foo" }})},
			in:      `{{ toUpper "test" }}`,
			exp:     "TEST",
		},
		{
			title:   "Test custom function is overridden by the DefaultFuncs",
			options: []Option{testOptionWithAdditionalFuncs(FuncMap{"toUpper": func(s string) string { return "foo" }})},
			in:      `{{ toUpper "test" }}`,
			exp:     "TEST",
		},
		{
			title: "Test later Option overrides the previous",
			options: []Option{
				testOptionWithAdditionalFuncs(FuncMap{"printFoo": func() string { return "foo" }}),
				testOptionWithAdditionalFuncs(FuncMap{"printFoo": func() string { return "bar" }}),
			},
			in:  `{{ printFoo }}`,
			exp: "bar",
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			tmpl, err := FromGlobs([]string{}, tc.options...)
			require.NoError(t, err)
			f := tmpl.ExecuteTextString
			if tc.html {
				f = tmpl.ExecuteHTMLString
			}
			got, err := f(tc.in, tc.data)
			if tc.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

// This test asserts that template functions are thread-safe.
func TestTemplateFuncs(t *testing.T) {
	tmpl, err := FromGlobs([]string{})
	require.NoError(t, err)

	for _, tc := range []struct {
		title  string
		in     string
		data   any
		exp    string
		expErr string
	}{{
		title: "Template using toUpper",
		in:    `{{ "abc" | toUpper }}`,
		exp:   "ABC",
	}, {
		title: "Template using toLower",
		in:    `{{ "ABC" | toLower }}`,
		exp:   "abc",
	}, {
		title: "Template using title",
		in:    `{{ "abc" | title }}`,
		exp:   "Abc",
	}, {
		title: "Template using trimSpace",
		in:    `{{ " abc " | trimSpace }}`,
		exp:   "abc",
	}, {
		title: "Template using join",
		in:    `{{ . | join "," }}`,
		data:  []string{"abc", "def"},
		exp:   "abc,def",
	}, {
		title: "Template using match",
		in:    `{{ match "[a-z]+" "abc" }}`,
		exp:   "true",
	}, {
		title: "Template using reReplaceAll",
		in:    `{{ reReplaceAll "ab" "AB" "abc" }}`,
		exp:   "ABc",
	}, {
		title: "Template using date",
		in:    `{{ . | date "2006-01-02" }}`,
		data:  time.Date(2024, 1, 1, 8, 15, 30, 0, time.UTC),
		exp:   "2024-01-01",
	}, {
		title: "Template using tz",
		in:    `{{ . | tz "Europe/Paris" }}`,
		data:  time.Date(2024, 1, 1, 8, 15, 30, 0, time.UTC),
		exp:   "2024-01-01 09:15:30 +0100 CET",
	}, {
		title:  "Template using invalid tz",
		in:     `{{ . | tz "Invalid/Timezone" }}`,
		data:   time.Date(2024, 1, 1, 8, 15, 30, 0, time.UTC),
		expErr: "template: :1:7: executing \"\" at <tz \"Invalid/Timezone\">: error calling tz: unknown time zone Invalid/Timezone",
	}, {
		title: "Template using HumanizeDuration - seconds - float64",
		in:    "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
		data:  []float64{0, 1, 60, 3600, 86400, 86400 + 3600, -(86400*2 + 3600*3 + 60*4 + 5), 899.99},
		exp:   "0s:1s:1m 0s:1h 0m 0s:1d 0h 0m 0s:1d 1h 0m 0s:-2d 3h 4m 5s:14m 59s:",
	}, {
		title: "Template using HumanizeDuration - seconds - string.",
		in:    "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
		data:  []string{"0", "1", "60", "3600", "86400"},
		exp:   "0s:1s:1m 0s:1h 0m 0s:1d 0h 0m 0s:",
	}, {
		title: "Template using HumanizeDuration - subsecond and fractional seconds - float64.",
		in:    "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
		data:  []float64{.1, .0001, .12345, 60.1, 60.5, 1.2345, 12.345},
		exp:   "100ms:100us:123.5ms:1m 0s:1m 0s:1.234s:12.35s:",
	}, {
		title: "Template using HumanizeDuration - subsecond and fractional seconds - string.",
		in:    "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
		data:  []string{".1", ".0001", ".12345", "60.1", "60.5", "1.2345", "12.345"},
		exp:   "100ms:100us:123.5ms:1m 0s:1m 0s:1.234s:12.35s:",
	}, {
		title:  "Template using HumanizeDuration - string with error.",
		in:     `{{ humanizeDuration "one" }}`,
		expErr: "template: :1:3: executing \"\" at <humanizeDuration \"one\">: error calling humanizeDuration: strconv.ParseFloat: parsing \"one\": invalid syntax",
	}, {
		title: "Template using HumanizeDuration - int.",
		in:    "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
		data:  []int{0, -1, 1, 1234567},
		exp:   "0s:-1s:1s:14d 6h 56m 7s:",
	}, {
		title: "Template using HumanizeDuration - uint.",
		in:    "{{ range . }}{{ humanizeDuration . }}:{{ end }}",
		data:  []uint{0, 1, 1234567},
		exp:   "0s:1s:14d 6h 56m 7s:",
	}, {
		title: "Template using since",
		in:    "{{ . | since | humanizeDuration }}",
		data:  time.Now().Add(-1 * time.Hour),
		exp:   "1h 0m 0s",
	}, {
		title: "Template using toJson with string",
		in:    `{{ "hello" | toJson }}`,
		exp:   `"hello"`,
	}, {
		title: "Template using toJson with map",
		in:    `{{ . | toJson }}`,
		data:  map[string]string{"key": "value"},
		exp:   `{"key":"value"}`,
	}, {
		title: "Template using toJson with Alerts.Firing()",
		in:    `{{ .Alerts.Firing | toJson }}`,
		data: Data{
			Alerts: Alerts{
				{Status: "firing", Labels: KV{"alertname": "test"}},
				{Status: "resolved"},
			},
		},
		exp: `[{"status":"firing","labels":{"alertname":"test"},"annotations":null,"startsAt":"0001-01-01T00:00:00Z","endsAt":"0001-01-01T00:00:00Z","generatorURL":"","fingerprint":""}]`,
	}} {
		t.Run(tc.title, func(t *testing.T) {
			wg := sync.WaitGroup{}
			for range 10 {
				wg.Go(func() {
					got, err := tmpl.ExecuteTextString(tc.in, tc.data)
					if tc.expErr == "" {
						require.NoError(t, err)
						require.Equal(t, tc.exp, got)
					} else {
						require.EqualError(t, err, tc.expErr)
						require.Empty(t, got)
					}
				})
			}
			wg.Wait()
		})
	}
}

func TestDeepCopyWithTemplate(t *testing.T) {
	identity := TemplateFunc(func(s string) (string, error) { return s, nil })
	withSuffix := TemplateFunc(func(s string) (string, error) { return s + "-templated", nil })

	for _, tc := range []struct {
		title   string
		input   any
		fn      TemplateFunc
		want    any
		wantErr string
	}{
		{
			title: "string keeps templated value",
			input: "hello",
			fn:    withSuffix,
			want:  "hello-templated",
		},
		{
			title: "string parsed as YAML map",
			input: "foo: bar",
			fn:    identity,
			want:  map[string]any{"foo": "bar"},
		},
		{
			title: "slice templating applied recursively",
			input: []any{"foo", 42},
			fn:    withSuffix,
			want:  []any{"foo-templated", 42},
		},
		{
			title: "map converts keys and drops non-string",
			input: map[any]any{
				"foo":    "bar",
				42:       "ignore",
				"nested": []any{"baz"},
			},
			fn: withSuffix,
			want: map[string]any{
				"foo-templated":    "bar-templated",
				"nested-templated": []any{"baz-templated"},
			},
		},
		{
			title: "non string value returned as-is",
			input: 123,
			fn:    identity,
			want:  123,
		},
		{
			title: "nil input",
			input: nil,
			fn:    identity,
			want:  nil,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			got, err := DeepCopyWithTemplate(tc.input, tc.fn)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
