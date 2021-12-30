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
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

const (
	// Message components
	Message          = "Message"
	Subject          = "Subject"
	MessageAttribute = "MessageAttribute"

	// Modified Message attribute value format
	ComponentAndModifiedReason = "%s: %s"

	// The errors
	MessageNotValidUtf8                = "Error - not a valid UTF-8 encoded string"
	MessageIsEmpty                     = "Error - the message should not be empty"
	MessageSizeExceeded                = "Error - the message has been truncated from %dKB because it exceeds the %dKB size limit"
	SubjectNotASCII                    = "Error - contains non printable ASCII characters"
	SubjectSizeExceeded                = "Error - subject has been truncated from %d characters because it exceeds the 100 character size limit"
	MessageAttributeSizeExceeded       = "Error - %d of message attributes have been removed because of %dKB size limit exceeded"
	MessageAttributeNotValidKeyOrValue = "Error - %d of message attributes have been removed because of invalid MessageAttributeKey or MessageAttributeValue"

	// Message components size limit
	subjectSizeLimitInCharacters         = 100
	messageAttributeKeyLimitInCharacters = 256
	// Max message size for a message in a SNS publish request is 256KB, except for SMS messages where the limit is 1600 characters/runes.
	messageSizeLimitInBytes            = 256 * 1024
	messageSizeLimitInCharactersForSMS = 1600
)

var isInvalidMessageAttributeKeyPrefix = regexp.MustCompile(`^(AWS\.)|^(Amazon\.)|^(\.)`).MatchString
var isInvalidMessageAttributeKeySuffix = regexp.MustCompile(`\.$`).MatchString
var isInvalidMessageAttributeKeySubstring = regexp.MustCompile(`\.{2}`).MatchString
var isValidMessageAttributeKeyCharacters = regexp.MustCompile(`^[a-zA-Z0-9_\-.]*$`).MatchString

var truncatedMessageAttributeKey = "truncated"
var truncatedMessageAttributeValue = &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String("true")}
var modifiedMessageAttributeKey = "modified"

//Used for testing
var jsonMarshal = json.Marshal

// Notifier implements a Notifier for SNS notifications.
type Notifier struct {
	conf    *config.SNSConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
	isFifo  *bool
}

// New returns a new SNS notification handler.
func New(c *config.SNSConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "sns", httpOpts...)
	if err != nil {
		return nil, err
	}
	return &Notifier{
		conf:    c,
		tmpl:    t,
		logger:  l,
		client:  client,
		retrier: &notify.Retrier{},
	}, nil
}

func (n *Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	var (
		err  error
		data = notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
		tmpl = notify.TmplText(n.tmpl, data, &err)
	)

	client, err := createSNSClient(n.client, n, tmpl)
	if err != nil {
		if e, ok := err.(awserr.RequestFailure); ok {
			return n.retrier.Check(e.StatusCode(), strings.NewReader(e.Message()))
		} else {
			return true, err
		}
	}

	publishInput, err := createPublishInput(ctx, n, tmpl)
	if err != nil {
		return true, err
	}

	publishOutput, err := client.Publish(publishInput)
	if err != nil {
		if e, ok := err.(awserr.RequestFailure); ok {
			return n.retrier.Check(e.StatusCode(), strings.NewReader(e.Message()))
		} else {
			return true, err
		}
	}

	level.Debug(n.logger).Log("msg", "SNS message successfully published", "message_id", publishOutput.MessageId, "sequence number", publishOutput.SequenceNumber)

	return false, nil
}

