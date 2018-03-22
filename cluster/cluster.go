package cluster

import (
	"context"
	"io/ioutil"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/hashicorp/memberlist"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"

	"github.com/prometheus/alertmanager/cluster/clusterpb"
	"github.com/prometheus/client_golang/prometheus"
)

// Peer is a single peer in a gossip cluster.
type Peer struct {
	mlist    *memberlist.Memberlist
	delegate *delegate

	mtx    sync.RWMutex
	states map[string]State
	stopc  chan struct{}
	readyc chan struct{}

	logger log.Logger
}

const (
	DefaultPushPullInterval = 60 * time.Second
	DefaultGossipInterval   = 200 * time.Millisecond
)

func Join(
	l log.Logger,
	reg prometheus.Registerer,
	bindAddr string,
	advertiseAddr string,
	knownPeers []string,
	waitIfEmpty bool,
	pushPullInterval time.Duration,
	gossipInterval time.Duration,
) (*Peer, error) {
	bindHost, bindPortStr, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return nil, err
	}
	bindPort, err := strconv.Atoi(bindPortStr)
	if err != nil {
		return nil, errors.Wrap(err, "invalid listen address")
	}
	var advertiseHost string
	var advertisePort int

	if advertiseAddr != "" {
		var advertisePortStr string
		advertiseHost, advertisePortStr, err = net.SplitHostPort(advertiseAddr)
		if err != nil {
			return nil, errors.Wrap(err, "invalid advertise address")
		}
		advertisePort, err = strconv.Atoi(advertisePortStr)
		if err != nil {
			return nil, errors.Wrap(err, "invalid advertise address, wrong port")
		}
	}

	resolvedPeers, err := resolvePeers(context.Background(), knownPeers, advertiseAddr, net.Resolver{}, waitIfEmpty)
	if err != nil {
		return nil, errors.Wrap(err, "resolve peers")
	}
	level.Debug(l).Log("msg", "resolved peers to following addresses", "peers", strings.Join(resolvedPeers, ","))

	// Initial validation of user-specified advertise address.
	addr, err := calculateAdvertiseAddress(bindHost, advertiseHost)
	if err != nil {
		level.Warn(l).Log("err", "couldn't deduce an advertise address: "+err.Error())
	} else if hasNonlocal(resolvedPeers) && isUnroutable(addr.String()) {
		level.Warn(l).Log("err", "this node advertises itself on an unroutable address", "addr", addr.String())
		level.Warn(l).Log("err", "this node will be unreachable in the cluster")
		level.Warn(l).Log("err", "provide --cluster.advertise-address as a routable IP address or hostname")
	}

	// TODO(fabxc): generate human-readable but random names?
	name, err := ulid.New(ulid.Now(), rand.New(rand.NewSource(time.Now().UnixNano())))
	if err != nil {
		return nil, err
	}

	p := &Peer{
		states: map[string]State{},
		stopc:  make(chan struct{}),
		readyc: make(chan struct{}),
		logger: l,
	}
	p.delegate = newDelegate(l, reg, p)

	cfg := memberlist.DefaultLANConfig()
	cfg.Name = name.String()
	cfg.BindAddr = bindHost
	cfg.BindPort = bindPort
	cfg.Delegate = p.delegate
	cfg.Events = p.delegate
	cfg.GossipInterval = gossipInterval
	cfg.PushPullInterval = pushPullInterval
	cfg.LogOutput = ioutil.Discard

	if advertiseAddr != "" {
		cfg.AdvertiseAddr = advertiseHost
		cfg.AdvertisePort = advertisePort
	}

	ml, err := memberlist.Create(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "create memberlist")
	}
	p.mlist = ml

	n, err := ml.Join(resolvedPeers)
	if err != nil {
		level.Warn(l).Log("msg", "failed to join cluster", "err", err)
	} else {
		level.Debug(l).Log("msg", "joined cluster", "peers", n)
	}

	if n > 0 {
		go p.warnIfAlone(l, 10*time.Second)
	}
	return p, nil
}

func (p *Peer) warnIfAlone(logger log.Logger, d time.Duration) {
	tick := time.NewTicker(d)
	defer tick.Stop()

	for {
		select {
		case <-p.stopc:
			return
		case <-tick.C:
			if n := p.mlist.NumMembers(); n <= 1 {
				level.Warn(logger).Log("NumMembers", n, "msg", "I appear to be alone in the cluster")
			}
		}
	}
}

// AddState adds a new state that will be gossiped. It returns a channel to which
// broadcast messages for the state can be sent.
func (p *Peer) AddState(key string, s State) *Channel {
	p.states[key] = s
	return &Channel{key: key, bcast: p.delegate.bcast}
}

