// Copyright The Prometheus Authors
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

package common

import (
	"encoding/json"
	"net/url"
	"testing"

	commoncfg "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

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
	commoncfg.MarshalSecretValue = true
	defer func() { commoncfg.MarshalSecretValue = false }()

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
			input:        MustParseURL("http://example.com/"),
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
