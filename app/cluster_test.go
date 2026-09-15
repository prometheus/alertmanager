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

package app

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/cluster"
)

// newTestPeer creates a single-node gossip peer bound to an ephemeral
// port, registering its teardown with t.Cleanup.
func newTestPeer(t *testing.T) *cluster.Peer {
	t.Helper()

	peer, err := cluster.Create(
		promslog.NewNopLogger(),
		prometheus.NewRegistry(),
		"127.0.0.1:0", // bind
		"",            // advertise
		nil,           // known peers
		true,          // wait if empty
		cluster.DefaultPushPullInterval,
		cluster.DefaultGossipInterval,
		cluster.DefaultTCPTimeout,
		cluster.DefaultResolvePeersTimeout,
		cluster.DefaultProbeTimeout,
		cluster.DefaultProbeInterval,
		nil,   // TLS transport config
		false, // allow insecure advertise
		"",    // label
		"",    // name
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = peer.Leave(time.Second) })
	return peer
}

func TestClusterWait(t *testing.T) {
	peer := newTestPeer(t)

	const timeout = 100 * time.Millisecond
	wait := clusterWait(peer, timeout)

	// A freshly created single-node peer has position 0, so its wait is
	// zero base timeouts; in all cases it is a non-negative multiple of
	// the base timeout.
	got := wait()
	require.GreaterOrEqual(t, got, time.Duration(0))
	require.Equal(t, time.Duration(peer.Position())*timeout, got)
}
