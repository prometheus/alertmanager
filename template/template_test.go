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
	_ "embed"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/types"
)

//go:embed global-dojo.tmpl
var globalDojoTemplate string

func TestPairNames(t *testing.T) {
	pairs := Pairs{
		{"name1", "value1"},
		{"name2", "value2"},
		{"name3", "value3"},
	}

	expected := []string{"name1", "name2", "name3"}
	require.EqualValues(t, expected, pairs.Names())
}

func TestPairValues(t *testing.T) {
	pairs := Pairs{
		{"name1", "value1"},
		{"name2", "value2"},
		{"name3", "value3"},
	}

	expected := []string{"value1", "value2", "value3"}
	require.EqualValues(t, expected, pairs.Values())
}

func TestKVSortedPairs(t *testing.T) {
	kv := KV{"d": "dVal", "b": "bVal", "c": "cVal"}

	expectedPairs := Pairs{
		{"b", "bVal"},
		{"c", "cVal"},
		{"d", "dVal"},
	}

	for i, p := range kv.SortedPairs() {
		require.EqualValues(t, p.Name, expectedPairs[i].Name)
		require.EqualValues(t, p.Value, expectedPairs[i].Value)
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
		require.EqualValues(t, p.Name, expectedPairs[i].Name)
		require.EqualValues(t, p.Value, expectedPairs[i].Value)
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
	require.EqualValues(t, expected, kv.Names())
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
				Receiver:          "webhook",
				Status:            "resolved",
				Alerts:            Alerts{},
				GroupLabels:       KV{},
				CommonLabels:      KV{},
				CommonAnnotations: KV{},
				ExternalURL:       u.String(),
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
				GroupLabels:       KV{"job": "foo"},
				CommonLabels:      KV{"job": "foo"},
				CommonAnnotations: KV{"runbook": "foo"},
				ExternalURL:       u.String(),
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
				GroupLabels:       KV{},
				CommonLabels:      KV{},
				CommonAnnotations: KV{},
				ExternalURL:       u.String(),
			},
		},
	} {
		tc := tc
		t.Run("", func(t *testing.T) {
			got := tmpl.Data(tc.receiver, tc.groupLabels, tc.alerts...)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestTemplateExpansion(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}
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
			title: "Template using reReplaceAll",
			in:    `{{ reReplaceAll "ab" "AB" "abcdabcda"}}`,
			exp:   "ABcdABcda",
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
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			if tc.html {
				f = tmpl.ExecuteHTMLString
			}
			got, err := f(tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoSubjectStableText(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.subject.stable_text with no group_by and firing alerts",
			in:    `{{ template "dojo.subject.stable_text" . }}`,
			data: Data{
				Receiver: "Receiver",
				Status:   "firing",
			},
			exp: "Alerts for Receiver",
		},
		{
			title: "dojo.subject.stable_text with no group_by and resolved alerts",
			in:    `{{ template "dojo.subject.stable_text" . }}`,
			data: Data{
				Receiver: "Receiver",
				Status:   "resolved",
			},
			exp: "Resolved: Alerts for Receiver",
		},
		{
			title: "dojo.subject.stable_text with group_by, alertname and firing alerts",
			in:    `{{ template "dojo.subject.stable_text" . }}`,
			data: Data{
				Receiver: "Receiver",
				Status:   "firing",
				GroupLabels: KV{
					"alertname": "AlertName",
					"label1":    "value1",
					"label2":    "value2",
				},
			},
			exp: "[AlertName] (label1=value1 label2=value2)",
		},
		{
			title: "dojo.subject.stable_text with group_by, alertname and resolved alerts",
			in:    `{{ template "dojo.subject.stable_text" . }}`,
			data: Data{
				Receiver: "Receiver",
				Status:   "resolved",
				GroupLabels: KV{
					"alertname": "AlertName",
					"label1":    "value1",
					"label2":    "value2",
				},
			},
			exp: "Resolved: [AlertName] (label1=value1 label2=value2)",
		},
		{
			title: "dojo.subject.stable_text with group_by and firing alerts",
			in:    `{{ template "dojo.subject.stable_text" . }}`,
			data: Data{
				Receiver: "Receiver",
				Status:   "firing",
				GroupLabels: KV{
					"label1": "value1",
					"label2": "value2",
				},
			},
			exp: "(label1=value1 label2=value2)",
		},
		{
			title: "dojo.subject.stable_text with group_by and resolved alerts",
			in:    `{{ template "dojo.subject.stable_text" . }}`,
			data: Data{
				Receiver: "Receiver",
				Status:   "resolved",
				GroupLabels: KV{
					"label1": "value1",
					"label2": "value2",
				},
			},
			exp: "Resolved: (label1=value1 label2=value2)",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoAlertText(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.alert.text with alertname and labels",
			in:    `{{ template "dojo.alert.text" (index .Alerts 0) }}`,
			data: Data{
				Alerts: Alerts{
					{
						Status: "firing",
						Labels: KV{
							"alertname": "AlertName",
							"label1":    "value1",
							"label2":    "value2",
						},
						Annotations: KV{
							"annotation1": "value1",
							"annotation2": "value2",
						},
						GeneratorURL: "http://generator.url/",
					},
				},
			},
			exp: "[AlertName] (label1=value1 label2=value2)\n" +
				"Annotations:\n" +
				"annotation1: value1\n" +
				"annotation2: value2\n" +
				"http://generator.url/",
		},
		{
			title: "dojo.alert.text with alertname and no extra labels",
			in:    `{{ template "dojo.alert.text" (index .Alerts 0) }}`,
			data: Data{
				Alerts: Alerts{
					{
						Status: "firing",
						Labels: KV{
							"alertname": "AlertName",
						},
						Annotations: KV{
							"annotation1": "value1",
							"annotation2": "value2",
						},
						GeneratorURL: "http://generator.url/",
					},
				},
			},
			exp: "[AlertName]\n" +
				"Annotations:\n" +
				"annotation1: value1\n" +
				"annotation2: value2\n" +
				"http://generator.url/",
		},
		{
			title: "dojo.alert.text without alertname and labels",
			in:    `{{ template "dojo.alert.text" (index .Alerts 0) }}`,
			data: Data{
				Alerts: Alerts{
					{
						Status: "firing",
						Labels: KV{
							"label1": "value1",
							"label2": "value2",
						},
						Annotations: KV{
							"annotation1": "value1",
							"annotation2": "value2",
						},
						GeneratorURL: "http://generator.url/",
					},
				},
			},
			exp: "(label1=value1 label2=value2)\n" +
				"Annotations:\n" +
				"annotation1: value1\n" +
				"annotation2: value2\n" +
				"http://generator.url/",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoAlertsStatusGroupedText(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.alerts.status_grouped_text with firing & resolved",
			in:    `{{ template "dojo.alerts.status_grouped_text" .Alerts }}`,
			data: Data{
				Alerts: Alerts{
					{
						Status: "firing",
						Labels: KV{
							"alertname": "AlertName1",
							"label1":    "value1",
							"label2":    "value2",
						},
						Annotations: KV{
							"annotation1": "value1",
							"annotation2": "value2",
						},
						GeneratorURL: "http://generator.url/",
					},
					{
						Status: "resolved",
						Labels: KV{
							"label1": "value1",
							"label2": "value2",
						},
						Annotations: KV{
							"annotation1": "value1",
							"annotation2": "value2",
						},
						GeneratorURL: "http://generator.url/",
					},
				},
			},
			exp: "FIRING:\n" +
				"\n" +
				"[AlertName1] (label1=value1 label2=value2)\n" +
				"Annotations:\n" +
				"annotation1: value1\n" +
				"annotation2: value2\n" +
				"http://generator.url/\n" +
				"\n" +
				"RESOLVED:\n" +
				"\n" +
				"(label1=value1 label2=value2)\n" +
				"Annotations:\n" +
				"annotation1: value1\n" +
				"annotation2: value2\n" +
				"http://generator.url/",
		},
		{
			title: "dojo.alerts.status_grouped_text with only firing",
			in:    `{{ template "dojo.alerts.status_grouped_text" .Alerts }}`,
			data: Data{
				Alerts: Alerts{
					{
						Status: "firing",
						Labels: KV{
							"alertname": "AlertName1",
							"label1":    "value1",
							"label2":    "value2",
						},
						Annotations: KV{
							"annotation1": "value1",
							"annotation2": "value2",
						},
						GeneratorURL: "http://generator.url/",
					},
					{
						Status: "firing",
						Labels: KV{
							"label1": "value1",
							"label2": "value2",
						},
						Annotations: KV{
							"annotation1": "value1",
							"annotation2": "value2",
						},
						GeneratorURL: "http://generator.url/",
					},
				},
			},
			exp: "[AlertName1] (label1=value1 label2=value2)\n" +
				"Annotations:\n" +
				"annotation1: value1\n" +
				"annotation2: value2\n" +
				"http://generator.url/\n" +
				"\n" +
				"(label1=value1 label2=value2)\n" +
				"Annotations:\n" +
				"annotation1: value1\n" +
				"annotation2: value2\n" +
				"http://generator.url/",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoAlertsUrlFiring(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.alerts.url.firing with group_by and with alertname and urgency",
			in:    `{{ template "dojo.alerts.url.firing" . }}`,
			data: Data{
				GroupLabels: KV{
					"alertname": "AlertName",
					"foo":       "$b a r",
				},
				CommonLabels: KV{
					"tenant":    "example",
					"urgency":   "high",
					"alertname": "AlertName",
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/alerting/list?" +
				"dataSource=DATASOURCE_NAME&" +
				"queryString=" +
				"tenant%3Dexample," +
				"urgency%3Dhigh," +
				"alertname%3DAlertName," +
				"foo%3D%24b+a+r,&" +
				"ruleType=alerting&" +
				"alertState=firing",
		},
		{
			title: "dojo.alerts.url.firing with group_by and without alertname and urgency",
			in:    `{{ template "dojo.alerts.url.firing" . }}`,
			data: Data{
				GroupLabels: KV{
					"foo": "$b a r",
				},
				CommonLabels: KV{
					"tenant":    "example",
					"alertname": "AlertName", // NOT to be used!
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/alerting/list?" +
				"dataSource=DATASOURCE_NAME&" +
				"queryString=" +
				"tenant%3Dexample," +
				"urgency!~%5E(high%7Clow)$," +
				"foo%3D%24b+a+r,&" +
				"ruleType=alerting&" +
				"alertState=firing",
		},
		{
			title: "dojo.alerts.url.firing without group_by",
			in:    `{{ template "dojo.alerts.url.firing" . }}`,
			data: Data{
				CommonLabels: KV{
					"tenant":    "example",
					"urgency":   "high",
					"alertname": "AlertName",
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/alerting/list?" +
				"dataSource=DATASOURCE_NAME&" +
				"queryString=" +
				"tenant%3Dexample," +
				"urgency%3Dhigh,&" +
				"ruleType=alerting&" +
				"alertState=firing",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoAlertsUrlHistory(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.alerts.url.history with group_by and with alertname and urgency",
			in:    `{{ template "dojo.alerts.url.history" . }}`,
			data: Data{
				GroupLabels: KV{
					"alertname": "AlertName",
					"foo":       "$b a r",
				},
				CommonLabels: KV{
					"tenant":    "example",
					"urgency":   "high",
					"alertname": "AlertName",
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/d/luyBQ9Y7z/?orgId=1&" +
				"var-data_source=DATASOURCE_NAME&" +
				"var-tenant=example&" +
				"var-urgency=high&" +
				"var-alertname=AlertName&" +
				"var-label=foo%7C%3D%7C%24b+a+r&",
		},
		{
			title: "dojo.alerts.url.history with group_by and without alertname and urgency",
			in:    `{{ template "dojo.alerts.url.history" . }}`,
			data: Data{
				GroupLabels: KV{
					"foo": "$b a r",
				},
				CommonLabels: KV{
					"tenant":    "example",
					"alertname": "AlertName", // NOT to be used!
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/d/luyBQ9Y7z/?orgId=1&" +
				"var-data_source=DATASOURCE_NAME&" +
				"var-tenant=example&" +
				"var-label=urgency%7C!~%7C%5E(high__gfp__low)$&" +
				"var-label=foo%7C%3D%7C%24b+a+r&",
		},
		{
			title: "dojo.alerts.url.history without group_by",
			in:    `{{ template "dojo.alerts.url.history" . }}`,
			data: Data{
				CommonLabels: KV{
					"tenant":    "example",
					"urgency":   "high",
					"alertname": "AlertName",
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/d/luyBQ9Y7z/?orgId=1&" +
				"var-data_source=DATASOURCE_NAME&" +
				"var-tenant=example&" +
				"var-urgency=high&",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoAlertsUrlNewSilence(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.alerts.url.new_silence with group_by and with alertname and urgency",
			in:    `{{ template "dojo.alerts.url.new_silence" . }}`,
			data: Data{
				GroupLabels: KV{
					"alertname": "AlertName",
					"foo":       "$b a r",
				},
				CommonLabels: KV{
					"tenant":    "example",
					"urgency":   "high",
					"alertname": "AlertName",
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/alerting/silence/new?" +
				"alertmanager=ALERTMANAGER_NAME&" +
				"matcher=tenant%3Dexample&" +
				"matcher=urgency%3Dhigh&" +
				"matcher=alertname%3DAlertName&" +
				"matcher=foo%3D%24b+a+r&",
		},
		{
			title: "dojo.alerts.url.new_silence with group_by and without alertname and urgency",
			in:    `{{ template "dojo.alerts.url.new_silence" . }}`,
			data: Data{
				GroupLabels: KV{
					"foo": "$b a r",
				},
				CommonLabels: KV{
					"tenant":    "example",
					"alertname": "AlertName", // NOT to be used!
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/alerting/silence/new?" +
				"alertmanager=ALERTMANAGER_NAME&" +
				"matcher=tenant%3Dexample&" +
				"matcher=urgency!~%5E(high|low)$&" +
				"matcher=foo%3D%24b+a+r&",
		},
		{
			title: "dojo.alerts.url.new_silence without group_by",
			in:    `{{ template "dojo.alerts.url.new_silence" . }}`,
			data: Data{
				CommonLabels: KV{
					"tenant":    "example",
					"urgency":   "high",
					"alertname": "AlertName",
					"must":      "not",
					"be":        "used",
				},
			},
			exp: "https://paymentsense.grafana.net/alerting/silence/new?" +
				"alertmanager=ALERTMANAGER_NAME&" +
				"matcher=tenant%3Dexample&" +
				"matcher=urgency%3Dhigh&",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoDocumentationHighUrgency(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.documentation.high_urgency",
			in:    `{{ template "dojo.documentation.high_urgency" . }}`,
			data: Data{
				GroupLabels: KV{
					"label1": "value $1",
					"label2": "value $2",
				},
				CommonLabels: KV{
					"tenant":  "example",
					"urgency": "high",
				},
			},
			exp: "Alert(s) of high urgency have fired meaning there's likely business impact going on. Please work on fixing the problem IMMEDIATELY!\n" +
				"\n" +
				"You must work until all firing alerts are resolved.",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoDocumentationLowUrgency(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.documentation.low_urgency",
			in:    `{{ template "dojo.documentation.low_urgency" . }}`,
			data: Data{
				GroupLabels: KV{
					"label1": "value $1",
					"label2": "value $2",
				},
				CommonLabels: KV{
					"tenant":  "example",
					"urgency": "low",
				},
			},
			exp: "Alert(s) fired indicating there's tolerable business impact or that action needs to be taken to prevent issues. They can be worked on the next business day.\n" +
				"\n" +
				"If this is a misfire, then fix the alert so that it only triggers when action is required.",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}

func TestDojoDocumentationUnknownUrgency(t *testing.T) {
	tmpl, err := FromGlobs()
	require.NoError(t, err)

	for _, tc := range []struct {
		title string
		in    string
		data  interface{}

		exp  string
		fail bool
	}{
		{
			title: "dojo.documentation.unknown_urgency",
			in:    `{{ template "dojo.documentation.unknown_urgency" . }}`,
			data: Data{
				GroupLabels: KV{
					"label1": "value $1",
					"label2": "value $2",
				},
				CommonLabels: KV{
					"tenant":  "example",
					"urgency": "low",
				},
			},
			exp: "An alert without a label urgency set to either high or low fired. The worst is assumed here: that the alert is of high urgency.\n" +
				"\n" +
				"This only happens when a misconfiguration happened on this alert, so it needs fixing. There are two actions required.\n" +
				"\n" +
				"The immediate action, is to evaluate the real urgency of the firing alert(s) and work on it accordingly.\n" +
				"\n" +
				"The secondary action, is to fix the alert configuration so that it fires with a correctly defined urgency next time.",
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			f := tmpl.ExecuteTextString
			got, err := f(globalDojoTemplate+tc.in, tc.data)
			if tc.fail {
				require.NotNil(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.exp, got)
		})
	}
}
