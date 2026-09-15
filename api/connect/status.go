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

package apiconnect

import (
	"context"
	"sort"

	"connectrpc.com/connect"
	"github.com/prometheus/common/version"
	"google.golang.org/protobuf/types/known/timestamppb"

	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
)

// Ensure API satisfies the generated StatusService handler interface.
var _ statusv3alphaconnect.StatusServiceHandler = (*API)(nil)

// GetStatus returns the Alertmanager instance and cluster status.
func (api *API) GetStatus(ctx context.Context, _ *connect.Request[statusv3alpha.GetStatusRequest]) (*connect.Response[statusv3alpha.GetStatusResponse], error) {
	var original string
	if snapshot := api.configSnapshot.Load(); snapshot != nil {
		original = *snapshot
	}

	status := &statusv3alpha.AlertmanagerStatus{
		StartTime: timestamppb.New(api.uptime),
		VersionInfo: &statusv3alpha.VersionInfo{
			Version:   version.Version,
			Revision:  version.Revision,
			Branch:    version.Branch,
			BuildUser: version.BuildUser,
			BuildDate: version.BuildDate,
			GoVersion: version.GoVersion,
		},
		Config: &statusv3alpha.AlertmanagerConfig{
			Original: original,
		},
		Cluster: &statusv3alpha.ClusterStatus{
			State: statusv3alpha.ClusterStatus_STATE_DISABLED,
			Peers: []*statusv3alpha.PeerStatus{},
		},
	}

	// If clustering is disabled, api.peer is nil and the cluster is
	// reported as disabled.
	if api.peer != nil {
		clusterStatus, err := api.snapshotClusterStatus(ctx)
		if err != nil {
			return nil, err
		}
		status.Cluster = clusterStatus
	}

	resp := connect.NewResponse(&statusv3alpha.GetStatusResponse{Status: status})
	resp.Header().Set("Cache-Control", "no-store")
	return resp, nil
}

func (api *API) snapshotClusterStatus(ctx context.Context) (*statusv3alpha.ClusterStatus, error) {
	select {
	case api.peerSnapshotSem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-api.peerSnapshotSem
		return nil, err
	}

	result := make(chan *statusv3alpha.ClusterStatus, 1)
	go func() {
		defer func() { <-api.peerSnapshotSem }()
		members := api.peer.Peers()
		peers := make([]*statusv3alpha.PeerStatus, 0, len(members))
		for _, member := range members {
			peers = append(peers, &statusv3alpha.PeerStatus{
				Name:    member.Name(),
				Address: member.Address(),
			})
		}
		sort.Slice(peers, func(i, j int) bool {
			return peers[i].Name < peers[j].Name
		})
		result <- &statusv3alpha.ClusterStatus{
			Name:  api.peer.Name(),
			State: clusterState(api.peer.Status()),
			Peers: peers,
		}
	}()

	select {
	case clusterStatus := <-result:
		return clusterStatus, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// clusterState maps a cluster.ClusterPeer status string onto the proto enum.
func clusterState(s string) statusv3alpha.ClusterStatus_State {
	switch s {
	case "ready":
		return statusv3alpha.ClusterStatus_STATE_READY
	case "settling":
		return statusv3alpha.ClusterStatus_STATE_SETTLING
	default:
		return statusv3alpha.ClusterStatus_STATE_UNSPECIFIED
	}
}
