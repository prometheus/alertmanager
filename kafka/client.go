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

package kafka

import (
	"context"
	"fmt"
	"log/slog"

	commoncfg "github.com/prometheus/common/config"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kslog"
)

// BuildOpts converts the high-level ClientOptions into a slice of
// franz-go options.  Callers append role-specific options (e.g.
// kgo.ConsumeTopics for consumers, or extra producer tuning) before
// passing the slice to kgo.NewClient.
//
// The returned option set:
//
//   - Seeds the supplied brokers and sets ClientID (defaulting to
//     DefaultClientID).
//   - Sets DefaultProduceTopic when Topic is non-empty.
//   - Applies ProducerLinger(DefaultLinger) — harmless for consumers.
//   - Applies RequiredAcks based on opts.Acks (defaults to LeaderAck).
//   - Disables idempotent writes unless the caller explicitly opted
//     into AcksAll; franz-go's idempotent producer mandates acks=all
//     and our default is acks=leader for low latency.
//   - Applies ProducerBatchCompression when opts.Compression is set.
//   - Applies DialTLSConfig when opts.TLSConfig is non-nil.
//   - Wires a kslog adapter so franz-go logs through the caller's
//     slog.Logger.  A nil logger is replaced with a discard logger.
//
// Validate is called internally; callers do not need to call it
// separately.
func BuildOpts(opts ClientOptions, logger *slog.Logger) ([]kgo.Opt, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	clientID := opts.ClientID
	if clientID == "" {
		clientID = DefaultClientID
	}

	kopts := []kgo.Opt{
		kgo.SeedBrokers(opts.Brokers...),
		kgo.ClientID(clientID),
		kgo.ProducerLinger(DefaultLinger),
		kgo.WithLogger(kslog.New(logger)),
	}
	if opts.Topic != "" {
		kopts = append(kopts, kgo.DefaultProduceTopic(opts.Topic))
	}

	acks, err := acksOpt(opts.Acks)
	if err != nil {
		return nil, err
	}
	kopts = append(kopts, acks)

	// franz-go's idempotent producer requires acks=all; disable it
	// when the caller chose a weaker (or unspecified, leader) ack
	// level to keep the default low-latency path working.
	if opts.Acks != AcksAll {
		kopts = append(kopts, kgo.DisableIdempotentWrite())
	}

	if opts.Compression != "" {
		codec, err := compressionCodec(opts.Compression)
		if err != nil {
			return nil, err
		}
		kopts = append(kopts, kgo.ProducerBatchCompression(codec))
	}

	if opts.TLSConfig != nil {
		tlsCfg, err := commoncfg.NewTLSConfig(opts.TLSConfig)
		if err != nil {
			return nil, fmt.Errorf("kafka: building TLS config: %w", err)
		}
		kopts = append(kopts, kgo.DialTLSConfig(tlsCfg))
	}

	return kopts, nil
}

// PingInBackground performs a best-effort connectivity check against
// the supplied client without blocking the caller.  Failure is logged
// at warn level; franz-go retries connections internally so subsequent
// produce/fetch calls will succeed once a broker becomes reachable.
//
// The goroutine exits after DefaultPingTimeout or when the client is
// closed (which cancels any in-flight broker dial).
func PingInBackground(client *kgo.Client, logger *slog.Logger) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
		defer cancel()
		if err := client.Ping(ctx); err != nil {
			logger.Warn("Kafka client could not reach brokers at startup; will retry in background",
				"err", err)
		}
	}()
}

// acksOpt translates the user-facing acks string into a franz-go
// option.  An empty string defaults to LeaderAck.
func acksOpt(s string) (kgo.Opt, error) {
	switch s {
	case "", AcksLeader:
		return kgo.RequiredAcks(kgo.LeaderAck()), nil
	case AcksNone:
		return kgo.RequiredAcks(kgo.NoAck()), nil
	case AcksAll:
		return kgo.RequiredAcks(kgo.AllISRAcks()), nil
	default:
		return nil, fmt.Errorf("kafka: unknown acks %q", s)
	}
}

// compressionCodec translates the user-facing compression string into
// a franz-go codec.
func compressionCodec(s string) (kgo.CompressionCodec, error) {
	switch s {
	case CompressionNone:
		return kgo.NoCompression(), nil
	case CompressionGzip:
		return kgo.GzipCompression(), nil
	case CompressionSnappy:
		return kgo.SnappyCompression(), nil
	case CompressionLZ4:
		return kgo.Lz4Compression(), nil
	case CompressionZstd:
		return kgo.ZstdCompression(), nil
	default:
		return kgo.NoCompression(), fmt.Errorf("kafka: unknown compression %q", s)
	}
}
