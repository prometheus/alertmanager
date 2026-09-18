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
	"maps"
	"slices"

	"connectrpc.com/connect"
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	configcommon "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/dispatch"
)

type receiverMetadata struct {
	name   string
	labels map[string]string
}

type reloadSnapshot struct {
	configuration  string
	routes         *dispatch.Route
	receivers      []receiverMetadata
	setAlertStatus func(context.Context, model.LabelSet)
}

func newReloadSnapshot(cfg *config.Config, setAlertStatus func(context.Context, model.LabelSet)) *reloadSnapshot {
	snapshot := &reloadSnapshot{
		configuration:  cfg.String(),
		receivers:      make([]receiverMetadata, 0, len(cfg.Receivers)),
		setAlertStatus: setAlertStatus,
	}
	if cfg.Route != nil {
		snapshot.routes = dispatch.NewRoute(cloneConfigRoute(cfg.Route), nil)
	}
	for _, receiver := range cfg.Receivers {
		snapshot.receivers = append(snapshot.receivers, receiverMetadata{
			name:   receiver.Name,
			labels: maps.Clone(receiver.Labels),
		})
	}
	return snapshot
}

func cloneConfigRoute(source *config.Route) *config.Route {
	if source == nil {
		return nil
	}
	result := &config.Route{
		Receiver:            source.Receiver,
		GroupByStr:          slices.Clone(source.GroupByStr),
		GroupBy:             slices.Clone(source.GroupBy),
		GroupByAll:          source.GroupByAll,
		Match:               maps.Clone(source.Match),
		MatchRE:             maps.Clone(source.MatchRE),
		Matchers:            make(configcommon.Matchers, len(source.Matchers)),
		MuteTimeIntervals:   slices.Clone(source.MuteTimeIntervals),
		ActiveTimeIntervals: slices.Clone(source.ActiveTimeIntervals),
		Continue:            source.Continue,
		Routes:              make([]*config.Route, 0, len(source.Routes)),
		Labels:              maps.Clone(source.Labels),
	}
	for index, matcher := range source.Matchers {
		if matcher != nil {
			cloned := *matcher
			result.Matchers[index] = &cloned
		}
	}
	for _, route := range source.Routes {
		result.Routes = append(result.Routes, cloneConfigRoute(route))
	}
	if source.GroupWait != nil {
		value := *source.GroupWait
		result.GroupWait = &value
	}
	if source.GroupInterval != nil {
		value := *source.GroupInterval
		result.GroupInterval = &value
	}
	if source.RepeatInterval != nil {
		value := *source.RepeatInterval
		result.RepeatInterval = &value
	}
	return result
}

// Update publishes one immutable view of reload-dependent API data.
func (api *API) Update(cfg *config.Config, setAlertStatus func(context.Context, model.LabelSet)) {
	if cfg == nil {
		api.reloadSnapshot.Store(nil)
		return
	}
	api.reloadSnapshot.Store(newReloadSnapshot(cfg, setAlertStatus))
}

func (api *API) currentReloadSnapshot() (*reloadSnapshot, error) {
	snapshot := api.reloadSnapshot.Load()
	if snapshot == nil {
		return nil, newRPCError(connect.CodeUnavailable, "configuration is not loaded", nil)
	}
	return snapshot, nil
}
