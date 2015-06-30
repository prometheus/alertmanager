// Copyright 2013 Prometheus Team
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

package manager

import (
	"github.com/prometheus/common/model"
)

// Alert models an action triggered by Prometheus.
type Alert struct {
	// Short summary of alert.
	Summary string `json:"summary"`

	// Long description of alert.
	Description string `json:"description"`

	// Runbook link or reference for the alert.
	Runbook string `json:"runbook"`

	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels model.LabelSet `json:"labels"`

	// Extra key/value information which is not used for aggregation.
	Payload map[string]string `json:"payload"`
}

func (a *Alert) Name() string {
	return string(a.Labels[model.AlertNameLabel])
}

func (a *Alert) Fingerprint() model.Fingerprint {
	return a.Labels.Fingerprint()
}
