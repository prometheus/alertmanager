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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for SNS notifications.
type Notifier struct {
	conf    *config.SNSConfig
	tmpl    *template.Template
	logger  *slog.Logger
	client  *http.Client
	retrier *notify.Retrier
}

// New returns a new SNS notification handler.
func New(c *config.SNSConfig, t *template.Template, l *slog.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {
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
		tmplErr error
		data    = notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
		tmpl    = notify.TmplText(n.tmpl, data, &tmplErr)
	)

	client, err := n.createSNSClient(ctx, tmpl, &tmplErr)
	if err != nil {
		// V2 error handling is different. We don't have awserr.RequestFailure.
		// We can check for a generic smithy.APIError to see if it's a service error.
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			// To maintain compatibility with the retrier, we attempt to get an HTTP status code.
			var respErr *smithyhttp.ResponseError
			if errors.As(err, &respErr) && respErr.Response != nil {
				return n.retrier.Check(respErr.Response.StatusCode, strings.NewReader(apiErr.ErrorMessage()))
			}
			// Fallback if we can't get a status code.
			return true, fmt.Errorf("failed to create SNS client: %s: %s", apiErr.ErrorCode(), apiErr.ErrorMessage())
		}
		return true, err
	}

	publishInput, err := n.createPublishInput(ctx, tmpl, &tmplErr)
	if err != nil {
		return true, err
	}

	publishOutput, err := client.Publish(ctx, publishInput)
	if err != nil {
		// V2 error handling uses errors.As to inspect the error chain.
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			var statusCode int
			var respErr *smithyhttp.ResponseError
			// Try to extract the HTTP status code for the retrier.
			if errors.As(err, &respErr) && respErr.Response != nil {
				statusCode = respErr.Response.StatusCode
			}

			// If we got a status code, use the retrier logic.
			if statusCode != 0 {
				retryable, checkErr := n.retrier.Check(statusCode, strings.NewReader(apiErr.ErrorMessage()))
				reasonErr := notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(statusCode), checkErr)
				return retryable, reasonErr
			}
		}
		// Fallback for non-API errors or if status code extraction fails.
		return true, err
	}

	n.logger.Debug("SNS message successfully published", "message_id", aws.ToString(publishOutput.MessageId), "sequence_number", aws.ToString(publishOutput.SequenceNumber))

	return false, nil
}

func (n *Notifier) createSNSClient(ctx context.Context, tmpl func(string) string, tmplErr *error) (*sns.Client, error) {
	// Base configuration options that apply to both STS (if used) and the final SNS client.
	baseCfgOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithHTTPClient(n.client),
		awsconfig.WithRegion(n.conf.Sigv4.Region),
	}
	if n.conf.Sigv4.Profile != "" {
		baseCfgOpts = append(baseCfgOpts, awsconfig.WithSharedConfigProfile(n.conf.Sigv4.Profile))
	}
	if n.conf.Sigv4.AccessKey != "" {
		creds := credentials.NewStaticCredentialsProvider(n.conf.Sigv4.AccessKey, string(n.conf.Sigv4.SecretKey), "")
		baseCfgOpts = append(baseCfgOpts, awsconfig.WithCredentialsProvider(creds))
	}

	// Final configuration options for the SNS client.
	snsCfgOpts := baseCfgOpts

	// If a RoleARN is provided, create an STS client to assume the role.
	// This uses a separate config load to ensure the STS client does not use a custom SNS endpoint.
	if n.conf.Sigv4.RoleARN != "" {
		stsCfg, err := awsconfig.LoadDefaultConfig(ctx, baseCfgOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to load base config for STS: %w", err)
		}
		stsClient := sts.NewFromConfig(stsCfg)
		stsProvider := stscreds.NewAssumeRoleProvider(stsClient, n.conf.Sigv4.RoleARN)
		// Add the AssumeRole provider to the options for the SNS client config.
		snsCfgOpts = append(snsCfgOpts, awsconfig.WithCredentialsProvider(aws.NewCredentialsCache(stsProvider)))
	}

	// Resolve the API URL from the template.
	apiURL := tmpl(n.conf.APIUrl)
	if *tmplErr != nil {
		return nil, notify.NewErrorWithReason(notify.ClientErrorReason, fmt.Errorf("execute 'api_url' template: %w", *tmplErr))
	}
	if apiURL != "" {
		snsCfgOpts = append(snsCfgOpts, awsconfig.WithBaseEndpoint(apiURL))
	}

	// Load the final configuration for the SNS client.
	snsCfg, err := awsconfig.LoadDefaultConfig(ctx, snsCfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load final config for SNS: %w", err)
	}

	// We will always need a region to be set.
	if snsCfg.Region == "" {
		return nil, fmt.Errorf("region not configured in sns.sigv4.region or in default credentials chain")
	}

	return sns.NewFromConfig(snsCfg), nil
}

