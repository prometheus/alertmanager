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

package pubsub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"golang.org/x/oauth2"
	auth "golang.org/x/oauth2/google"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Notifier implements a Notifier for generic Google Pubsub.
type Notifier struct {
	conf      *config.GooglePubsubConfig
	tmpl      *template.Template
	logger    log.Logger
	client    *http.Client
	retrier   *notify.Retrier
	targetURL string
}

// New returns a new Google Pubsub receiver.
// This receiver is used to send alerts to Google Pubsub publish endpoints.
// We are using the Google Pubsub REST API to publish messages to a topic.
// Authentication is done using a GCP service account file and google oauth2 library.
// API Reference: https://cloud.google.com/pubsub/docs/reference/rest/v1/projects.topics/publish
func New(conf *config.GooglePubsubConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	// validate config required fields
	configValidationError := ValidateConfig(conf)
	if configValidationError != nil {
		return nil, configValidationError
	}

	// log the service account file path
	serviceAccountFilePath := conf.Authorization.ServiceAccountFile
	_ = level.Info(l).Log("msg", "Google Pubsub ServiceAccountFile", "ServiceAccountFile", serviceAccountFilePath)

	// read the json file contents to byte[]
	serviceAccountBytes, err := os.ReadFile(serviceAccountFilePath)
	if err != nil {
		_ = level.Error(l).Log("msg", "failed to read GCP service account file", "error", err)
		return nil, err
	}

	// required scope for the Pubsub publish endpoint
	// see: https://cloud.google.com/pubsub/docs/reference/rest/v1/projects.topics/publish
	scope := "https://www.googleapis.com/auth/pubsub"
	tokenSource, err := auth.JWTAccessTokenSourceWithScope(serviceAccountBytes, scope)
	if err != nil || tokenSource == nil {
		// log
		_ = level.Error(l).Log("msg", "failed to generate GCP token source from service account file", "error", err)
		return nil, err
	}

	// formatting the template URL with Project and Topic variable to create the targetURL
	templateURL := "https://pubsub.googleapis.com/v1/projects/{project}/topics/{topic}:publish"
	if conf.TemplateURL != nil && strings.TrimSpace(conf.TemplateURL.String()) != "" {
		templateURL = conf.TemplateURL.String()
	}
	// replace the template variables with the actual values
	targetURL := strings.ReplaceAll(templateURL, "{project}", conf.Project)
	targetURL = strings.ReplaceAll(targetURL, "{topic}", conf.Topic)
	// log url after replacement
	_ = level.Info(l).Log("msg", "Google Pubsub endpoint URL", "url", targetURL)

	ctx := context.Background()
	// create the http client with oauth2 token source
	client := oauth2.NewClient(ctx, tokenSource)
	return &Notifier{
		conf:      conf,
		tmpl:      t,
		logger:    l,
		targetURL: targetURL,
		client:    client,
		// Webhooks are assumed to respond with 2xx response codes on a successful
		// request and 5xx response codes are assumed to be recoverable.
		retrier: &notify.Retrier{
			CustomDetailsFunc: func(_ int, body io.Reader) string {
				return errDetails(body, targetURL)
			},
		},
	}, nil
}

// errDetails print formatted error details.
func errDetails(body io.Reader, url string) string {
	if body == nil {
		return url
	}
	bs, err := io.ReadAll(body)
	if err != nil {
		return url
	}
	return fmt.Sprintf("%s: %s", url, string(bs))
}

// ValidateConfig validates the Google Pubsub configuration.
func ValidateConfig(conf *config.GooglePubsubConfig) error {
	if strings.TrimSpace(conf.Project) == "" {
		return fmt.Errorf("google Pubsub Project is empty")
	}
	if strings.TrimSpace(conf.Topic) == "" {
		return fmt.Errorf("google Pubsub Topic is empty")
	}
	// validate Authorization.ServiceAccountFile is not empty
	if conf.Authorization == nil || strings.TrimSpace(conf.Authorization.ServiceAccountFile) == "" {
		return fmt.Errorf("google Pubsub Service Account is empty")
	}
	return nil
}