// Leave the cluster, waiting up to timeout.
func (p *Peer) Leave(timeout time.Duration) error {
	close(p.stopc)
	return p.mlist.Leave(timeout)
}

// Name returns the unique ID of this peer in the cluster.
func (p *Peer) Name() string {
	return p.mlist.LocalNode().Name
}

// ClusterSize returns the current number of alive members in the cluster.
func (p *Peer) ClusterSize() int {
	return p.mlist.NumMembers()
}

// Return true when router has settled.
func (p *Peer) Ready() bool {
	select {
	case <-p.readyc:
		return true
	default:
	}
	return false
}

// Wait until Settle() has finished.
func (p *Peer) WaitReady() {
	<-p.readyc
}

// Return a status string representing the peer state.
func (p *Peer) Status() string {
	if p.Ready() {
		return "ready"
	} else {
		return "settling"
	}
}

// Info returns a JSON-serializable dump of cluster state.
// Useful for debug.
func (p *Peer) Info() map[string]interface{} {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	return map[string]interface{}{
		"self":    p.mlist.LocalNode(),
		"members": p.mlist.Members(),
	}
}

// Self returns the node information about the peer itself.
func (p *Peer) Self() *memberlist.Node {
	return p.mlist.LocalNode()
}

// Peers returns the peers in the cluster.
func (p *Peer) Peers() []*memberlist.Node {
	return p.mlist.Members()
}

// Position returns the position of the peer in the cluster.
func (p *Peer) Position() int {
	all := p.Peers()
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})

	k := 0
	for _, n := range all {
		if n.Name == p.Self().Name {
			break
		}
		k++
	}
	return k
}

// Settle waits until the mesh is ready (and sets the appropriate internal state when it is).
// The idea is that we don't want to start "working" before we get a chance to know most of the alerts and/or silences.
// Inspired from https://github.com/apache/cassandra/blob/7a40abb6a5108688fb1b10c375bb751cbb782ea4/src/java/org/apache/cassandra/gms/Gossiper.java
// This is clearly not perfect or strictly correct but should prevent the alertmanager to send notification before it is obviously not ready.
// This is especially important for those that do not have persistent storage.
func (p *Peer) Settle(ctx context.Context, interval time.Duration) {
	const NumOkayRequired = 3
	level.Info(p.logger).Log("msg", "Waiting for gossip to settle...", "interval", interval)
	start := time.Now()
	nPeers := 0
	nOkay := 0
	totalPolls := 0
	for {
		select {
		case <-ctx.Done():
			elapsed := time.Since(start)
			level.Info(p.logger).Log("msg", "gossip not settled but continuing anyway", "polls", totalPolls, "elapsed", elapsed)
			close(p.readyc)
			return
		case <-time.After(interval):
		}
		elapsed := time.Since(start)
		n := len(p.Peers())
		if nOkay >= NumOkayRequired {
			level.Info(p.logger).Log("msg", "gossip settled; proceeding", "elapsed", elapsed)
			break
		}
		if n == nPeers {
			nOkay++
			level.Debug(p.logger).Log("msg", "gossip looks settled", "elapsed", elapsed)
		} else {
			nOkay = 0
			level.Info(p.logger).Log("msg", "gossip not settled", "polls", totalPolls, "before", nPeers, "now", n, "elapsed", elapsed)
		}
		nPeers = n
		totalPolls++
	}
	close(p.readyc)
}

// State is a piece of state that can be serialized and merged with other
// serialized state.
type State interface {
	// MarshalBinary serializes the underlying state.
	MarshalBinary() ([]byte, error)

	// Merge merges serialized state into the underlying state.
	Merge(b []byte) error
}

// Channel allows clients to send messages for a specific state type that will be
// broadcasted in a best-effort manner.
type Channel struct {
	key   string
	bcast *memberlist.TransmitLimitedQueue
}

// We use a simple broadcast implementation in which items are never invalidated by others.
type simpleBroadcast []byte

func (b simpleBroadcast) Message() []byte                       { return []byte(b) }
func (b simpleBroadcast) Invalidates(memberlist.Broadcast) bool { return false }
func (b simpleBroadcast) Finished()                             {}

// Broadcast enqueues a message for broadcasting.
func (c *Channel) Broadcast(b []byte) {
	b, err := proto.Marshal(&clusterpb.Part{Key: c.key, Data: b})
	if err != nil {
		return
	}
	c.bcast.QueueBroadcast(simpleBroadcast(b))
}

