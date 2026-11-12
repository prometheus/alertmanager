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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/prometheus/alertmanager/notify"
	notifytest "github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/notify/webhook"
	"github.com/prometheus/alertmanager/types"
)

func TestNotifyProducesWebhookV4Message(t *testing.T) {
	const topic = "alerts"
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, topic))
	require.NoError(t, err)
	t.Cleanup(cluster.Close)

	n, err := New(&Config{Brokers: cluster.ListenAddrs(), Topic: topic}, notifytest.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, n.Close()) })

	alert := &types.Alert{Alert: model.Alert{
		Labels:      model.LabelSet{"alertname": "HighLatency", "severity": "critical"},
		Annotations: model.LabelSet{"summary": "Latency is high"},
		StartsAt:    time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
	}}
	const groupKey = `{}/{alertname="HighLatency"}`
	ctx := notify.WithGroupKey(context.Background(), groupKey)
	ctx = notify.WithReceiverName(ctx, "kafka-alerts")
	ctx = notify.WithGroupLabels(ctx, model.LabelSet{"alertname": "HighLatency"})

	retry, err := n.Notify(ctx, alert)
	require.NoError(t, err)
	require.False(t, retry)

	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(cluster.ListenAddrs()...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err)
	defer consumer.Close()
	fetchCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fetches := consumer.PollFetches(fetchCtx)
	require.NoError(t, fetches.Err())
	records := fetches.Records()
	require.Len(t, records, 1)
	require.Equal(t, groupKey, string(records[0].Key))

	var msg webhook.Message
	require.NoError(t, json.Unmarshal(records[0].Value, &msg))
	require.Equal(t, "4", msg.Version)
	require.Equal(t, groupKey, msg.GroupKey)
	require.Equal(t, "kafka-alerts", msg.Receiver)
	require.Equal(t, "firing", msg.Status)
	require.Len(t, msg.Alerts, 1)
	require.Equal(t, "HighLatency", msg.Alerts[0].Labels["alertname"])
}

type fakeProducer struct {
	err    error
	closed bool
}

func (p *fakeProducer) ProduceSync(_ context.Context, records ...*kgo.Record) kgo.ProduceResults {
	results := make(kgo.ProduceResults, len(records))
	for i, record := range records {
		results[i] = kgo.ProduceResult{Record: record, Err: p.err}
	}
	return results
}

func (p *fakeProducer) Close() { p.closed = true }

func TestNotifyErrors(t *testing.T) {
	n := &Notifier{
		conf:     &Config{Topic: "alerts"},
		tmpl:     notifytest.CreateTmpl(t),
		logger:   promslog.NewNopLogger(),
		producer: &fakeProducer{err: errors.New("broker unavailable")},
	}

	retry, err := n.Notify(context.Background(), &types.Alert{})
	require.ErrorContains(t, err, "group key missing")
	require.False(t, retry)

	ctx := notify.WithGroupKey(context.Background(), "group")
	retry, err = n.Notify(ctx, &types.Alert{})
	require.ErrorContains(t, err, "broker unavailable")
	require.True(t, retry)
}

func TestClose(t *testing.T) {
	p := &fakeProducer{}
	n := &Notifier{producer: p}
	require.NoError(t, n.Close())
	require.True(t, p.closed)
}
