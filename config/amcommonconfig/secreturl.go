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

package amcommonconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"

	commoncfg "github.com/prometheus/common/config"
)

const secretToken = "<secret>"

var secretTokenJSON string

// SecretTemplateURL is a Secret string that represents a URL which may contain
// Go template syntax. Unlike SecretURL, it allows templated values and only
// validates non-templated URLs at unmarshal time.
type SecretTemplateURL commoncfg.Secret

// MarshalYAML implements the yaml.Marshaler interface for SecretTemplateURL.
func (s SecretTemplateURL) MarshalYAML() (any, error) {
	if s != "" {
		if commoncfg.MarshalSecretValue {
			return string(s), nil
		}
		return secretToken, nil
	}
	return nil, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for SecretTemplateURL.
func (s *SecretTemplateURL) UnmarshalYAML(unmarshal func(any) error) error {
	type plain commoncfg.Secret
	if err := unmarshal((*plain)(s)); err != nil {
		return err
	}

	urlStr := string(*s)

	// Skip validation for empty strings or secret token
	if urlStr == "" || urlStr == secretToken {
		return nil
	}

	// Check if the URL contains template syntax
	isTemplated, err := containsTemplating(urlStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	// Only validate as URL if it's not templated
	if !isTemplated {
		if _, err := parseURL(urlStr); err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}
	}

	return nil
}

// MarshalJSON implements the json.Marshaler interface for SecretTemplateURL.
func (s SecretTemplateURL) MarshalJSON() ([]byte, error) {
	return commoncfg.Secret(s).MarshalJSON()
}

// UnmarshalJSON implements the json.Unmarshaler interface for SecretTemplateURL.
func (s *SecretTemplateURL) UnmarshalJSON(data []byte) error {
	if string(data) == secretToken || string(data) == secretTokenJSON {
		*s = ""
		return nil
	}
	// Just unmarshal as a string since Secret doesn't have UnmarshalJSON
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = SecretTemplateURL(str)
	return nil
}

// containsTemplating checks if the string contains template syntax.
func containsTemplating(s string) (bool, error) {
	if !strings.Contains(s, "{{") {
		return false, nil
	}
	// If it contains template syntax, validate it's actually a valid templ.
	_, err := template.New("").Parse(s)
	if err != nil {
		return true, err
	}
	return true, nil
}

// URL is a custom type that represents an HTTP or HTTPS URL and allows validation at configuration load time.
type URL struct {
	*url.URL
}

func parseURL(s string) (*URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q for URL", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("missing host for URL")
	}
	return &URL{u}, nil
}