// delegate implements memberlist.Delegate and memberlist.EventDelegate
// and broadcasts its peer's state in the cluster.
type delegate struct {
	*Peer

	logger log.Logger
	bcast  *memberlist.TransmitLimitedQueue

	messagesReceived     *prometheus.CounterVec
	messagesReceivedSize *prometheus.CounterVec
	messagesSent         *prometheus.CounterVec
	messagesSentSize     *prometheus.CounterVec
}

func newDelegate(l log.Logger, reg prometheus.Registerer, p *Peer) *delegate {
	bcast := &memberlist.TransmitLimitedQueue{
		NumNodes:       p.ClusterSize,
		RetransmitMult: 3,
	}
	messagesReceived := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_received_total",
		Help: "Total number of cluster messsages received.",
	}, []string{"msg_type"})
	messagesReceivedSize := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_received_size_total",
		Help: "Total size of cluster messages received.",
	}, []string{"msg_type"})
	messagesSent := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_sent_total",
		Help: "Total number of cluster messsages sent.",
	}, []string{"msg_type"})
	messagesSentSize := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "alertmanager_cluster_messages_sent_size_total",
		Help: "Total size of cluster messages sent.",
	}, []string{"msg_type"})
	gossipClusterMembers := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_cluster_members",
		Help: "Number indicating current number of members in cluster.",
	}, func() float64 {
		return float64(p.ClusterSize())
	})
	peerPosition := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_peer_position",
		Help: "Position the Alertmanager instance believes it's in. The position determines a peer's behavior in the cluster.",
	}, func() float64 {
		return float64(p.Position())
	})
	healthScore := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_cluster_health_score",
		Help: "Health score of the cluster. Lower values are better and zero means 'totally healthy'.",
	}, func() float64 {
		return float64(p.mlist.GetHealthScore())
	})
	messagesQueued := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_cluster_messages_queued",
		Help: "Number of cluster messsages which are queued.",
	}, func() float64 {
		return float64(bcast.NumQueued())
	})

	messagesReceived.WithLabelValues("full_state")
	messagesReceivedSize.WithLabelValues("full_state")
	messagesReceived.WithLabelValues("update")
	messagesReceivedSize.WithLabelValues("update")
	messagesSent.WithLabelValues("full_state")
	messagesSentSize.WithLabelValues("full_state")
	messagesSent.WithLabelValues("update")
	messagesSentSize.WithLabelValues("update")

	reg.MustRegister(messagesReceived, messagesReceivedSize, messagesSent, messagesSentSize,
		gossipClusterMembers, peerPosition, healthScore, messagesQueued)

	return &delegate{
		logger:               l,
		Peer:                 p,
		bcast:                bcast,
		messagesReceived:     messagesReceived,
		messagesReceivedSize: messagesReceivedSize,
		messagesSent:         messagesSent,
		messagesSentSize:     messagesSentSize,
	}
}

// NodeMeta retrieves meta-data about the current node when broadcasting an alive message.
func (d *delegate) NodeMeta(limit int) []byte {
	return []byte{}
}

// NotifyMsg is the callback invoked when a user-level gossip message is received.
func (d *delegate) NotifyMsg(b []byte) {
	d.messagesReceived.WithLabelValues("update").Inc()
	d.messagesReceivedSize.WithLabelValues("update").Add(float64(len(b)))

	var p clusterpb.Part
	if err := proto.Unmarshal(b, &p); err != nil {
		level.Warn(d.logger).Log("msg", "decode broadcast", "err", err)
		return
	}
	s, ok := d.states[p.Key]
	if !ok {
		return
	}
	if err := s.Merge(p.Data); err != nil {
		level.Warn(d.logger).Log("msg", "merge broadcast", "err", err, "key", p.Key)
		return
	}
}

// GetBroadcasts is called when user data messages can be broadcasted.
func (d *delegate) GetBroadcasts(overhead, limit int) [][]byte {
	msgs := d.bcast.GetBroadcasts(overhead, limit)
	d.messagesSent.WithLabelValues("update").Add(float64(len(msgs)))
	for _, m := range msgs {
		d.messagesSentSize.WithLabelValues("update").Add(float64(len(m)))
	}
	return msgs
}

// LocalState is called when gossip fetches local state.
func (d *delegate) LocalState(_ bool) []byte {
	all := &clusterpb.FullState{
		Parts: make([]clusterpb.Part, 0, len(d.states)),
	}
	for key, s := range d.states {
		b, err := s.MarshalBinary()
		if err != nil {
			level.Warn(d.logger).Log("msg", "encode local state", "err", err, "key", key)
			return nil
		}
		all.Parts = append(all.Parts, clusterpb.Part{Key: key, Data: b})
	}
	b, err := proto.Marshal(all)
	if err != nil {
		level.Warn(d.logger).Log("msg", "encode local state", "err", err)
		return nil
	}
	d.messagesSent.WithLabelValues("full_state").Inc()
	d.messagesSentSize.WithLabelValues("full_state").Add(float64(len(b)))
	return b
}

