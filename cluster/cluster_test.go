// Copyright 2018 Prometheus Team
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
	"context"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/hashicorp/go-sockaddr"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/client_golang/prometheus"
)

func TestClusterJoinAndReconnect(t *testing.T) {
	ip, _ := sockaddr.GetPrivateIP()
	if ip == "" {
		t.Skipf("skipping tests because no private IP address can be found")
		return
	}
	t.Run("TestJoinLeave", testJoinLeave)
	t.Run("TestReconnect", testReconnect)
	t.Run("TestRemoveFailedPeers", testRemoveFailedPeers)
	t.Run("TestInitiallyFailingPeers", testInitiallyFailingPeers)
}

func testJoinLeave(t *testing.T) {
	logger := log.NewNopLogger()
	p, err := Create(
		logger,
		prometheus.NewRegistry(),
		"0.0.0.0:0",
		"",
		[]string{},
		true,
		DefaultPushPullInterval,
		DefaultGossipInterval,
		DefaultTcpTimeout,
		DefaultProbeTimeout,
		DefaultProbeInterval,
	)
	require.NoError(t, err)
	require.NotNil(t, p)
	err = p.Join(
		DefaultReconnectInterval,
		DefaultReconnectTimeout,
	)
	require.NoError(t, err)
	require.False(t, p.Ready())
	require.Equal(t, p.Status(), "settling")
	go p.Settle(context.Background(), 0*time.Second)
	p.WaitReady()
	require.Equal(t, p.Status(), "ready")

	// Create the peer who joins the first.
	p2, err := Create(
		logger,
		prometheus.NewRegistry(),
		"0.0.0.0:0",
		"",
		[]string{p.Self().Address()},
		true,
		DefaultPushPullInterval,
		DefaultGossipInterval,
		DefaultTcpTimeout,
		DefaultProbeTimeout,
		DefaultProbeInterval,
	)
	require.NoError(t, err)
	require.NotNil(t, p2)
	err = p2.Join(
		DefaultReconnectInterval,
		DefaultReconnectTimeout,
	)
	require.NoError(t, err)
	go p2.Settle(context.Background(), 0*time.Second)

	require.Equal(t, 2, p.ClusterSize())
	p2.Leave(0 * time.Second)
	require.Equal(t, 1, p.ClusterSize())
	require.Equal(t, 1, len(p.failedPeers))
	require.Equal(t, p2.Self().Address(), p.peers[p2.Self().Address()].Node.Address())
	require.Equal(t, p2.Name(), p.failedPeers[0].Name)
}

func testReconnect(t *testing.T) {
	logger := log.NewNopLogger()
	p, err := Create(
		logger,
		prometheus.NewRegistry(),
		"0.0.0.0:0",
		"",
		[]string{},
		true,
		DefaultPushPullInterval,
		DefaultGossipInterval,
		DefaultTcpTimeout,
		DefaultProbeTimeout,
		DefaultProbeInterval,
	)
	require.NoError(t, err)
	require.NotNil(t, p)
	err = p.Join(
		DefaultReconnectInterval,
		DefaultReconnectTimeout,
	)
	require.NoError(t, err)
	go p.Settle(context.Background(), 0*time.Second)
	p.WaitReady()

	p2, err := Create(
		logger,
		prometheus.NewRegistry(),
		"0.0.0.0:0",
		"",
		[]string{},
		true,
		DefaultPushPullInterval,
		DefaultGossipInterval,
		DefaultTcpTimeout,
		DefaultProbeTimeout,
		DefaultProbeInterval,
	)
	require.NoError(t, err)
	require.NotNil(t, p2)
	err = p2.Join(
		DefaultReconnectInterval,
		DefaultReconnectTimeout,
	)
	require.NoError(t, err)
	go p2.Settle(context.Background(), 0*time.Second)
	p2.WaitReady()

	p.peerJoin(p2.Self())
	p.peerLeave(p2.Self())

	require.Equal(t, 1, p.ClusterSize())
	require.Equal(t, 1, len(p.failedPeers))

	p.reconnect()

	require.Equal(t, 2, p.ClusterSize())
	require.Equal(t, 0, len(p.failedPeers))
	require.Equal(t, StatusAlive, p.peers[p2.Self().Address()].status)
}

func testRemoveFailedPeers(t *testing.T) {
	logger := log.NewNopLogger()
	p, err := Create(
		logger,
		prometheus.NewRegistry(),
		"0.0.0.0:0",
		"",
		[]string{},
		true,
		DefaultPushPullInterval,
		DefaultGossipInterval,
		DefaultTcpTimeout,
		DefaultProbeTimeout,
		DefaultProbeInterval,
	)
	require.NoError(t, err)
	require.NotNil(t, p)
	err = p.Join(
		DefaultReconnectInterval,
		DefaultReconnectTimeout,
	)
	require.NoError(t, err)
	n := p.Self()

	now := time.Now()
	p1 := peer{
		status:    StatusFailed,
		leaveTime: now,
		Node:      n,
	}
	p2 := peer{
		status:    StatusFailed,
		leaveTime: now.Add(-time.Hour),
		Node:      n,
	}
	p3 := peer{
		status:    StatusFailed,
		leaveTime: now.Add(30 * -time.Minute),
		Node:      n,
	}
	p.failedPeers = []peer{p1, p2, p3}

	p.removeFailedPeers(30 * time.Minute)
	require.Equal(t, 1, len(p.failedPeers))
	require.Equal(t, p1, p.failedPeers[0])
}

func testInitiallyFailingPeers(t *testing.T) {
	logger := log.NewNopLogger()
	myAddr := "1.2.3.4:5000"
	peerAddrs := []string{myAddr, "2.3.4.5:5000", "3.4.5.6:5000", "foo.example.com:5000"}
	p, err := Create(
		logger,
		prometheus.NewRegistry(),
		"0.0.0.0:0",
		"",
		[]string{},
		true,
		DefaultPushPullInterval,
		DefaultGossipInterval,
		DefaultTcpTimeout,
		DefaultProbeTimeout,
		DefaultProbeInterval,
	)
	require.NoError(t, err)
	require.NotNil(t, p)
	err = p.Join(
		DefaultReconnectInterval,
		DefaultReconnectTimeout,
	)
	require.NoError(t, err)

	p.setInitialFailed(peerAddrs, myAddr)

	// We shouldn't have added "our" bind addr and the FQDN address to the
	// failed peers list.
	require.Equal(t, len(peerAddrs)-2, len(p.failedPeers))
	for _, addr := range peerAddrs {
		if addr == myAddr || addr == "foo.example.com:5000" {
			continue
		}

		pr, ok := p.peers[addr]
		require.True(t, ok)
		require.Equal(t, StatusFailed.String(), pr.status.String())
		require.Equal(t, addr, pr.Address())
		expectedLen := len(p.failedPeers) - 1
		p.peerJoin(pr.Node)
		require.Equal(t, expectedLen, len(p.failedPeers))
	}
}
