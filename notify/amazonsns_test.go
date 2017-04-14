// Copyright 2015 Prometheus Team
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
package notify

import (
	"errors"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
)

func dummySNSSetup(t *testing.T) (context.Context, []*types.Alert, *AmazonSNS) {
	ctx := WithReceiverName(context.Background(), "name")
	lset := model.LabelSet{
		"group_label_key": "group_label_value",
	}
	ctx = WithGroupLabels(ctx, lset)

	alerts := []*types.Alert{{}, {}}

	c := &config.AmazonSNSConfig{
		TopicARN: "arn:aws:sns:no-region:1234567890:topic",
		Subject:  `{{ template "amazon_sns.default.subject" . }}`,
		Message:  `{{ template "amazon_sns.default.message" . }}`,
	}
	tmpl, err := template.FromGlobs()
	require.NoError(t, err, "Failed template setup")
	tmpl.ExternalURL, err = url.Parse("http://localhost/")
	require.NoError(t, err, "Failed template URL setup")

	n := NewAmazonSNS(c, tmpl)

	return ctx, alerts, n
}

func TestAmazonSNSIntegration(t *testing.T) {
	ctx, alerts, n := dummySNSSetup(t)

	n.testPublisher = func(c *aws.Config, pi *sns.PublishInput) error {
		require.Contains(t, aws.StringValue(c.Region), "no-region", "AWS Config not set to correct region")
		subj := aws.StringValue(pi.Subject)
		require.Contains(t, subj, "[FIRING:2]", "Default Subject missing alerts raised summary")
		require.Contains(t, subj, "group_label_value", "Default Subject missing Context label")
		mess := aws.StringValue(pi.Message)
		require.Contains(t, mess, "Source", "Default Message missing structure")
		return nil
	}

	retry, err := n.Notify(ctx, alerts...)
	require.NoError(t, err, "Happy path")
	require.Equal(t, false, retry, "Happy path no need to retry")
}

func TestAmazonSNSBadARN(t *testing.T) {
	ctx, alerts, n := dummySNSSetup(t)

	n.conf.TopicARN = "fdsfds"
	n.testPublisher = func(c *aws.Config, pi *sns.PublishInput) error {
		t.Fatal("Shouldn't attempt to publish to a bad TopicARN")
		return nil
	}

	retry, err := n.Notify(ctx, alerts...)
	require.Error(t, err, "Bad ARN should error")
	require.Contains(t, err.Error(), "fdsfds", "Incorrect bad ARN message")
	require.Equal(t, false, retry, "Bad ARN shouldn't retry")
}

func TestAmazonSNSRegionOverride(t *testing.T) {
	ctx, alerts, n := dummySNSSetup(t)

	n.conf.AWSRegion = "somewhere"
	n.testPublisher = func(c *aws.Config, pi *sns.PublishInput) error {
		require.Contains(t, aws.StringValue(c.Region), "somewhere", "Failed to override AWS Region")
		return nil
	}

	retry, err := n.Notify(ctx, alerts...)
	require.NoError(t, err, "SNS Override should work")
	require.Equal(t, false, retry, "SNS Override no need to retry")
}

func TestAmazonSNSPublishFailRetry(t *testing.T) {
	ctx, alerts, n := dummySNSSetup(t)

	n.testPublisher = func(c *aws.Config, pi *sns.PublishInput) error {
		return errors.New("Dummy failure")
	}

	retry, err := n.Notify(ctx, alerts...)
	require.Error(t, err, "Publish error should propagate")
	require.Contains(t, err.Error(), "Dummy failure", "Need to see the error message from SNS publish upon failure")
	require.Equal(t, true, retry, "Should retry on publish failure")
}
