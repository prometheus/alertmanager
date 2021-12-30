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
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/go-kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	commoncfg "github.com/prometheus/common/config"
	"net/url"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

var logger = log.NewNopLogger()

func TestValidateMessage(t *testing.T) {
	var modifiedReasons []string

	invalidUtf8String := "\xc3\x28"
	err := validateMessage(logger, invalidUtf8String, &modifiedReasons)
	require.Equal(t, MessageNotValidUtf8, err.Error())
	require.Equal(t, 1, len(modifiedReasons))
	require.Equal(t, "Message: Error - not a valid UTF-8 encoded string", modifiedReasons[0])
	require.Equal(t, len(modifiedReasons), 1)

	emptyString := ""
	err = validateMessage(logger, emptyString, &modifiedReasons)
	require.Equal(t, MessageIsEmpty, err.Error())
	require.Equal(t, 2, len(modifiedReasons))
	require.Equal(t, "Message: Error - the message should not be empty", modifiedReasons[1])
}

func TestValidateAndTruncateSubject(t *testing.T) {
	var modifiedReasons []string
	notTruncate := make([]rune, 100)
	for i := range notTruncate {
		notTruncate[i] = 'e'
	}
	truncatedMessage := validateAndTruncateSubject(logger, string(notTruncate), &modifiedReasons)
	require.NotEqual(t, notTruncate, truncatedMessage)
	require.Equal(t, 100, utf8.RuneCountInString(string(truncatedMessage)))

	willBeTruncate := make([]rune, 101)
	for i := range willBeTruncate {
		willBeTruncate[i] = 'e'
	}
	truncatedMessage = validateAndTruncateSubject(logger, string(willBeTruncate), &modifiedReasons)
	require.Equal(t, string(notTruncate), truncatedMessage)
	require.Equal(t, len(modifiedReasons), 1)
	require.Equal(t, "Subject: Error - subject has been truncated from 101 characters because it exceeds the 100 character size limit", modifiedReasons[0])

	invalidAsciiString := "\xc3\x28"
	truncatedMessage = validateAndTruncateSubject(logger, invalidAsciiString, &modifiedReasons)
	require.Equal(t, truncatedMessage, SubjectNotASCII)
	require.Equal(t, len(modifiedReasons), 2)
	require.Equal(t, "Subject: Error - contains non printable ASCII characters", modifiedReasons[1])
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

	attributesAfterValidation := createAndValidateMessageAttributes(notifier, temlFunction(t), &modifiedReasons)

	require.Equal(t, 4, len(attributesAfterValidation))
	require.Equal(t, true, attributesAfterValidation["Valid0"] != nil)
	require.Equal(t, true, attributesAfterValidation["AmazonValid1"] != nil)
	require.Equal(t, true, attributesAfterValidation["valid.2"] != nil)
	require.Equal(t, true, attributesAfterValidation["valid-_3"] != nil)
	require.Equal(t, len(modifiedReasons), 1)
	require.Equal(t, "MessageAttribute: Error - 6 of message attributes have been removed because of invalid MessageAttributeKey or MessageAttributeValue", modifiedReasons[0])
}

func TestAddModifiedMessageAttributes(t *testing.T) {
	reasons := []string{"1", "2"}
	attributes := map[string]*sns.MessageAttributeValue{
		"truncated": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String("true")},
	}

	addModifiedMessageAttributes(attributes, reasons)

	require.Equal(t, 2, len(attributes))
	require.Equal(t, "[\"1\",\"2\"]", *attributes["modified"].StringValue)
}

func TestTruncateMessageAttributesAndMessage_TotalSmallerThanSizeLimit(t *testing.T) {
	logger := log.NewNopLogger()

	reasons := []string{"1", "2"}
	sBuff := make([]byte, 30*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}

	attributes := map[string]*sns.MessageAttributeValue{
		"truncated":  &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String("true")},
		"customized": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
	}

	truncateAttributes, truncatedMessage, _ := truncateMessageAttributesAndMessage(logger, "", attributes, string(sBuff), false, &reasons)
	require.Equal(t, 2, len(truncateAttributes))
	require.Equal(t, len(string(sBuff)), len(truncatedMessage))
	require.Equal(t, 2, len(reasons))
	require.Equal(t, true, getTotalSizeInBytes(reasons, truncateAttributes, truncatedMessage) <= messageSizeLimitInBytes)
}

func TestTruncateMessageAttributesAndMessage_SMS(t *testing.T) {
	reasons := []string{"1", "2"}
	smsBuff := make([]rune, 1700)
	for i := range smsBuff {
		smsBuff[i] = 'e'
	}
	attributes := map[string]*sns.MessageAttributeValue{
		"truncated":  &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String("true")},
		"customized": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(smsBuff))},
	}
	_, truncatedMessage, _ := truncateMessageAttributesAndMessage(logger, "123", attributes, string(smsBuff), false, &reasons)
	require.Equal(t, messageSizeLimitInCharactersForSMS, utf8.RuneCountInString(truncatedMessage))
}

func TestTruncateMessageAttributesAndMessage_MessageAttributesLargerThanSizeLimit(t *testing.T) {
	reasons := []string{"1", "2"}
	sBuff := make([]byte, 150*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	attributes := map[string]*sns.MessageAttributeValue{
		"truncated":   &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String("true")},
		"customized1": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
		"customized2": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
	}
	truncateAttributes, truncatedMessage, _ := truncateMessageAttributesAndMessage(logger, "", attributes, string(sBuff), false, &reasons)
	require.Equal(t, 2, len(truncateAttributes))
	require.Equal(t, true, len(truncatedMessage) < 150*1024)
	require.Equal(t, "true", *truncateAttributes["truncated"].StringValue)
	require.Equal(t, 4, len(reasons))
	require.Equal(t, true, getTotalSizeInBytes(reasons, truncateAttributes, truncatedMessage) < messageSizeLimitInBytes)
}

