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
	"net/url"
	"testing"
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