func createSNSClient(httpClient *http.Client, n *Notifier, tmpl func(string) string) (*sns.SNS, error) {
	var creds *credentials.Credentials = nil
	// If there are provided sigV4 credentials we want to use those to create a session.
	if n.conf.Sigv4.AccessKey != "" && n.conf.Sigv4.SecretKey != "" {
		creds = credentials.NewStaticCredentials(n.conf.Sigv4.AccessKey, string(n.conf.Sigv4.SecretKey), "")
	}
	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:   aws.String(n.conf.Sigv4.Region),
			Endpoint: aws.String(tmpl(n.conf.APIUrl)),
		},
		Profile: n.conf.Sigv4.Profile,
	})
	if err != nil {
		return nil, err
	}

	if n.conf.Sigv4.RoleARN != "" {
		var stsSess *session.Session
		if n.conf.APIUrl == "" {
			stsSess = sess
		} else {
			// If we have set the API URL we need to create a new session to get the STS Credentials.
			stsSess, err = session.NewSessionWithOptions(session.Options{
				Config: aws.Config{
					Region:      aws.String(n.conf.Sigv4.Region),
					Credentials: creds,
				},
				Profile: n.conf.Sigv4.Profile,
			})
			if err != nil {
				return nil, err
			}
		}
		creds = stscreds.NewCredentials(stsSess, n.conf.Sigv4.RoleARN)
	}
	// Use our generated session with credentials to create the SNS Client.
	client := sns.New(sess, &aws.Config{Credentials: creds, HTTPClient: httpClient})
	// We will always need a region to be set by either the local config or the environment.
	if aws.StringValue(sess.Config.Region) == "" {
		return nil, fmt.Errorf("region not configured in sns.sigv4.region or in default credentials chain")
	}
	return client, nil
}

func createPublishInput(ctx context.Context, n *Notifier, tmpl func(string) string) (*sns.PublishInput, error) {
	var modifiedReasons []string
	publishInput := &sns.PublishInput{}
	messageAttributes := createAndValidateMessageAttributes(n, tmpl, &modifiedReasons)
	if n.conf.TopicARN != "" {
		topicTmpl := tmpl(n.conf.TopicARN)
		publishInput.SetTopicArn(topicTmpl)
		if n.isFifo == nil {
			// If we are using a topic ARN it could be a FIFO topic specified by the topic postfix .fifo.
			n.isFifo = aws.Bool(n.conf.TopicARN[len(n.conf.TopicARN)-5:] == ".fifo")
		}
		if *n.isFifo {
			// Deduplication key and Message Group ID are only added if it's a FIFO SNS Topic.
			key, err := notify.ExtractGroupKey(ctx)
			if err != nil {
				return nil, err
			}
			publishInput.SetMessageDeduplicationId(key.Hash())
			publishInput.SetMessageGroupId(key.Hash())
		}
	}
	if n.conf.PhoneNumber != "" {
		publishInput.SetPhoneNumber(tmpl(n.conf.PhoneNumber))
	}
	if n.conf.TargetARN != "" {
		publishInput.SetTargetArn(tmpl(n.conf.TargetARN))
	}

	messageToSend := tmpl(n.conf.Message)
	validationErr := validateMessage(n.logger, messageToSend, &modifiedReasons)

	if validationErr != nil {
		messageToSend = validationErr.Error()
		// If we modified the message with error message we need to add a message attribute showing that it was truncated.
		messageAttributes[truncatedMessageAttributeKey] = truncatedMessageAttributeValue
	}

	if n.conf.Subject != "" {
		subjectToSend := validateAndTruncateSubject(n.logger, tmpl(n.conf.Subject), &modifiedReasons)

		publishInput.SetSubject(subjectToSend)
	}

	truncateAttributes, truncatedMessage, err := truncateMessageAttributesAndMessage(n.logger, n.conf.PhoneNumber, messageAttributes, messageToSend, validationErr != nil, &modifiedReasons)
	if err != nil {
		return nil, err
	}

	err = addModifiedMessageAttributes(truncateAttributes, modifiedReasons)
	if err != nil {
		return nil, err
	}

	publishInput.SetMessage(truncatedMessage)
	publishInput.SetMessageAttributes(truncateAttributes)

	return publishInput, nil
}

