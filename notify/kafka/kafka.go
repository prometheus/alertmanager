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
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"

	ckafka "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
)

// Notifier implements a Notifier for Discord notifications.
type Notifier struct {
	conf                *config.KafkaConfig
	logger              *slog.Logger
	writer              *ckafka.Writer
	partitionIndex      int
	partitionIndexMutex sync.Mutex
}

// KafkaMessage is the message sent to Kafka.
type KafkaMessage struct {
	Alerts []*types.Alert `json:"alerts"`
}

// New returns a new Kafka notifier.
func New(c *config.KafkaConfig, l *slog.Logger) (*Notifier, error) {
	mechanism := plain.Mechanism{
		Username: c.Username,
		Password: c.Password,
	}

	transport := ckafka.Transport{
		SASL:        mechanism,
		DialTimeout: 45 * time.Second,
		TLS:         &tls.Config{},
	}

	writer := &ckafka.Writer{
		Addr:         ckafka.TCP(c.Brokers...),
		Topic:        c.Topic,
		Balancer:     &ckafka.LeastBytes{},
		RequiredAcks: ckafka.RequireAll,
		Transport:    &transport,
	}

	n := &Notifier{
		conf:   c,
		logger: l,
		writer: writer,
	}

	return n, nil
}

// GetPartitionIndex returns the current partition index.
func (n *Notifier) GetPartitionIndex() int {
	return n.partitionIndex
}

// NextPartition returns the next partition index.
func (n *Notifier) NextPartition() {
	n.partitionIndexMutex.Lock()
	n.partitionIndex = (n.partitionIndex + 1) % n.conf.NumberOfPartition
	n.partitionIndexMutex.Unlock()
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var buf bytes.Buffer
	message := KafkaMessage{Alerts: as}

	if err := json.NewEncoder(&buf).Encode(message); err != nil {
		slog.Log(ctx, slog.LevelError, "Failed to encode alert", "err", err)
		return false, err
	}

	if err := n.Produce(ctx, n.conf.Topic, "", buf.Bytes()); err != nil {
		slog.Log(ctx, slog.LevelError, "Failed to produce alert", "err", err)
		return false, err
	}

	return false, nil
}

// Produce sends a message to Kafka.
func (n *Notifier) Produce(ctx context.Context, topic, key string, value []byte) error {
	return n.writer.WriteMessages(ctx,
		ckafka.Message{
			Key:       []byte(key),
			Value:     value,
			Partition: n.GetPartitionIndex(),
		},
	)
}
