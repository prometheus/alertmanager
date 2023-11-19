// Copyright 2021 Prometheus Team
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

package sns

import (
	"github.com/go-kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	commoncfg "github.com/prometheus/common/config"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

var logger = log.NewNopLogger()

func TestValidateAndTruncateMessage(t *testing.T) {
	sBuff := make([]byte, 257*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	truncatedMessage, isTruncated, err := validateAndTruncateMessage(string(sBuff), 256*1024)
	require.True(t, isTruncated)
	require.NoError(t, err)
	require.NotEqual(t, sBuff, truncatedMessage)
	require.Equal(t, len(truncatedMessage), 256*1024)

	sBuff = make([]byte, 100)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	truncatedMessage, isTruncated, err = validateAndTruncateMessage(string(sBuff), 100)
	require.False(t, isTruncated)
	require.NoError(t, err)
	require.Equal(t, string(sBuff), truncatedMessage)

	invalidUtf8String := "\xc3\x28"
	_, _, err = validateAndTruncateMessage(invalidUtf8String, 100)
	require.Error(t, err)
}

func TestCreateAndValidateMessageAttributes(t *testing.T) {
	var modifiedReasons []string
	attributes := map[string]string{
		"Invalid0":        "",
		".Invalid1":       "123",
		"Invalid2.":       "123",
		"AWS.Invalid3":    "123",
		"Amazon.Invalid4": "123",
		"Invalid..5":      "123",
		"Valid0":          "123",
		"AmazonValid1":    "123",
		"valid.2":         "123",
		"valid-_3":        "123",
	}
	notifier, err := New(
		&config.SNSConfig{
			Attributes: attributes,
			HTTPConfig: &commoncfg.HTTPClientConfig{},
		},
		CreateTmpl(t),
		logger,
	)
	require.NoError(t, err)

	attributesAfterValidation := notifier.createAndValidateMessageAttributes(temlFunction(t), &modifiedReasons)

	require.Equal(t, 4, len(attributesAfterValidation))
	require.Equal(t, true, attributesAfterValidation["Valid0"] != nil)
	require.Equal(t, true, attributesAfterValidation["AmazonValid1"] != nil)
	require.Equal(t, true, attributesAfterValidation["valid.2"] != nil)
	require.Equal(t, true, attributesAfterValidation["valid-_3"] != nil)
	require.Equal(t, len(modifiedReasons), 1)
	require.Equal(t, "MessageAttribute: Error - 6 of message attributes have been removed because of invalid MessageAttributeKey or MessageAttributeValue", modifiedReasons[0])
}

// CreateTmpl returns a ready-to-use template.
func CreateTmpl(t *testing.T) *template.Template {
	tmpl, err := template.FromGlobs()
	require.NoError(t, err)
	tmpl.ExternalURL, _ = url.Parse("http://am")
	return tmpl
}

// CreateTmpl returns a ready-to-use template.
func temlFunction(t *testing.T) func(string) string {
	return func(input string) string {
		return input
	}
}
