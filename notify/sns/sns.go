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
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
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
	isFifo  *bool
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

func (n *Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	var (
		err   error
		data                           = notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)
		tmpl                           = notify.TmplText(n.tmpl, data, &err)
		creds *credentials.Credentials = nil
	)
	if n.conf.Sigv4.AccessKey != "" && n.conf.Sigv4.SecretKey != "" {
		creds = credentials.NewStaticCredentials(n.conf.Sigv4.AccessKey, string(n.conf.Sigv4.SecretKey), "")
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:      aws.String(n.conf.Sigv4.Region),
			Credentials: creds,
			Endpoint:    aws.String(tmpl(n.conf.APIUrl)),
		},
		Profile: n.conf.Sigv4.Profile,
	})

	if _, err := sess.Config.Credentials.Get(); err != nil {
		return false, fmt.Errorf("could not get SigV4 credentials: %w", err)
	}

	if n.conf.Sigv4.RoleARN != "" {
		sess.Config.Credentials = stscreds.NewCredentials(sess, n.conf.Sigv4.RoleARN)
	}

	client := sns.New(sess, &aws.Config{Credentials: creds})
	publishInput := &sns.PublishInput{}

	if n.conf.TopicARN != "" {
		publishInput.SetTopicArn(tmpl(n.conf.TopicARN))
		messageToSend, isTrunc, err := validateAndTruncateMessage(tmpl(n.conf.Message))
		if err != nil {
			return false, err
		}
		if isTrunc {
			n.conf.Attributes["truncated"] = "true"
		}

		if n.isFifo == nil {
			checkFifo, err := checkTopicFifoAttribute(client, n.conf.TopicARN)
			if err != nil {
				if e, ok := err.(awserr.RequestFailure); ok {
					return n.retrier.Check(e.StatusCode(), strings.NewReader(e.Message()))
				} else {
					return true, err
				}
			}
			n.isFifo = &checkFifo
		}
		if *n.isFifo {
			// Deduplication key and Message Group ID are only added if it's a FIFO SNS Topic.
			key, err := notify.ExtractGroupKey(ctx)
			if err != nil {
				return false, err
			}
			publishInput.SetMessageDeduplicationId(key.Hash())
			publishInput.SetMessageGroupId(key.Hash())
		}

		publishInput.SetMessage(messageToSend)
	}
	if n.conf.PhoneNumber != "" {
		publishInput.SetPhoneNumber(tmpl(n.conf.PhoneNumber))
		// If SMS message is over 1600 chars, SNS will reject the message.
		_, isTruncated := notify.Truncate(tmpl(n.conf.Message), 1600)
		if isTruncated {
			return false, fmt.Errorf("SMS message exeeds length of 1600 charactors")
		} else {
			publishInput.SetMessage(tmpl(n.conf.Message))
		}
	}
	if n.conf.TargetARN != "" {
		publishInput.SetTargetArn(tmpl(n.conf.TargetARN))
		messageToSend, isTrunc, err := validateAndTruncateMessage(tmpl(n.conf.Message))
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
			attributes[tmpl(k)] = &sns.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(tmpl(v))}
		}
		publishInput.SetMessageAttributes(attributes)
	}

	if n.conf.Subject != "" {
		publishInput.SetSubject(tmpl(n.conf.Subject))
	}

	publishOutput, err := client.Publish(publishInput)
	if err != nil {
		if e, ok := err.(awserr.RequestFailure); ok {
			return n.retrier.Check(e.StatusCode(), strings.NewReader(e.Message()))
		} else {
			return true, err
		}
	}

	err = n.logger.Log(publishOutput.String())
	if err != nil {
		return false, err
	}

	return false, nil
}

func checkTopicFifoAttribute(client *sns.SNS, topicARN string) (bool, error) {
	topicAttributes, err := client.GetTopicAttributes(&sns.GetTopicAttributesInput{TopicArn: aws.String(topicARN)})
	if err != nil {
		return false, err
	}
	ta := topicAttributes.Attributes["FifoTopic"]
	if ta != nil && *ta == "true" {
		return true, nil
	}
	return false, nil
}

func validateAndTruncateMessage(message string) (string, bool, error) {
	if utf8.ValidString(message) {
		// if the message is larger than 256KB we have to truncate.
		if len(message) > 256*1024 {
			truncated := make([]byte, 256*1024)
			copy(truncated, message)
			return string(truncated), true, nil
		}
		return message, false, nil
	}
	return "", false, fmt.Errorf("non utf8 encoded message string")
}