func addModifiedMessageAttributes(attributes map[string]*sns.MessageAttributeValue, modifiedReasons []string) error {
	if len(modifiedReasons) > 0 {
		valueString, err := getModifiedReasonMessageAttributeValue(modifiedReasons)
		if err != nil {
			return err
		}
		attributes[modifiedMessageAttributeKey] = &sns.MessageAttributeValue{DataType: aws.String("String.Array"), StringValue: aws.String(valueString)}
	}

	return nil
}

func validateMessage(logger log.Logger, message string, modifiedReasons *[]string) error {
	if !utf8.ValidString(message) {
		*modifiedReasons = append(*modifiedReasons, fmt.Sprintf(ComponentAndModifiedReason, Message, MessageNotValidUtf8))
		level.Info(logger).Log("msg", "message has been modified because of not a valid UTF-8 encoded string", "originalMessage", message)
		return errors.New(MessageNotValidUtf8)
	}
	if len(message) == 0 {
		*modifiedReasons = append(*modifiedReasons, fmt.Sprintf(ComponentAndModifiedReason, Message, MessageIsEmpty))
		level.Info(logger).Log("msg", "message has been modified because of empty")
		return errors.New(MessageIsEmpty)
	}
	return nil
}

func validateAndTruncateSubject(logger log.Logger, subject string, modifiedReasons *[]string) string {
	if !isASCII(subject) {
		*modifiedReasons = append(*modifiedReasons, fmt.Sprintf(ComponentAndModifiedReason, Subject, SubjectNotASCII))
		level.Info(logger).Log("msg", "subject has been modified because of contains non printable ASCII characters", "originalSubject", subject)
		return SubjectNotASCII
	}

	charactersInSubject := utf8.RuneCountInString(subject)
	if charactersInSubject <= subjectSizeLimitInCharacters {
		return subject
	}

	// If the message is larger than our specified size we have to truncate.
	level.Info(logger).Log("msg", "subject has been truncated because of size limit exceeded", "originalSubject", subject)
	*modifiedReasons = append(*modifiedReasons, fmt.Sprintf(ComponentAndModifiedReason, Subject, fmt.Sprintf(SubjectSizeExceeded, charactersInSubject)))
	return subject[:subjectSizeLimitInCharacters]
}

func createAndValidateMessageAttributes(n *Notifier, tmpl func(string) string, modifiedReasons *[]string) map[string]*sns.MessageAttributeValue {
	numberOfInvalidMessageAttributes := 0
	// Convert the given attributes map into the AWS Message Attributes Format.
	attributes := make(map[string]*sns.MessageAttributeValue, len(n.conf.Attributes))
	for k, v := range n.conf.Attributes {
		attributeKey := tmpl(k)
		attributeValue := tmpl(v)
		if !isValidateMessageAttribute(attributeKey, attributeValue) {
			numberOfInvalidMessageAttributes++
			level.Debug(n.logger).Log("msg", "messageAttribute has been removed because of invalid key/value", "attributeKey", attributeKey, "attributeValue", attributeValue)
			continue
		}
		attributes[attributeKey] = &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(attributeValue)}
	}

	if numberOfInvalidMessageAttributes > 0 {
		level.Info(n.logger).Log("msg", "messageAttributes has been removed because of invalid key/value", "numberOfRemovedAttributes", numberOfInvalidMessageAttributes)
		*modifiedReasons = append(
			*modifiedReasons,
			fmt.Sprintf(ComponentAndModifiedReason, MessageAttribute, fmt.Sprintf(MessageAttributeNotValidKeyOrValue, numberOfInvalidMessageAttributes)),
		)
	}
	return attributes
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

