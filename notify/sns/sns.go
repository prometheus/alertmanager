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
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	commoncfg "github.com/prometheus/common/config"
)

// Notifier implements a Notifier for SNS notifications.
type Notifier struct {
	conf    *config.SNSConfig
	tmpl    *template.Template
	logger  log.Logger
	client  *http.Client
	retrier *notify.Retrier
}

func (n Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	credentials := credentials.NewStaticCredentials(n.conf.Sigv4.AccessKey, string(n.conf.Sigv4.SecretKey), "")
	if n.conf.Sigv4.AccessKey == "" {
		credentials = nil
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:      aws.String(n.conf.Sigv4.Region),
			Credentials: credentials,
			Endpoint:    aws.String(n.conf.APIUrl),
		},
		Profile: n.conf.Sigv4.Profile,
	})

	if _, err := sess.Config.Credentials.Get(); err != nil {
		return false, fmt.Errorf("could not get SigV4 credentials: %w", err)
	}

	if n.conf.Sigv4.RoleARN != "" {
		sess.Config.Credentials = stscreds.NewCredentials(sess, n.conf.Sigv4.RoleARN)
	}

	data := notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
	tmpl := notify.TmplText(n.tmpl, data, &err)
	message := tmpl(n.conf.Message)

	client := sns.New(sess, &aws.Config{Credentials: credentials})
	publishInput := &sns.PublishInput{}

	if n.conf.TopicARN != "" {
		publishInput.SetTopicArn(n.conf.TopicARN)
		messageToSend, isTrunc, err := validateAndTruncateMessage(message)
		if err != nil {
			return false, err
		}
		if isTrunc {
			n.conf.Attributes["truncated"] = "true"
		}
		publishInput.SetMessage(messageToSend)
	}
	if n.conf.PhoneNumber != "" {
		publishInput.SetPhoneNumber(n.conf.PhoneNumber)
		// If SMS message is over 1600 chars, SNS will reject the message.
		_, isTruncated := notify.Truncate(message, 1600)
		if isTruncated {
			return false, fmt.Errorf("SMS message exeeds length of 1600 charactors")
		} else {
			publishInput.SetMessage(message)
		}
	}
	if n.conf.TargetARN != "" {
		publishInput.SetTargetArn(n.conf.TargetARN)
		messageToSend, isTrunc, err := validateAndTruncateMessage(message)
		if err != nil {
			return false, err
		}
		if isTrunc {
			n.conf.Attributes["truncated"] = "true"
		}
		publishInput.SetMessage(messageToSend)
	}

	if len(n.conf.Attributes) > 0 {
		attributes := map[string]*sns.MessageAttributeValue{}
		for k, v := range n.conf.Attributes {
			attributes[k] = &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(v)}
		}
		publishInput.SetMessageAttributes(attributes)
	}

	if n.conf.Subject != "" {
		publishInput.SetSubject(n.conf.Subject)
	}

	// Deduplication key is only added if it's a FIFO SNS Topic.
	if n.conf.IsFIFOTopic {
		key, err := notify.ExtractGroupKey(ctx)
		if err != nil {
			return false, err
		}
		publishInput.SetMessageDeduplicationId(key.Hash())
	}

	publishOutput, err := client.Publish(publishInput)
	if err != nil {
		// AWS Response is bad, probably a config issue.
		return false, err
	}

	err = n.logger.Log(publishOutput.String())
	if err != nil {
		return false, err
	}

	// Response is good and does not need to be retried.
	return false, nil
}

func validateAndTruncateMessage(message string) (string, bool, error) {
	if utf8.ValidString(message) {
		// if the message is larger than 256KB we have to truncate.
		if len(message) > 256*1024 {
			truncated := make([]byte, 256*1024, 256*1024)
			copy(truncated, message)
			return string(truncated), true, nil
		}
		return message, false, nil
	}
	return "", false, fmt.Errorf("non utf8 encoded message string")
}

// New returns a new SNS notification handler.
func New(c *config.SNSConfig, t *template.Template, l log.Logger, httpOpts ...commoncfg.HTTPClientOption) (*Notifier, error) {

	client, err := commoncfg.NewClientFromConfig(*c.HTTPConfig, "sns", append(httpOpts, commoncfg.WithHTTP2Disabled())...)
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