func TestTruncateMessageAttributesAndMessage_messageHasBeenModified(t *testing.T) {
	// messageAttributes + message > 256KB, however the message has already been modified, truncate the messageAttributes and keep the original message
	reasons := []string{"1", "2"}
	sBuff := make([]byte, 150*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	attributes := map[string]*sns.MessageAttributeValue{
		"truncated":   &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String("true")},
		"customized1": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
		"customized2": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
	}
	truncateAttributes, truncatedMessage, _ := truncateMessageAttributesAndMessage(logger, "", attributes, string(sBuff), true, &reasons)
	require.Equal(t, 1, len(truncateAttributes))
	require.Equal(t, 150*1024, len(truncatedMessage))
	require.Equal(t, 3, len(reasons))
	require.Equal(t, true, getTotalSizeInBytes(reasons, truncateAttributes, truncatedMessage) <= messageSizeLimitInBytes)

}

func TestTruncateMessageAttributesAndMessage_atLeast1ByteForMessage(t *testing.T) {
	//we still have rooms for reasons and at least 1 byte for message
	messageBuff := make([]byte, messageSizeLimitInBytes)
	for i := range messageBuff {
		messageBuff[i] = byte(33)
	}

	reservedMessageModifiedBytes, _ := getMessageSizeExceedReservedBytes(string(messageBuff))
	reasons := []string{"1", "2"}
	modifiedReasonBytes, _ := getModifiedReasonMessageAttributeSize(reasons)
	sBuff := make([]byte, messageSizeLimitInBytes-reservedMessageModifiedBytes-1-modifiedReasonBytes-len("customized1")-len("String"))
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	attributes := map[string]*sns.MessageAttributeValue{
		"customized1": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
	}
	truncateAttributes, truncatedMessage, _ := truncateMessageAttributesAndMessage(logger, "", attributes, string(sBuff), false, &reasons)
	require.Equal(t, 2, len(truncateAttributes))
	require.Equal(t, "true", *truncateAttributes["truncated"].StringValue)
	require.Equal(t, true, len(truncatedMessage) >= 1)
	require.Equal(t, 3, len(reasons))
	fmt.Println("message", len(truncatedMessage))
	require.Equal(t, true, getTotalSizeInBytes(reasons, truncateAttributes, truncatedMessage) <= messageSizeLimitInBytes)
}

func TestTruncateMessageAttributesAndMessage_truncateMessage(t *testing.T) {
	reasons := []string{"1", "2"}
	sBuff := make([]byte, 3*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	attributes := map[string]*sns.MessageAttributeValue{
		"customized1": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
	}

	sBuffMessage := make([]byte, 256*1024)

	truncateAttributes, truncatedMessage, _ := truncateMessageAttributesAndMessage(logger, "", attributes, string(sBuffMessage), false, &reasons)
	require.Equal(t, 2, len(truncateAttributes))
	require.Equal(t, "true", *truncateAttributes["truncated"].StringValue)
	require.Equal(t, true, len(truncatedMessage) >= 1)
	require.Equal(t, 3, len(reasons))
	require.Equal(t, true, getTotalSizeInBytes(reasons, truncateAttributes, truncatedMessage) <= messageSizeLimitInBytes)
}

func TestTruncateMessageAttributesAndMessage_exactSize(t *testing.T) {
	var reasons []string
	sBuff := make([]byte, 128*1024-len("String")-len("customized1"))
	for i := range sBuff {
		sBuff[i] = byte(33)
	}
	attributes := map[string]*sns.MessageAttributeValue{
		"customized1": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
	}

	sBuffMessage := make([]byte, 128*1024)

	truncateAttributes, truncatedMessage, _ := truncateMessageAttributesAndMessage(logger, "", attributes, string(sBuffMessage), false, &reasons)
	require.Equal(t, 1, len(truncateAttributes))
	require.Equal(t, true, truncateAttributes["truncated"] == nil)
	require.Equal(t, true, len(truncatedMessage) == 128*1024)
	require.Equal(t, 0, len(reasons))
	require.Equal(t, true, getTotalSizeInBytes(reasons, truncateAttributes, truncatedMessage) == 256*1024)
}

func TestTruncateMessageAttributesAndMessage_marshalFailure(t *testing.T) {
	storedMarshal := jsonMarshal
	jsonMarshal = fakemarshal
	defer restoremarshal(storedMarshal)

	reasons := []string{"1", "2"}
	sBuff := make([]byte, 30*1024)
	for i := range sBuff {
		sBuff[i] = byte(33)
	}

	attributes := map[string]*sns.MessageAttributeValue{
		"truncated":  &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String("true")},
		"customized": &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(string(sBuff))},
	}

	_, _, err := truncateMessageAttributesAndMessage(logger, "", attributes, string(sBuff), false, &reasons)
	require.Equal(t, true, err != nil)
}

func getTotalSizeInBytes(modifiedReasons []string, attributes map[string]*sns.MessageAttributeValue, message string) int {
	attributesSize := 0
	for k, v := range attributes {
		attributesSize += len(k) + len(*v.DataType) + len(*v.StringValue)
	}

	modifiedReasonsSize := 0
	if len(modifiedReasons) > 0 {
		jsonString, _ := json.Marshal(modifiedReasons)
		modifiedReasonsSize = len("String.Array") + len("modified") + len(string(jsonString))
	}
	return modifiedReasonsSize + attributesSize + len(message)
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

func fakemarshal(v interface{}) ([]byte, error) {
	return []byte{}, errors.New("Marshalling failed")
}

func restoremarshal(replace func(v interface{}) ([]byte, error)) {
	jsonMarshal = replace
}
