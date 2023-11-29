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
	"github.com/prometheus/alertmanager/types"
)

const (
	EventAlertReceived             string = "received"
	EventAlertRejected             string = "rejected"
	EventAlertAddedToAggrGroup     string = "addedAggrGroup"
	EventAlertFailedAddToAggrGroup string = "failedAddAggrGroup"
	EventAlertPipelineStart        string = "pipelineStart"
	EventAlertPipelinePassStage    string = "pipelinePassStage"
	EventAlertMuted                string = "muted"
	EventAlertSent                 string = "sent"
	EventAlertSendFailed           string = "sendFailed"
)

type AlertEventMeta map[string]interface{}

type LifeCycleObserver interface {
	Observe(event string, alerts []*types.Alert, meta AlertEventMeta)
}
