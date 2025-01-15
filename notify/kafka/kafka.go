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
	"fmt"
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
	numberOfPartition   int
	partitionIndex      int
	partitionIndexMutex sync.Mutex
}

// New returns a new Kafka notifier.
func New(c *config.KafkaConfig, l *slog.Logger) (*Notifier, error) {
	transport := ckafka.Transport{}

	if c.SecurityProtocol != nil {
		transport.TLS = &tls.Config{}

		if *c.SecurityProtocol == "SASL_SSL" {
			// default is PLAIN mechanism
			transport.SASL = plain.Mechanism{
				Username: *c.Username,
				Password: *c.Password,
			}
		}
	}

	writer := &ckafka.Writer{
		Addr:         ckafka.TCP(c.Brokers...),
		Topic:        c.Topic,
		Balancer:     &ckafka.LeastBytes{},
		RequiredAcks: ckafka.RequireAll,
		Transport:    &transport,
	}

	if c.Timeout != nil {
		writer.WriteTimeout = *c.Timeout
	} else {
		writer.WriteTimeout = 45 * time.Second
	}

	n := &Notifier{
		conf:   c,
		logger: l,
		writer: writer,
	}

	if c.NumberOfPartition != nil {
		n.numberOfPartition = *c.NumberOfPartition
	} else {
		n.numberOfPartition = 1
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
	n.partitionIndex = (n.partitionIndex + 1) % n.numberOfPartition
	n.partitionIndexMutex.Unlock()
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	// Because retry is supported by kafka-go so it will be always false
	var buf bytes.Buffer
	shouldRetry := false

	for _, alert := range as {
		if err := json.NewEncoder(&buf).Encode(alert); err != nil {
			slog.Log(ctx, slog.LevelError, fmt.Sprintf("Failed to marshal alert: %s", alert.Name()), "err", err)
		}

		if err := n.Produce(ctx, alert.Name(), buf.Bytes()); err != nil {
			slog.Log(ctx, slog.LevelError, fmt.Sprintf("Failed to produce alert: %s", alert.Name()), "err", err)
		}
	}

	return shouldRetry, nil
}

// Produce sends a message to Kafka.
func (n *Notifier) Produce(ctx context.Context, key string, value []byte) error {
	message := ckafka.Message{
		Key:   []byte(key),
		Value: value,
	}

	if n.conf.NumberOfPartition != nil {
		message.Partition = n.GetPartitionIndex()
	}

	return n.writer.WriteMessages(ctx, message)
}
