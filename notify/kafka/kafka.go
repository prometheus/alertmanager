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
	conf           *config.KafkaConfig
	logger         *slog.Logger
	partition      int
	partitionMutex sync.Mutex
	sendFunc       func(ctx context.Context, msgs ...ckafka.Message) error
}

// New returns a new Kafka notifier.
func New(c *config.KafkaConfig, l *slog.Logger, sendFunc *func(ctx context.Context, msgs ...ckafka.Message) error) (*Notifier, error) {
	n := &Notifier{
		conf:   c,
		logger: l,
	}

	if sendFunc != nil {
		n.sendFunc = *sendFunc
		return n, nil
	}

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

	if c.NumberOfPartition > 0 {
		n.partition = c.NumberOfPartition
	} else {
		n.partition = 1
	}

	n.sendFunc = func(ctx context.Context, msgs ...ckafka.Message) error {
		return writer.WriteMessages(ctx, msgs...)
	}

	return n, nil
}

// GetPartitionIndex returns the current partition index.
func (n *Notifier) GetPartitionIndex() int {
	return n.partition
}

// NextPartition returns the next partition index.
func (n *Notifier) NextPartition() {
	n.partitionMutex.Lock()
	n.partition = (n.partition + 1) % n.conf.NumberOfPartition
	n.partitionMutex.Unlock()
}

// Notify implements the Notifier interface.
func (n *Notifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	var buf bytes.Buffer
	var msgs []ckafka.Message
	// Because retry is supported by kafka-go so it will be always false
	shouldRetry := false

	for _, alert := range as {
		if err := json.NewEncoder(&buf).Encode(alert); err != nil {
			slog.Log(ctx, slog.LevelError, fmt.Sprintf("Failed to marshal alert: %s", alert.Name()), "err", err)
		}

		message := ckafka.Message{
			Key:   []byte(alert.Name()),
			Value: buf.Bytes(),
		}

		if n.conf.NumberOfPartition > 0 {
			message.Partition = n.GetPartitionIndex()
			n.NextPartition()
		}

		msgs = append(msgs, message)
	}

	if err := n.sendFunc(ctx, msgs...); err != nil {
		slog.Log(ctx, slog.LevelError, "Failed to send message", "err", err)
		return shouldRetry, err
	}

	return shouldRetry, nil
}
