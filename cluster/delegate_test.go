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

package cluster

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/prometheus/alertmanager/cluster/clusterpb"
)

func TestNotifyMsgMetricType(t *testing.T) {
	t.Parallel()

	messagesReceived := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_messages_received_total",
	}, []string{"msg_type"})
	messagesReceivedSize := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_messages_received_size_total",
	}, []string{"msg_type"})
	d := &delegate{
		Peer:                 &Peer{states: map[string]State{}},
		messagesReceived:     messagesReceived,
		messagesReceivedSize: messagesReceivedSize,
	}

	for _, tc := range []struct {
		name      string
		fullState bool
		msgType   string
	}{
		{name: "update", msgType: update},
		{name: "full state", fullState: true, msgType: fullState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := proto.Marshal(&clusterpb.Part{FullState: tc.fullState})
			require.NoError(t, err)

			d.NotifyMsg(msg)

			require.Equal(t, float64(1), testutil.ToFloat64(messagesReceived.WithLabelValues(tc.msgType)))
			require.Equal(t, float64(len(msg)), testutil.ToFloat64(messagesReceivedSize.WithLabelValues(tc.msgType)))
		})
	}
}
