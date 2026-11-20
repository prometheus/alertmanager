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

// Package kafka provides notifications to Apache Kafka.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	sharedkafka "github.com/prometheus/alertmanager/kafka"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/webhook"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

type producer interface {
	ProduceSync(context.Context, ...*kgo.Record) kgo.ProduceResults
	Close()
}

// Notifier implements a notifier that produces grouped alerts to Kafka.
type Notifier struct {
	conf     *Config
	tmpl     *template.Template
	logger   *slog.Logger
	producer producer
}

// New returns a new Kafka notifier.
func New(conf *Config, tmpl *template.Template, logger *slog.Logger) (*Notifier, error) {
	if err := conf.validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	opts, err := sharedkafka.BuildOpts(conf.clientOptions(), logger)
	if err != nil {
		return nil, err
	}
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka: creating producer: %w", err)
	}
	sharedkafka.PingInBackground(client, logger)
	return &Notifier{
		conf:     conf,
		tmpl:     tmpl,
		logger:   logger,
		producer: client,
	}, nil
}

// Notify implements the notify.Notifier interface.
func (n *Notifier) Notify(ctx context.Context, alerts ...*types.Alert) (bool, error) {
	groupKey, err := notify.ExtractGroupKey(ctx)
	if err != nil {
		return false, err
	}
	msg := webhook.Message{
		Version:  "4",
		Data:     notify.GetTemplateData(ctx, n.tmpl, alerts, n.logger),
		GroupKey: groupKey.String(),
	}
	payload, err := json.Marshal(&msg)
	if err != nil {
		return false, fmt.Errorf("kafka: encoding notification: %w", err)
	}
	results := n.producer.ProduceSync(ctx, &kgo.Record{
		Topic: n.conf.Topic,
		Key:   []byte(groupKey.String()),
		Value: payload,
	})
	if err := results.FirstErr(); err != nil {
		return true, fmt.Errorf("kafka: producing notification: %w", err)
	}
	return false, nil
}

// Close releases the Kafka producer resources.
func (n *Notifier) Close() error {
	n.producer.Close()
	return nil
}
