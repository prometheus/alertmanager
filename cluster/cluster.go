package cluster

import (
	"context"
	"fmt"
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

	peerLock    sync.RWMutex
	peers       map[string]peer
	failedPeers []peer

	failedReconnectionsCounter     prometheus.Counter
	successfulReconnectionsCounter prometheus.Counter
	peerLeaveCounter               prometheus.Counter
	peerUpdateCounter              prometheus.Counter
	peerJoinCounter                prometheus.Counter

	logger log.Logger
}

// peer is an internal type used for bookkeeping. It holds the state of peers
// in the cluster.
type peer struct {
	status    PeerStatus
	leaveTime time.Time

	*memberlist.Node
}

// PeerStatus is the state that a peer is in.
type PeerStatus int

const (
	StatusNone PeerStatus = iota
	StatusAlive
	StatusFailed
)

func (s PeerStatus) String() string {
	switch s {
	case StatusNone:
		return "none"
	case StatusAlive:
		return "alive"
	case StatusFailed:
		return "failed"
	default:
		panic(fmt.Sprintf("unknown PeerStatus: %d", s))
	}
}

const (
	DefaultPushPullInterval  = 60 * time.Second
	DefaultGossipInterval    = 200 * time.Millisecond
	DefaultTcpTimeout        = 10 * time.Second
	DefaultProbeTimeout      = 500 * time.Millisecond
	DefaultProbeInterval     = 1 * time.Second
	DefaultReconnectInterval = 10 * time.Second
	DefaultReconnectTimeout  = 6 * time.Hour
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
	tcpTimeout time.Duration,
	probeTimeout time.Duration,
	probeInterval time.Duration,
	reconnectInterval time.Duration,
	reconnectTimeout time.Duration,
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
		peers:  map[string]peer{},
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
	cfg.TCPTimeout = tcpTimeout
	cfg.ProbeTimeout = probeTimeout
	cfg.ProbeInterval = probeInterval
	cfg.LogOutput = &logWriter{l: l}

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

	if reconnectInterval != 0 {
		go p.handleReconnect(reconnectInterval)
	}
	if reconnectTimeout != 0 {
		go p.handleReconnectTimeout(5*time.Minute, reconnectTimeout)
	}
	p.register(reg)

	return p, nil
}

type logWriter struct {
	l log.Logger
}

func (l *logWriter) Write(b []byte) (int, error) {
	return len(b), level.Debug(l.l).Log("memberlist", string(b))
}

func (p *Peer) register(reg prometheus.Registerer) {
	clusterFailedPeers := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "alertmanager_cluster_failed_peers",
		Help: "Number indicating the current number of failed peers in the cluster.",
	}, func() float64 {
		p.peerLock.RLock()
		defer p.peerLock.RUnlock()

		return float64(len(p.failedPeers))
	})
	p.failedReconnectionsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_cluster_failed_reconnections",
		Help: "A counter of the number of failed cluster peer reconnection attempts.",
	})

	p.successfulReconnectionsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_cluster_successful_reconnections",
		Help: "A counter of the number of successful cluster peer reconnections.",
	})

	p.peerLeaveCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_cluster_peers_left",
		Help: "A counter of the number of peers that have left.",
	})
	p.peerUpdateCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_cluster_peers_update",
		Help: "A counter of the number of peers that have updated metadata.",
	})
	p.peerJoinCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_cluster_peers_joined",
		Help: "A counter of the number of peers that have joined.",
	})

	reg.MustRegister(clusterFailedPeers, p.failedReconnectionsCounter, p.successfulReconnectionsCounter,
		p.peerLeaveCounter, p.peerUpdateCounter, p.peerJoinCounter)
}

func (p *Peer) handleReconnectTimeout(d time.Duration, timeout time.Duration) {
	tick := time.NewTicker(d)
	defer tick.Stop()

	for {
		select {
		case <-p.stopc:
			return
		case <-tick.C:
			p.removeFailedPeers(timeout)
		}
	}
}

func (p *Peer) removeFailedPeers(timeout time.Duration) {
	p.peerLock.Lock()
	defer p.peerLock.Unlock()

	now := time.Now()

	keep := make([]peer, 0, len(p.failedPeers))
	for _, pr := range p.failedPeers {
		if pr.leaveTime.Add(timeout).After(now) {
			keep = append(keep, pr)
		}
	}

	p.failedPeers = keep
}

func (p *Peer) handleReconnect(d time.Duration) {
	tick := time.NewTicker(d)
	defer tick.Stop()

	for {
		select {
		case <-p.stopc:
			return
		case <-tick.C:
			p.reconnect()
		}
	}
}

func (p *Peer) reconnect() {
	p.peerLock.RLock()
	defer p.peerLock.RUnlock()

	logger := log.With(p.logger, "msg", "reconnect")
	for _, pr := range p.failedPeers {
		// No need to do book keeping on failedPeers here. If a
		// reconnect is successful, they will be announced in
		// peerJoin().
		if _, err := p.mlist.Join([]string{pr.Address()}); err != nil {
			p.failedReconnectionsCounter.Inc()
			level.Debug(logger).Log("result", "failure", "peer", pr.Node, "addr", pr.Address())
		} else {
			p.successfulReconnectionsCounter.Inc()
			level.Debug(logger).Log("result", "success", "peer", pr.Node, "addr", pr.Address())
		}
	}
}

func (p *Peer) peerJoin(n *memberlist.Node) {
	p.peerLock.Lock()
	defer p.peerLock.Unlock()

	var oldStatus PeerStatus
	pr, ok := p.peers[n.Name]
	if !ok {
		oldStatus = StatusNone
		pr = peer{
			status: StatusAlive,
			Node:   n,
		}
	} else {
		oldStatus = pr.status
		pr.Node = n
		pr.status = StatusAlive
		pr.leaveTime = time.Time{}
	}

	p.peers[n.Name] = pr

	if oldStatus == StatusFailed {
		level.Debug(p.logger).Log("msg", "peer rejoined", "peer", pr.Node)
		p.failedPeers = removeOldPeer(p.failedPeers, pr.Name)
	}
}

func (p *Peer) peerLeave(n *memberlist.Node) {
	p.peerLock.Lock()
	defer p.peerLock.Unlock()

	pr, ok := p.peers[n.Name]
	if !ok {
		// Why are we receiving a leave notification from a node that
		// never joined?
		return
	}

	pr.status = StatusFailed
	pr.leaveTime = time.Now()
	p.failedPeers = append(p.failedPeers, pr)
	p.peers[n.Name] = pr

	p.peerLeaveCounter.Inc()
	level.Debug(p.logger).Log("msg", "peer left", "peer", pr.Node)
}

func (p *Peer) peerUpdate(n *memberlist.Node) {
	p.peerLock.Lock()
	defer p.peerLock.Unlock()

	pr, ok := p.peers[n.Name]
	if !ok {
		// Why are we receiving an update from a node that never
		// joined?
		return
	}

	pr.Node = n
	p.peers[n.Name] = pr

	p.peerUpdateCounter.Inc()
	level.Debug(p.logger).Log("msg", "peer updated", "peer", pr.Node)
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
	level.Debug(p.logger).Log("msg", "leaving cluster")
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

func removeOldPeer(old []peer, name string) []peer {
	new := make([]peer, 0, len(old))
	for _, p := range old {
		if p.Name != name {
			new = append(new, p)
		}
	}

	return new
}
