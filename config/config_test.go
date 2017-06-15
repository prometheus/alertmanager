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
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestDefaultReceiverExists(t *testing.T) {
	in := `
route:
   group_wait: 30s
`

	conf := &Config{}
	err := yaml.Unmarshal([]byte(in), conf)

	expected := "Root route must specify a default receiver"

	if err == nil {
		t.Fatalf("no error returned, expected:\n%v", expected)
	}
	if err.Error() != expected {
		t.Errorf("\nexpected:\n%v\ngot:\n%v", expected, err.Error())
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
		t.Errorf("Error parsing %s: %s", "testdata/good.yml", err)
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
		t.Errorf("Error parsing %s: %s", "testdata/good.yml", err)
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
		t.Errorf("Error parsing %s: %s", "testdata/good.yml", err)
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
