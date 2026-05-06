// Copyright 2019 Prometheus Team
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

// Package apiurl resolves Slack notifier api_url / api_url_file into outbound HTTP URLs.
package apiurl

import (
	"fmt"
	"os"
	"strings"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

// Resolver turns Slack notifier config (api_url or api_url_file) into the actual HTTP
// URL for each outgoing request (initial post, chat.update, etc.).
type Resolver struct {
	apiURL     *amcommoncfg.SecretURL
	apiURLFile string
}

// NewResolver captures api_url / api_url_file from config when the notifier is built.
//
// APIURLFile is stored as a path only. The file is read from disk on every call to
// URLForMethod when apiURL is nil—there is no in-memory cache of the URL string.
// That way changes to the file (rotation, out-of-band updates) apply to the next
// notification without restarting Alertmanager, matching historical behavior.
func NewResolver(apiURL *amcommoncfg.SecretURL, apiURLFile string) *Resolver {
	return &Resolver{apiURL: apiURL, apiURLFile: apiURLFile}
}

func (r *Resolver) URLForMethod(method string) (string, error) {
	if method == "" {
		if r.apiURL != nil {
			apiURLStr := r.apiURL.String()
			if apiURLStr != "" {
				return apiURLStr, nil
			}
			return "", fmt.Errorf("slack api url is empty")
		}

		// Read api_url_file on each resolution; see New.
		parsed, err := r.getURLFromFile()
		if err != nil {
			return "", err
		}
		return parsed.String(), nil
	}

	var baseURL *amcommoncfg.SecretURL
	if r.apiURL != nil {
		baseURL = r.apiURL
	} else {
		// Read api_url_file on each resolution; see New.
		parsed, err := r.getURLFromFile()
		if err != nil {
			return "", err
		}
		secret := amcommoncfg.SecretURL(*parsed)
		baseURL = &secret
	}

	return webAPIMethodURL(baseURL, method)
}

func (r *Resolver) getURLFromFile() (*amcommoncfg.URL, error) {
	content, err := os.ReadFile(r.apiURLFile)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(content))
	return amcommoncfg.ParseURL(raw)
}

// webAPIMethodURL returns a Slack Web API URL for the given method, using the same
// Scheme and host as postMessageURL. postMessageURL must be a URL whose path ends
// with a method name (e.g. .../api/chat.postMessage).
func webAPIMethodURL(postMessageURL *amcommoncfg.SecretURL, method string) (string, error) {
	if postMessageURL == nil || postMessageURL.URL == nil {
		return "", fmt.Errorf("slack api url is nil")
	}

	// Work on a copy so we never mutate the original URL.
	u := *postMessageURL.URL
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("slack api url %q is missing scheme or host", u.String())
	}

	pathWithoutTrailingSlash := strings.TrimSuffix(u.Path, "/")
	lastSlashIndex := strings.LastIndex(pathWithoutTrailingSlash, "/")
	if lastSlashIndex < 0 {
		return "", fmt.Errorf("slack api url %q has no path segment to replace", u.String())
	}
	u.Path = pathWithoutTrailingSlash[:lastSlashIndex+1] + method
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
