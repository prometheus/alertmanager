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

package apiurl

import (
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
)

const (
	baseURL                = "https://slack.com"
	defaultURLWithAPIPath  = baseURL + "/api/chat.postMessage"
	defaultURLWithFilePath = baseURL + "/file/chat.postMessage"

	addReactionsMethod = "reactions.add"
	updateChatMethod   = "chat.update"
)

func TestResolver_URLForMethod_ValidScenarios(t *testing.T) {
	t.Parallel()

	defaultAPIURLFile := createTempAPIURLFile(t, "test_url_for_method", defaultURLWithFilePath)

	tests := []struct {
		name       string
		apiURL     *amcommoncfg.SecretURL
		apiURLFile string
		method     string
		want       string
	}{
		{
			name:       "empty method with apiURL",
			apiURL:     getSecretURL(t, defaultURLWithAPIPath),
			apiURLFile: "",
			method:     "",
			want:       defaultURLWithAPIPath,
		},
		{
			name:       "empty method without apiURL but with apiURLFile",
			apiURL:     nil,
			apiURLFile: defaultAPIURLFile.Name(),
			method:     "",
			want:       defaultURLWithFilePath,
		},
		{
			name:       "empty method with apiURL and apiURLFile, should use apiURL",
			apiURL:     getSecretURL(t, defaultURLWithAPIPath),
			apiURLFile: defaultAPIURLFile.Name(),
			method:     "",
			want:       defaultURLWithAPIPath,
		},
		{
			name:       "method chat.update with apiURL",
			apiURL:     getSecretURL(t, defaultURLWithAPIPath),
			apiURLFile: defaultAPIURLFile.Name(),
			method:     updateChatMethod,
			want:       baseURL + "/api/" + updateChatMethod,
		},
		{
			name:       "method chat.update with apiURL with trailing slash",
			apiURL:     getSecretURL(t, defaultURLWithAPIPath+"/"),
			apiURLFile: defaultAPIURLFile.Name(),
			method:     updateChatMethod,
			want:       baseURL + "/api/" + updateChatMethod,
		},
		{
			name:       "method reactions.add with apiURLFile",
			apiURL:     nil,
			apiURLFile: defaultAPIURLFile.Name(),
			method:     addReactionsMethod,
			want:       baseURL + "/file/" + addReactionsMethod,
		},
		{
			name:       "method reactions.add with apiURLFile with empty spaces",
			apiURL:     nil,
			apiURLFile: createTempAPIURLFile(t, "test_url_for_method_with_new_lines_and_spaces", defaultURLWithFilePath+"\n    \n").Name(),
			method:     addReactionsMethod,
			want:       baseURL + "/file/" + addReactionsMethod,
		},
		{
			name:       "method reactions.add with apiURLFile with trailing slash",
			apiURL:     nil,
			apiURLFile: createTempAPIURLFile(t, "test_url_for_method_with_trailing_slash", defaultURLWithFilePath+"/").Name(),
			method:     addReactionsMethod,
			want:       baseURL + "/file/" + addReactionsMethod,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := NewResolver(tt.apiURL, tt.apiURLFile)

			got, err := resolver.URLForMethod(tt.method)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolver_URLForMethod_InvalidScenarios(t *testing.T) {
	t.Parallel()

	invalidURL := amcommoncfg.SecretURL{
		URL: &url.URL{},
	}
	tests := []struct {
		name             string
		apiURL           *amcommoncfg.SecretURL
		apiURLFile       string
		method           string
		expectedErrorMsg string
	}{
		{
			name:             "invalid URL",
			apiURL:           &invalidURL,
			apiURLFile:       "",
			method:           "",
			expectedErrorMsg: "slack api url is empty",
		},
		{
			name:             "no apiURL nor apiURLFile",
			apiURL:           nil,
			apiURLFile:       "unknown",
			method:           "",
			expectedErrorMsg: "open unknown: no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := NewResolver(tt.apiURL, tt.apiURLFile)

			_, err := resolver.URLForMethod(tt.method)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectedErrorMsg)
		})
	}
}

func getSecretURL(t *testing.T, raw string) *amcommoncfg.SecretURL {
	t.Helper()
	u, err := amcommoncfg.ParseURL(raw)
	require.NoError(t, err)
	s := amcommoncfg.SecretURL(*u)
	return &s
}

func createTempAPIURLFile(t *testing.T, pattern, url string) *os.File {
	t.Helper()
	apiURLFileWithNewLines, err := os.CreateTemp(t.TempDir(), pattern)
	require.NoError(t, err)
	_, err = apiURLFileWithNewLines.WriteString(url)
	require.NoError(t, err)
	require.NoError(t, apiURLFileWithNewLines.Close())
	return apiURLFileWithNewLines
}
