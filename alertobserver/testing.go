// Copyright 2023 Prometheus Team
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

package alertobserver

import (
	"sync"

	"github.com/prometheus/alertmanager/types"
)

type FakeLifeCycleObserver struct {
	AlertsPerEvent      map[string][]*types.Alert
	PipelineStageAlerts map[string][]*types.Alert
	MetaPerEvent        map[string][]AlertEventMeta
	Mtx                 sync.RWMutex
}

func (o *FakeLifeCycleObserver) Observe(event string, alerts []*types.Alert, meta AlertEventMeta) {
	o.Mtx.Lock()
	defer o.Mtx.Unlock()
	if event == EventAlertPipelinePassStage {
		o.PipelineStageAlerts[meta["stageName"].(string)] = append(o.PipelineStageAlerts[meta["stageName"].(string)], alerts...)
	} else {
		o.AlertsPerEvent[event] = append(o.AlertsPerEvent[event], alerts...)
	}
	o.MetaPerEvent[event] = append(o.MetaPerEvent[event], meta)
}

func NewFakeLifeCycleObserver() *FakeLifeCycleObserver {
	return &FakeLifeCycleObserver{
		PipelineStageAlerts: map[string][]*types.Alert{},
		AlertsPerEvent:      map[string][]*types.Alert{},
		MetaPerEvent:        map[string][]AlertEventMeta{},
	}
}
