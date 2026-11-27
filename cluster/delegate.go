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
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"

	"github.com/prometheus/alertmanager/cluster/clusterpb"
)

const (
	// Maximum number of messages to be held in the queue.
	maxQueueSize = 4096
	fullState    = "full_state"
	update       = "update"

	// Mirrors memberlist's unexported maxPushStateBytes: peers reject
	// push/pull messages whose user state exceeds it.
	maxPushPullStateSize = 20 * 1024 * 1024
)

// delegate implements memberlist.Delegate and memberlist.EventDelegate
// and broadcasts its peer's state in the cluster.
type delegate struct {
	*Peer

	logger *slog.Logger
	bcast  *memberlist.TransmitLimitedQueue

	messagesReceived     *prometheus.CounterVec
	messagesReceivedSize *prometheus.CounterVec
	messagesSent         *prometheus.CounterVec
	messagesSentSize     *prometheus.CounterVec
	messagesPruned       prometheus.Counter
	nodeAlive            *prometheus.CounterVec
	nodePingDuration     *prometheus.HistogramVec
	conflictsCount       prometheus.Counter
}

func newDelegate(l *slog.Logger, reg prometheus.Registerer, p *Peer, retransmit int) *delegate {
	bcast := &memberlist.TransmitLimitedQueue{
		NumNodes:       p.ClusterSize,
		RetransmitMult: retransmit,
	}
	messagesReceived := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_received_total",
		Help: "Total number of cluster messages received.",
	}, []string{"msg_type"})
	messagesReceivedSize := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_received_size_total",
		Help: "Total size of cluster messages received.",
	}, []string{"msg_type"})
	messagesSent := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_sent_total",
		Help: "Total number of cluster messages sent.",
	}, []string{"msg_type"})
	messagesSentSize := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_sent_size_total",
		Help: "Total size of cluster messages sent.",
	}, []string{"msg_type"})
	messagesPruned := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_pruned_total",
		Help: "Total number of cluster messages pruned.",
	})
	promauto.With(reg).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_cluster_members",
		Help: "Number indicating current number of members in cluster.",
	}, func() float64 {
		return float64(p.ClusterSize())
	})
	promauto.With(reg).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_peer_position",
		Help: "Position the Alertmanager instance believes it's in. The position determines a peer's behavior in the cluster.",
	}, func() float64 {
		return float64(p.Position())
	})
	promauto.With(reg).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_cluster_health_score",
		Help: "Health score of the cluster. Lower values are better and zero means 'totally healthy'.",
	}, func() float64 {
		return float64(p.mlist.GetHealthScore())
	})
	promauto.With(reg).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_cluster_messages_queued",
		Help: "Number of cluster messages which are queued.",
	}, func() float64 {
		return float64(bcast.NumQueued())
	})
	nodeAlive := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_alive_messages_total",
		Help: "Total number of received alive messages.",
	}, []string{"peer"},
	)
	nodePingDuration := promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
		Name:                            "alertmanager_cluster_pings_seconds",
		Help:                            "Histogram of latencies for ping messages.",
		Buckets:                         []float64{.005, .01, .025, .05, .1, .25, .5},
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	}, []string{"peer"},
	)
	conflictsCount := promauto.With(reg).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_cluster_peer_name_conflicts_total",
		Help: "Total number of times memberlist has noticed conflicting peer names",
	})

	messagesReceived.WithLabelValues(fullState)
	messagesReceivedSize.WithLabelValues(fullState)
	messagesReceived.WithLabelValues(update)
	messagesReceivedSize.WithLabelValues(update)
	messagesSent.WithLabelValues(fullState)
	messagesSentSize.WithLabelValues(fullState)
	messagesSent.WithLabelValues(update)
	messagesSentSize.WithLabelValues(update)

	d := &delegate{
		logger:               l,
		Peer:                 p,
		bcast:                bcast,
		messagesReceived:     messagesReceived,
		messagesReceivedSize: messagesReceivedSize,
		messagesSent:         messagesSent,
		messagesSentSize:     messagesSentSize,
		messagesPruned:       messagesPruned,
		nodeAlive:            nodeAlive,
		nodePingDuration:     nodePingDuration,
		conflictsCount:       conflictsCount,
	}

	go d.handleQueueDepth()

	return d
}

// NodeMeta retrieves meta-data about the current node when broadcasting an alive message.
func (d *delegate) NodeMeta(limit int) []byte {
	return []byte{}
}

// NotifyMsg is the callback invoked when a user-level gossip message is received.
func (d *delegate) NotifyMsg(b []byte) {
	var p clusterpb.Part
	if err := proto.Unmarshal(b, &p); err != nil {
		d.messagesReceived.WithLabelValues(update).Inc()
		d.messagesReceivedSize.WithLabelValues(update).Add(float64(len(b)))
		d.logger.Warn("decode broadcast", "err", err)
		return
	}

	msgType := update
	if p.FullState {
		msgType = fullState
	}
	d.messagesReceived.WithLabelValues(msgType).Inc()
	d.messagesReceivedSize.WithLabelValues(msgType).Add(float64(len(b)))

	d.mtx.RLock()
	s, ok := d.states[p.Key]
	d.mtx.RUnlock()

	if !ok {
		return
	}
	if err := s.Merge(p.Data); err != nil {
		d.logger.Warn("merge broadcast", "err", err, "key", p.Key)
		return
	}
}

// NotifyConflict is the callback when memberlist encounters two nodes with the same ID.
func (d *delegate) NotifyConflict(existing, other *memberlist.Node) {
	d.logger.Warn("Found conflicting peer IDs", "peer", existing.Name)
	d.conflictsCount.Inc()
}