func (d *delegate) MergeRemoteState(buf []byte, _ bool) {
	d.messagesReceived.WithLabelValues("full_state").Inc()
	d.messagesReceivedSize.WithLabelValues("full_state").Add(float64(len(buf)))

	var fs clusterpb.FullState
	if err := proto.Unmarshal(buf, &fs); err != nil {
		level.Warn(d.logger).Log("msg", "merge remote state", "err", err)
		return
	}
	d.mtx.RLock()
	defer d.mtx.RUnlock()

	for _, p := range fs.Parts {
		s, ok := d.states[p.Key]
		if !ok {
			continue
		}
		if err := s.Merge(p.Data); err != nil {
			level.Warn(d.logger).Log("msg", "merge remote state", "err", err, "key", p.Key)
			return
		}
	}
}

// NotifyJoin is called if a peer joins the cluster.
func (d *delegate) NotifyJoin(n *memberlist.Node) {
	level.Debug(d.logger).Log("received", "NotifyJoin", "node", n.Name, "addr", n.Address())
}

// NotifyLeave is called if a peer leaves the cluster.
func (d *delegate) NotifyLeave(n *memberlist.Node) {
	level.Debug(d.logger).Log("received", "NotifyLeave", "node", n.Name, "addr", n.Address())
}

// NotifyUpdate is called if a cluster peer gets updated.
func (d *delegate) NotifyUpdate(n *memberlist.Node) {
	level.Debug(d.logger).Log("received", "NotifyUpdate", "node", n.Name, "addr", n.Address())
}

func resolvePeers(ctx context.Context, peers []string, myAddress string, res net.Resolver, waitIfEmpty bool) ([]string, error) {
	var resolvedPeers []string

	for _, peer := range peers {
		host, port, err := net.SplitHostPort(peer)
		if err != nil {
			return nil, errors.Wrapf(err, "split host/port for peer %s", peer)
		}

		retryCtx, cancel := context.WithCancel(ctx)

		ips, err := res.LookupIPAddr(ctx, host)
		if err != nil {
			// Assume direct address.
			resolvedPeers = append(resolvedPeers, peer)
			continue
		}

		if len(ips) == 0 {
			var lookupErrSpotted bool

			err := retry(2*time.Second, retryCtx.Done(), func() error {
				if lookupErrSpotted {
					// We need to invoke cancel in next run of retry when lookupErrSpotted to preserve LookupIPAddr error.
					cancel()
				}

				ips, err = res.LookupIPAddr(retryCtx, host)
				if err != nil {
					lookupErrSpotted = true
					return errors.Wrapf(err, "IP Addr lookup for peer %s", peer)
				}

				ips = removeMyAddr(ips, port, myAddress)
				if len(ips) == 0 {
					if !waitIfEmpty {
						return nil
					}
					return errors.New("empty IPAddr result. Retrying")
				}

				return nil
			})
			if err != nil {
				return nil, err
			}
		}

		for _, ip := range ips {
			resolvedPeers = append(resolvedPeers, net.JoinHostPort(ip.String(), port))
		}
	}

	return resolvedPeers, nil
}

func removeMyAddr(ips []net.IPAddr, targetPort string, myAddr string) []net.IPAddr {
	var result []net.IPAddr

	for _, ip := range ips {
		if net.JoinHostPort(ip.String(), targetPort) == myAddr {
			continue
		}
		result = append(result, ip)
	}

	return result
}

func hasNonlocal(clusterPeers []string) bool {
	for _, peer := range clusterPeers {
		if host, _, err := net.SplitHostPort(peer); err == nil {
			peer = host
		}
		if ip := net.ParseIP(peer); ip != nil && !ip.IsLoopback() {
			return true
		} else if ip == nil && strings.ToLower(peer) != "localhost" {
			return true
		}
	}
	return false
}

func isUnroutable(addr string) bool {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	if ip := net.ParseIP(addr); ip != nil && (ip.IsUnspecified() || ip.IsLoopback()) {
		return true // typically 0.0.0.0 or localhost
	} else if ip == nil && strings.ToLower(addr) == "localhost" {
		return true
	}
	return false
}

// retry executes f every interval seconds until timeout or no error is returned from f.
func retry(interval time.Duration, stopc <-chan struct{}, f func() error) error {
	tick := time.NewTicker(interval)
	defer tick.Stop()

	var err error
	for {
		if err = f(); err == nil {
			return nil
		}
		select {
		case <-stopc:
			return err
		case <-tick.C:
		}
	}
}
