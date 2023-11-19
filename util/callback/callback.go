// Copyright 2019 Prometheus Team
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

package callback

import (
	open_api_models "github.com/prometheus/alertmanager/api/v2/models"
)

type Callback interface {
	// V2GetAlertsCallback is called before v2 getAlerts api returned.
	V2GetAlertsCallback(alerts open_api_models.GettableAlerts) (open_api_models.GettableAlerts, error)

	// V2GetAlertGroupsCallback is called before v2 GetAlertGroups api returned.
	V2GetAlertGroupsCallback(alertgroups open_api_models.AlertGroups) (open_api_models.AlertGroups, error)
}

type NoopAPICallback struct{}

func (n NoopAPICallback) V2GetAlertsCallback(alerts open_api_models.GettableAlerts) (open_api_models.GettableAlerts, error) {
	return alerts, nil
}

func (n NoopAPICallback) V2GetAlertGroupsCallback(alertgroups open_api_models.AlertGroups) (open_api_models.AlertGroups, error) {
	return alertgroups, nil
}
