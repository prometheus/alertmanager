// Copyright 2025 Prometheus Team
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

package kafka

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

var nextPartition = 0

// Notifier implements a Notifier for Discord notifications.
type Notifier struct {
	conf       *config.KafkaConfig
	tmpl       *template.Template
	logger     *slog.Logger
	retrier    *notify.Retrier
	producer   *ckafka.Producer
}

type KafkaMessage struct {
	Alerts []*types.Alert `json:"alerts"`
}

// New returns a new Kafka notifier.
func New(c *config.KafkaConfig, l *slog.Logger) (*Notifier, error) {

	kafkaConfig := ckafka.ConfigMap{
		"bootstrap.servers": c.BootstrapServers,
	}

	if c.ExtrasConfigs != nil {
		for k, v := range *c.ExtrasConfigs {
			kafkaConfig.SetKey(k, v)
		}
	}

	p, err := ckafka.NewProducer(&kafkaConfig)

	if err != nil {
		return nil, err
	}

	slog.Log(context.Background(), slog.LevelInfo, "Connected to Kafka")

	n := &Notifier{
		conf:   c,
		logger: l,
		producer: p,
	}

	return n, nil
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	n.logger.Info("Sending alert to Kafka")
	var buf bytes.Buffer
	message := KafkaMessage{ Alerts: as }

	if err := json.NewEncoder(&buf).Encode(message); err != nil {
		slog.Log(ctx, slog.LevelError, "Failed to encode alert", "err", err)
		return false, err
	}

	if err := n.Produce(ctx, n.conf.Topic, "", buf.Bytes()); err != nil {
		slog.Log(ctx, slog.LevelError, "Failed to produce alert", "err", err)
		return false, err
	}

	if unflushed := n.producer.Flush(1000); unflushed == 0 {
		nextPartition++
		if nextPartition == n.conf.NumberOfPartition {
			nextPartition = 0
		}

		n.logger.Info("Successfully produced alert")
		return false, nil
	}

	return false, nil
}

func (n *Notifier) Produce(ctx context.Context, topic string, key string, value []byte) error {
	return n.producer.Produce(&ckafka.Message{
		TopicPartition: ckafka.TopicPartition{Topic: &topic, Partition: int32(nextPartition)},
		Key:            []byte(key),
		Value:          value,
	}, nil);
}