// Message defines the JSON object (the inner data [the Alert manager group event] of each message) send to Google Pubsub publish endpoints.
type Message struct {
	*template.Data

	// The protocol version.
	Version         string `json:"version"`
	GroupKey        string `json:"groupKey"`
	TruncatedAlerts uint64 `json:"truncatedAlerts"`
}

// PubsubMessage defines the JSON object (a single Pubsub message, data and message attributes) send to Google Pubsub publish endpoints.
type PubsubMessage struct {
	Data        []byte            `json:"data"`
	Attributes  map[string]string `json:"attributes"`
	OrderingKey string            `json:"orderingKey"`
}

// PubsubMessages defines the JSON object (multiple pubsub messages container) send to Google Pubsub publish endpoints.
type PubsubMessages struct {
	Messages []PubsubMessage `json:"messages"`
}

// truncateAlerts truncates the alerts to the maximum number of alerts.
func truncateAlerts(maxAlerts uint64, alerts []*types.Alert) ([]*types.Alert, uint64) {
	if maxAlerts != 0 && uint64(len(alerts)) > maxAlerts {
		return alerts[:maxAlerts], uint64(len(alerts)) - maxAlerts
	}

	return alerts, 0
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	alerts, numTruncated := truncateAlerts(n.conf.MaxAlerts, alerts)
	data := notify.GetTemplateData(ctx, n.tmpl, alerts, n.logger)

	level.Debug(n.logger).Log("notify", fmt.Sprintf("sending %d alerts to Google Pubsub", len(alerts)))

	groupKey, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		level.Error(n.logger).Log("err", err)
	}

	level.Debug(n.logger).Log("groupKey", groupKey.String())

	// create the inner message data JSON
	msg := &Message{
		Version:         "4",
		Data:            data,
		GroupKey:        groupKey.String(),
		TruncatedAlerts: numTruncated,
	}
	var msgBuf bytes.Buffer
	if err := json.NewEncoder(&msgBuf).Encode(msg); err != nil {
		_ = level.Error(n.logger).Log("msg", "failed to encode message", "error", err)
		return false, err
	}

	// create the outer pubsub message JSON
	pubsubMessages := PubsubMessages{
		Messages: make([]PubsubMessage, 1),
	}
	pubsubMessages.Messages[0].Data = msgBuf.Bytes()
	pubsubMessages.Messages[0].Attributes = make(map[string]string)
	// add all attributes to the pubsub message
	pubsubMessages.Messages[0].Attributes["key"] = groupKey.String()
	for k, v := range data.GroupLabels {
		pubsubMessages.Messages[0].Attributes[k] = v
	}
	for k, v := range data.CommonLabels {
		pubsubMessages.Messages[0].Attributes[k] = v
	}
	for k, v := range data.CommonAnnotations {
		pubsubMessages.Messages[0].Attributes[k] = v
	}
	pubsubMessages.Messages[0].OrderingKey = groupKey.String()

	// encode the pubsub message
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(pubsubMessages); err != nil {
		_ = level.Error(n.logger).Log("msg", "failed to encode pubsub message", "error", err)
		return false, err
	}

	resp, err := notify.PostJSON(ctx, n.client, n.targetURL, &buf)
	if err != nil {
		_ = level.Error(n.logger).Log("msg", "failed to send message", "error", err)
		return true, notify.RedactURL(err)
	}
	level.Debug(n.logger).Log("msg", "successfully sent message", "status", resp.StatusCode)
	defer notify.Drain(resp)

	// Check if the request should be retried.
	shouldRetry, err := n.retrier.Check(resp.StatusCode, resp.Body)
	if err != nil {
		_ = level.Error(n.logger).Log("msg", "failed to send message", "error", err)
		return shouldRetry, notify.NewErrorWithReason(notify.GetFailureReasonFromStatusCode(resp.StatusCode), err)
	}
	return shouldRetry, err
}
