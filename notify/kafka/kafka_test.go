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
	"context"
	"sync"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"

	ckafka "github.com/segmentio/kafka-go"
)

func TestKafkaNotify(t *testing.T) {
	sendFunc := func(ctx context.Context, msgs ...ckafka.Message) error {
		return nil
	}

	notifier, err := New(
		&config.KafkaConfig{
			Brokers: []string{"localhost:9092"},
		},
		promslog.NewNopLogger(),
		&sendFunc,
	)

	require.NoError(t, err)
	require.NotNil(t, notifier)

	notifier.Notify(context.Background(), &types.Alert{})

	require.Equal(t, 0, notifier.Partition)
}

func TestKafkaNotifyRoundRobin(t *testing.T) {
	var (
		counter      int
		counterMutex sync.Mutex
	)
	partitions := 2
	sendFunc := func(ctx context.Context, msgs ...ckafka.Message) error {
		counterMutex.Lock()
		defer counterMutex.Unlock()
		require.Equal(t, counter%partitions, msgs[0].Partition)
		counter++
		return nil
	}

	notifier, err := New(
		&config.KafkaConfig{
			Brokers:           []string{"localhost:9092"},
			NumberOfPartition: partitions,
		},
		promslog.NewNopLogger(),
		&sendFunc,
	)

	require.NoError(t, err)
	require.NotNil(t, notifier)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		notifier.Notify(context.Background(), &types.Alert{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		notifier.Notify(context.Background(), &types.Alert{})
	}()

	wg.Wait()

	require.Equal(t, partitions, counter)
}