/*
	The priority to fit in the size is
    1. The modified reasons why will become attributes["modified"]
    2. message attributes
    3. message
*/
func truncateMessageAttributesAndMessage(logger log.Logger, phoneNumber string, attributes map[string]*sns.MessageAttributeValue,
	message string, isMessageModified bool, modifiedReasons *[]string) (map[string]*sns.MessageAttributeValue, string, error) {
	if phoneNumber != "" {
		charactersInSubject := utf8.RuneCountInString(message)
		if charactersInSubject <= messageSizeLimitInCharactersForSMS {
			return attributes, message, nil
		}

		// SMS doesn't use customized messageAttributes
		return attributes, message[:messageSizeLimitInCharactersForSMS], nil
	}

	truncatedAttributes, attributesSize, err := truncateMessageAttributes(logger, attributes, modifiedReasons, isMessageModified, message)

	if err != nil {
		return attributes, message, err
	}

	truncatedMessage, isMessageTruncated, err := truncateMessage(logger, modifiedReasons, message, attributesSize)
	if err != nil {
		return attributes, message, err
	}

	if isMessageTruncated {
		truncatedAttributes[truncatedMessageAttributeKey] = truncatedMessageAttributeValue
	}

	return truncatedAttributes, truncatedMessage, nil
}

func createMessageAttributeSizeExceededReason(numberOfAttributeToBeTruncate int) string {
	return fmt.Sprintf(ComponentAndModifiedReason,
		MessageAttribute,
		fmt.Sprintf(MessageAttributeSizeExceeded, numberOfAttributeToBeTruncate, messageSizeLimitInBytes/1024))
}

func createMessageSizeExceededReason(originMessage string) string {
	return fmt.Sprintf(
		ComponentAndModifiedReason,
		Message,
		fmt.Sprintf(MessageSizeExceeded, len(originMessage)/1024, messageSizeLimitInBytes/1024))
}

func getMessageSizeExceedReservedBytes(message string) (int, error) {
	reservedTruncateAttributeValue := truncatedMessageAttributeValue
	reservedTruncateAttributeBytes :=
		len(truncatedMessageAttributeKey) + len(*reservedTruncateAttributeValue.DataType) + len(*reservedTruncateAttributeValue.StringValue)
	reservedMessageModifiedReasons := []string{
		createMessageSizeExceededReason(message),
	}
	reservedMessageModifiedReasonsBytes, err := getModifiedReasonMessageAttributeSize(reservedMessageModifiedReasons)
	if err != nil {
		return 0, err
	}

	return reservedTruncateAttributeBytes + reservedMessageModifiedReasonsBytes, nil
}

func truncateMessage(logger log.Logger, modifiedReasons *[]string, message string, attributeSize int) (string, bool, error) {
	modifiedReasonBytes, err := getModifiedReasonMessageAttributeSize(*modifiedReasons)
	if err != nil {
		return message, false, err
	}

	availableBytes := messageSizeLimitInBytes - modifiedReasonBytes - attributeSize

	if len(message) <= availableBytes {
		return message, false, nil
	}

	messageSizeExceedReservedBytes, err := getMessageSizeExceedReservedBytes(message)
	if err != nil {
		return message, false, err
	}
	availableBytes -= messageSizeExceedReservedBytes
	// If the message is larger than our specified size we have to truncate.
	*modifiedReasons = append(*modifiedReasons, createMessageSizeExceededReason(message))

	truncated := make([]byte, availableBytes)
	copy(truncated, message)
	level.Info(logger).Log("msg", "message has been truncated because of size limit exceeded", "originSize", len(message), "truncatedSize", len(truncated))
	return string(truncated), true, nil
}