func (n *Notifier) createPublishInput(ctx context.Context, tmpl func(string) string, tmplErr *error) (*sns.PublishInput, error) {
	publishInput := &sns.PublishInput{}
	messageAttributes := n.createMessageAttributes(tmpl)
	if *tmplErr != nil {
		return nil, notify.NewErrorWithReason(notify.ClientErrorReason, fmt.Errorf("execute 'attributes' template: %w", *tmplErr))
	}

	// Max message size for a message in an SNS publish request is 256KB,
	// except for SMS messages where the limit is 1600 characters/runes.
	messageSizeLimit := 256 * 1024
	if n.conf.TopicARN != "" {
		topicARN := tmpl(n.conf.TopicARN)
		if *tmplErr != nil {
			return nil, notify.NewErrorWithReason(notify.ClientErrorReason, fmt.Errorf("execute 'topic_arn' template: %w", *tmplErr))
		}
		publishInput.TopicArn = aws.String(topicARN)
		// If we are using a topic ARN, it could be a FIFO topic specified by the topic's suffix ".fifo".
		if strings.HasSuffix(topicARN, ".fifo") {
			key, err := notify.ExtractGroupKey(ctx)
			if err != nil {
				return nil, err
			}
			publishInput.MessageDeduplicationId = aws.String(key.Hash())
			publishInput.MessageGroupId = aws.String(key.Hash())
		}
	}
	if n.conf.PhoneNumber != "" {
		publishInput.PhoneNumber = aws.String(tmpl(n.conf.PhoneNumber))
		if *tmplErr != nil {
			return nil, notify.NewErrorWithReason(notify.ClientErrorReason, fmt.Errorf("execute 'phone_number' template: %w", *tmplErr))
		}
		// If we have an SMS message, we need to truncate to 1600 characters/runes.
		messageSizeLimit = 1600
	}
	if n.conf.TargetARN != "" {
		publishInput.TargetArn = aws.String(tmpl(n.conf.TargetARN))
		if *tmplErr != nil {
			return nil, notify.NewErrorWithReason(notify.ClientErrorReason, fmt.Errorf("execute 'target_arn' template: %w", *tmplErr))
		}
	}

	tmplMessage := tmpl(n.conf.Message)
	if *tmplErr != nil {
		return nil, notify.NewErrorWithReason(notify.ClientErrorReason, fmt.Errorf("execute 'message' template: %w", *tmplErr))
	}
	messageToSend, isTrunc, err := validateAndTruncateMessage(tmplMessage, messageSizeLimit)
	if err != nil {
		return nil, err
	}
	if isTrunc {
		// If we truncated the message we need to add a message attribute showing that it was truncated.
		messageAttributes["truncated"] = snstypes.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String("true")}
	}

	publishInput.Message = aws.String(messageToSend)
	publishInput.MessageAttributes = messageAttributes

	if n.conf.Subject != "" {
		publishInput.Subject = aws.String(tmpl(n.conf.Subject))
		if *tmplErr != nil {
			return nil, notify.NewErrorWithReason(notify.ClientErrorReason, fmt.Errorf("execute 'subject' template: %w", *tmplErr))
		}
	}

	return publishInput, nil
}

func validateAndTruncateMessage(message string, maxMessageSizeInBytes int) (string, bool, error) {
	if !utf8.ValidString(message) {
		return "", false, fmt.Errorf("non utf8 encoded message string")
	}
	if len(message) <= maxMessageSizeInBytes {
		return message, false, nil
	}
	// If the message is larger than our specified size we have to truncate.
	truncated := make([]byte, maxMessageSizeInBytes)
	copy(truncated, message)
	return string(truncated), true, nil
}

func (n *Notifier) createMessageAttributes(tmpl func(string) string) map[string]snstypes.MessageAttributeValue {
	// Convert the given attributes map into the AWS Message Attributes Format.
	attributes := make(map[string]snstypes.MessageAttributeValue, len(n.conf.Attributes))
	for k, v := range n.conf.Attributes {
		attributes[tmpl(k)] = snstypes.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(tmpl(v))}
	}
	return attributes
}
