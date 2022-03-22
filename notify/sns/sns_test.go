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
	"context"
	"github.com/go-kit/log"
	"testing"
	"unicode/utf8"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify/test"
	commoncfg "github.com/prometheus/common/config"
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

func TestValidateAndTruncateSubject(t *testing.T) {
	var modifiedReasons []string
	var subject string
	notTruncate := make([]rune, 100)
	for i := range notTruncate {
		notTruncate[i] = 'e'
	}
	subject = validateAndTruncateSubject(logger, string(notTruncate), &modifiedReasons)
	require.Equal(t, string(notTruncate), subject)
	require.Equal(t, 100, utf8.RuneCountInString(string(subject)))
	require.Equal(t, 0, len(modifiedReasons))

	willBeTruncate := make([]rune, 101)
	for i := range willBeTruncate {
		willBeTruncate[i] = 'e'
	}
	subject = validateAndTruncateSubject(logger, string(willBeTruncate), &modifiedReasons)
	require.Equal(t, string(notTruncate), subject)
	require.Equal(t, 1, len(modifiedReasons))
	require.Equal(t, "Subject: Error - subject has been truncated from 101 characters because it exceeds the 100 character size limit", modifiedReasons[0])

	modifiedReasons = nil
	nonAsciiString := "\xc3\x28"
	subject = validateAndTruncateSubject(logger, nonAsciiString, &modifiedReasons)
	require.Equal(t, SubjectContainsIllegalChars, subject)
	require.Equal(t, 1, len(modifiedReasons))
	require.Equal(t, "Subject: Error - contains control- or non-ASCII characters", modifiedReasons[0])

	modifiedReasons = nil
	asciiControlString := "\a\b\t"
	subject = validateAndTruncateSubject(logger, asciiControlString, &modifiedReasons)
	require.Equal(t, SubjectContainsIllegalChars, subject)
	require.Equal(t, 1, len(modifiedReasons))
	require.Equal(t, "Subject: Error - contains control- or non-ASCII characters", modifiedReasons[0])

	modifiedReasons = nil
	newLineString := "abc\ndef"
	subject = validateAndTruncateSubject(logger, newLineString, &modifiedReasons)
	require.Equal(t, SubjectContainsIllegalChars, subject)
	require.Equal(t, 1, len(modifiedReasons))
	require.Equal(t, "Subject: Error - contains control- or non-ASCII characters", modifiedReasons[0])

	modifiedReasons = nil
	emptyString := ""
	subject = validateAndTruncateSubject(logger, emptyString, &modifiedReasons)
	require.Equal(t, SubjectEmpty, subject)
	require.Equal(t, 1, len(modifiedReasons))
	require.Equal(t, "Subject: Error - subject, if provided, must be non-empty", modifiedReasons[0])
}

func TestCreatePublishInput_noErrors(t *testing.T) {
	var ctx = context.Background()

	attributes := map[string]string{
		"attribName1": "attribValue1",
		"attribName2": "attribValue2",
		"attribName3": "attribValue3",
	}
	notifier, err := New(
		&config.SNSConfig{
			Attributes:  attributes,
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			TopicARN:    "TestTopic",
			PhoneNumber: "TestPhone",
			TargetARN:   "TestTarget",
			Subject:     "TestSubject",
			Message:     "TestMessage",
		},
		test.CreateTmpl(t),
		logger,
	)
	require.NoError(t, err)

	publishInput, err := notifier.createPublishInput(ctx, temlFunction(t))

	require.Equal(t, "TestTopic", *publishInput.TopicArn)
	require.Equal(t, "TestPhone", *publishInput.PhoneNumber)
	require.Equal(t, "TestTarget", *publishInput.TargetArn)
	require.Equal(t, "TestSubject", *publishInput.Subject)
	require.Equal(t, "TestMessage", *publishInput.Message)

	_, hasModifiedAttrib := publishInput.MessageAttributes["modified"]
	require.False(t, hasModifiedAttrib)
}

func TestCreatePublishInput_subjectOmitted(t *testing.T) {
	var ctx = context.Background()

	attributes := map[string]string{
		"attribName1": "attribValue1",
		"attribName2": "attribValue2",
		"attribName3": "attribValue3",
	}
	notifier, err := New(
		&config.SNSConfig{
			Attributes:  attributes,
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			TopicARN:    "TestTopic",
			PhoneNumber: "TestPhone",
			TargetARN:   "TestTarget",
			Subject:     "",
			Message:     "TestMessage",
		},
		test.CreateTmpl(t),
		logger,
	)
	require.NoError(t, err)

	publishInput, err := notifier.createPublishInput(ctx, temlFunction(t))

	require.Equal(t, "TestTopic", *publishInput.TopicArn)
	require.Equal(t, "TestPhone", *publishInput.PhoneNumber)
	require.Equal(t, "TestTarget", *publishInput.TargetArn)
	require.Nil(t, publishInput.Subject)
	require.Equal(t, "TestMessage", *publishInput.Message)

	require.Nil(t, publishInput.MessageAttributes["modified"])
}

func TestCreatePublishInput_subjectEmpty(t *testing.T) {
	var ctx = context.Background()

	attributes := map[string]string{
		"attribName1": "attribValue1",
		"attribName2": "attribValue2",
		"attribName3": "attribValue3",
	}
	notifier, err := New(
		&config.SNSConfig{
			Attributes:  attributes,
			HTTPConfig:  &commoncfg.HTTPClientConfig{},
			TopicARN:    "TestTopic",
			PhoneNumber: "TestPhone",
			TargetARN:   "TestTarget",
			Subject:     "TestSubject",
			Message:     "TestMessage",
		},
		test.CreateTmpl(t),
		logger,
	)
	require.NoError(t, err)
	temlFunc := func(input string) string {
		if input == "TestSubject" {
			return ""
		}
		return input
	}

	publishInput, err := notifier.createPublishInput(ctx, temlFunc)

	require.Equal(t, "TestTopic", *publishInput.TopicArn)
	require.Equal(t, "TestPhone", *publishInput.PhoneNumber)
	require.Equal(t, "TestTarget", *publishInput.TargetArn)
	require.Equal(t, SubjectEmpty, *publishInput.Subject)
	require.Equal(t, "TestMessage", *publishInput.Message)

	require.Contains(t, *publishInput.MessageAttributes["modified"].StringValue, SubjectEmpty)
}

func temlFunction(t *testing.T) func(string) string {
	return func(input string) string {
		return input
	}
}