// GetBroadcasts is called when user data messages can be broadcasted.
func (d *delegate) GetBroadcasts(overhead, limit int) [][]byte {
	msgs := d.bcast.GetBroadcasts(overhead, limit)
	d.messagesSent.WithLabelValues(update).Add(float64(len(msgs)))
	for _, m := range msgs {
		d.messagesSentSize.WithLabelValues(update).Add(float64(len(m)))
	}
	return msgs
}

// LocalState is called when gossip fetches local state.
func (d *delegate) LocalState(_ bool) []byte {
	parts, err := d.localParts()
	if err != nil {
		d.logger.Warn("encode local state", "err", err)
		return nil
	}
	b, err := proto.Marshal(&clusterpb.FullState{Parts: parts})
	if err != nil {
		d.logger.Warn("encode local state", "err", err)
		return nil
	}
	if len(b) > maxPushPullStateSize {
		// Peers would reject the whole push/pull, so leave the state out and
		// let syncLargeState deliver it instead.
		d.logger.Debug("local state exceeds push/pull limit, sending it via reliable messages", "size", len(b), "limit", maxPushPullStateSize)
		return nil
	}
	d.messagesSent.WithLabelValues(fullState).Inc()
	d.messagesSentSize.WithLabelValues(fullState).Add(float64(len(b)))
	return b
}

func (d *delegate) localParts() ([]*clusterpb.Part, error) {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	parts := make([]*clusterpb.Part, 0, len(d.states))
	for key, s := range d.states {
		b, err := s.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
		parts = append(parts, &clusterpb.Part{Key: key, Data: b})
	}
	return parts, nil
}

// syncLargeState sends the local state to a random peer when it is too large
// for memberlist's push/pull.
func (d *delegate) syncLargeState() {
	if nodes := d.otherMembers(); len(nodes) > 0 {
		d.sendLargeState(nodes[rand.IntN(len(nodes))])
	}
}

// sendLargeState sends each local state part to n as a reliable user message
// if the full state is too large for memberlist's push/pull. Receivers merge
// the parts in NotifyMsg.
func (d *delegate) sendLargeState(n *memberlist.Node) {
	parts, err := d.localParts()
	if err != nil {
		d.logger.Warn("encode local state", "err", err)
		return
	}
	if proto.Size(&clusterpb.FullState{Parts: parts}) <= maxPushPullStateSize {
		return
	}
	for _, p := range parts {
		p.FullState = true
		b, err := proto.Marshal(p)
		if err != nil {
			d.logger.Warn("encode local state", "err", err, "key", p.Key)
			return
		}
		if err := d.mlist.SendReliable(n, b); err != nil {
			d.logger.Warn("send local state", "err", err, "key", p.Key, "peer", n.Name)
			return
		}
		d.messagesSent.WithLabelValues(fullState).Inc()
		d.messagesSentSize.WithLabelValues(fullState).Add(float64(len(b)))
	}
}

func (d *delegate) MergeRemoteState(buf []byte, _ bool) {
	d.messagesReceived.WithLabelValues(fullState).Inc()
	d.messagesReceivedSize.WithLabelValues(fullState).Add(float64(len(buf)))

	var fs clusterpb.FullState
	if err := proto.Unmarshal(buf, &fs); err != nil {
		d.logger.Warn("merge remote state", "err", err)
		return
	}
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	for _, p := range fs.Parts {
		s, ok := d.states[p.Key]
		if !ok {
			d.logger.Warn("unknown state key", "len", len(buf), "key", p.Key)
			continue
		}
		if err := s.Merge(p.Data); err != nil {
			d.logger.Warn("merge remote state", "err", err, "key", p.Key)
			return
		}
	}
}

// NotifyJoin is called if a peer joins the cluster.
func (d *delegate) NotifyJoin(n *memberlist.Node) {
	d.logger.Debug("NotifyJoin", "node", n.Name, "addr", n.Address())
	d.peerJoin(n)
	if n.Name != d.name {
		// Memberlist holds its node lock while calling NotifyJoin.
		go d.sendLargeState(n)
	}
}

// NotifyLeave is called if a peer leaves the cluster.
func (d *delegate) NotifyLeave(n *memberlist.Node) {
	d.logger.Debug("NotifyLeave", "node", n.Name, "addr", n.Address())
	d.peerLeave(n)
}

// NotifyUpdate is called if a cluster peer gets updated.
func (d *delegate) NotifyUpdate(n *memberlist.Node) {
	d.logger.Debug("NotifyUpdate", "node", n.Name, "addr", n.Address())
	d.peerUpdate(n)
}

// NotifyAlive implements the memberlist.AliveDelegate interface.
func (d *delegate) NotifyAlive(peer *memberlist.Node) error {
	d.nodeAlive.WithLabelValues(peer.Name).Inc()
	return nil
}

// AckPayload implements the memberlist.PingDelegate interface.
func (d *delegate) AckPayload() []byte {
	return []byte{}
}

// NotifyPingComplete implements the memberlist.PingDelegate interface.
func (d *delegate) NotifyPingComplete(peer *memberlist.Node, rtt time.Duration, payload []byte) {
	d.nodePingDuration.WithLabelValues(peer.Name).Observe(rtt.Seconds())
}

// handleQueueDepth ensures that the queue doesn't grow unbounded by pruning
// older messages at regular interval.
func (d *delegate) handleQueueDepth() {
	for {
		select {
		case <-d.stopc:
			return
		case <-time.After(15 * time.Minute):
			n := d.bcast.NumQueued()
			if n > maxQueueSize {
				d.logger.Warn("dropping messages because too many are queued", "current", n, "limit", maxQueueSize)
				d.bcast.Prune(maxQueueSize)
				d.messagesPruned.Add(float64(n - maxQueueSize))
			}
		}
	}
}
