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
package notify

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/alertmanager/alert"
)

// ClusterGossipSettleStage waits until the Gossip has settled to forward alerts.
type ClusterGossipSettleStage struct {
	peer Peer
}

// NewClusterGossipSettleStage returns a new ClusterGossipSettleStage.
func NewClusterGossipSettleStage(p Peer) *ClusterGossipSettleStage {
	return &ClusterGossipSettleStage{peer: p}
}

func (n *ClusterGossipSettleStage) Exec(ctx context.Context, _ *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	if n.peer != nil {
		if err := n.peer.WaitReady(ctx); err != nil {
			return ctx, nil, err
		}
	}
	return ctx, alerts, nil
}

// ClusterWaitStage waits for a certain amount of time before continuing or until the
// context is done.
type ClusterWaitStage struct {
	wait func() time.Duration
}

// NewClusterWaitStage returns a new ClusterWaitStage.
func NewClusterWaitStage(wait func() time.Duration) *ClusterWaitStage {
	return &ClusterWaitStage{
		wait: wait,
	}
}

// Exec implements the Stage interface.
func (ws *ClusterWaitStage) Exec(ctx context.Context, _ *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	select {
	case <-time.After(ws.wait()):
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	}
	return ctx, alerts, nil
}