func truncateMessageAttributes(logger log.Logger, attributes map[string]*sns.MessageAttributeValue,
	modifiedReasons *[]string, isMessageModified bool, message string) (map[string]*sns.MessageAttributeValue, int, error) {

	modifiedReasonBytes, err := getModifiedReasonMessageAttributeSize(*modifiedReasons)
	if err != nil {
		return attributes, 0, err
	}

	availableBytes := messageSizeLimitInBytes - modifiedReasonBytes

	// We need to at least keep 1 byte for the message
	availableBytes = availableBytes - 1
	// If message already gets modified, it means we replace the original message with an error. We don't want to truncate message in this case
	if isMessageModified {
		availableBytes = availableBytes - len(message)
	}

	truncatedAttributes, attributeSize := fitMessageAttributeInAvailableSize(attributes, availableBytes)

	reservedMessageAttributeModifiedReasons := []string{
		// reserved for maximum number of attributes can be truncate
		createMessageAttributeSizeExceededReason(len(attributes)),
	}
	reservedMessageAttributeModifiedReasonsBytes, err := getModifiedReasonMessageAttributeSize(reservedMessageAttributeModifiedReasons)
	if err != nil {
		return truncatedAttributes, attributeSize, err
	}

	if len(truncatedAttributes) < len(attributes) {
		availableBytes -= reservedMessageAttributeModifiedReasonsBytes
		// truncate message attributes again in order to fit in the message attribute modified reasons
		truncatedAttributes, attributeSize = fitMessageAttributeInAvailableSize(attributes, availableBytes)
	}

	reservedMessageModifiedBytes, err := getMessageSizeExceedReservedBytes(message)
	if err != nil {
		return truncatedAttributes, attributeSize, err
	}

	if !isMessageModified && len(message) > availableBytes-attributeSize {
		availableBytes -= reservedMessageModifiedBytes
		// truncate message attributes again in order to fit in the message modified reasons
		truncatedAttributes, attributeSize = fitMessageAttributeInAvailableSize(attributes, availableBytes)
	}

	if len(truncatedAttributes) < len(attributes) {
		removedNumber := len(attributes) - len(truncatedAttributes)
		level.Info(logger).Log("msg", "messageAttributes has been removed because of size limit exceeded", "numberOfRemovedAttributes", removedNumber)
		*modifiedReasons = append(*modifiedReasons, createMessageAttributeSizeExceededReason(removedNumber))
	}

	return truncatedAttributes, attributeSize, nil
}

func fitMessageAttributeInAvailableSize(attributes map[string]*sns.MessageAttributeValue, availableBytes int) (map[string]*sns.MessageAttributeValue, int) {
	attributesSize := 0
	truncatedAttributes := make(map[string]*sns.MessageAttributeValue)

	for k, v := range attributes {
		pendingAddingAttributeSize := len(k) + len(*v.DataType) + len(*v.StringValue)
		if attributesSize+pendingAddingAttributeSize <= availableBytes {
			truncatedAttributes[k] = v
			attributesSize += pendingAddingAttributeSize
		}
	}

	return truncatedAttributes, attributesSize
}

func getModifiedReasonMessageAttributeSize(modifiedReasons []string) (int, error) {
	if len(modifiedReasons) > 0 {
		valueString, err := getModifiedReasonMessageAttributeValue(modifiedReasons)
		if err != nil {
			return 0, err
		}
		return len("String.Array") + len(modifiedMessageAttributeKey) + len(valueString), nil
	}
	return 0, nil
}

func getModifiedReasonMessageAttributeValue(modifiedReasons []string) (string, error) {
	jsonString, err := jsonMarshal(modifiedReasons)
	if err != nil {
		return "", err
	}

	return string(jsonString), nil
}

func isValidateMessageAttribute(messageAttributeKey string, messageAttributeValue string) bool {
	if len(messageAttributeKey) == 0 || len(messageAttributeValue) == 0 {
		return false
	}

	if !isValidMessageAttributeKeyCharacters(messageAttributeKey) ||
		isInvalidMessageAttributeKeyPrefix(messageAttributeKey) ||
		isInvalidMessageAttributeKeySuffix(messageAttributeKey) ||
		isInvalidMessageAttributeKeySubstring(messageAttributeKey) {
		return false
	}

	if utf8.RuneCountInString(messageAttributeKey) > messageAttributeKeyLimitInCharacters {
		return false
	}

	if !utf8.ValidString(messageAttributeValue) {
		return false
	}

	return true
}
