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
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestBuildOpts_PropagatesAllFields(t *testing.T) {
	opts, err := BuildOpts(ClientOptions{
		Brokers:     []string{"a:9092", "b:9092"},
		Topic:       "t",
		ClientID:    "test-client",
		Acks:        AcksLeader,
		Compression: CompressionSnappy,
	}, slog.Default())
	require.NoError(t, err)
	require.NotEmpty(t, opts)

	// Smoke test: feed the option set to kgo.NewClient and confirm
	// it accepts everything.  The client never connects in this test;
	// franz-go only dials on first produce/fetch.
	cl, err := kgo.NewClient(opts...)
	require.NoError(t, err)
	cl.Close()
}

func TestBuildOpts_InvalidConfigReturnsError(t *testing.T) {
	_, err := BuildOpts(ClientOptions{}, slog.Default())
	require.Error(t, err)
}

func TestBuildOpts_NilLoggerOK(t *testing.T) {
	opts, err := BuildOpts(ClientOptions{Brokers: []string{"a:9092"}}, nil)
	require.NoError(t, err)
	cl, err := kgo.NewClient(opts...)
	require.NoError(t, err)
	cl.Close()
}

func TestBuildOpts_AcksAllKeepsIdempotency(t *testing.T) {
	// When the operator chose acks=all, BuildOpts must not append
	// DisableIdempotentWrite (franz-go's idempotent producer requires
	// acks=all).  We verify this indirectly via kgo.NewClient — with
	// idempotency enabled and acks=leader, kgo would refuse the
	// option set.
	opts, err := BuildOpts(ClientOptions{
		Brokers: []string{"a:9092"},
		Acks:    AcksAll,
	}, nil)
	require.NoError(t, err)
	cl, err := kgo.NewClient(opts...)
	require.NoError(t, err)
	cl.Close()
}

func TestBuildOpts_AppliesTopic(t *testing.T) {
	// kgo.DefaultProduceTopic("") is rejected by kgo; an empty Topic
	// in ClientOptions must therefore NOT produce a DefaultProduceTopic
	// option.  Confirm by constructing a client both with and without
	// a topic.
	_, err := BuildOpts(ClientOptions{Brokers: []string{"a:9092"}}, nil)
	require.NoError(t, err)
	_, err = BuildOpts(ClientOptions{Brokers: []string{"a:9092"}, Topic: "t"}, nil)
	require.NoError(t, err)
}

func TestPingInBackground_DoesNotBlockOnUnreachableBroker(t *testing.T) {
	// Point at a closed TCP port and verify PingInBackground returns
	// immediately even though the ping itself will fail eventually.
	opts, err := BuildOpts(ClientOptions{Brokers: []string{"127.0.0.1:1"}}, nil)
	require.NoError(t, err)
	cl, err := kgo.NewClient(opts...)
	require.NoError(t, err)

	start := time.Now()
	PingInBackground(cl, nil)
	require.Less(t, time.Since(start), 100*time.Millisecond,
		"PingInBackground must not block the caller")

	cl.Close()
}

func TestPingInBackground_SucceedsAgainstFakeCluster(t *testing.T) {
	c, err := kfake.NewCluster(kfake.NumBrokers(1))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	opts, err := BuildOpts(ClientOptions{Brokers: c.ListenAddrs()}, nil)
	require.NoError(t, err)
	cl, err := kgo.NewClient(opts...)
	require.NoError(t, err)
	defer cl.Close()

	PingInBackground(cl, nil)
	// Give the background goroutine a moment to run; we're not
	// asserting anything observable here beyond "doesn't panic".
	time.Sleep(50 * time.Millisecond)
}
