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

package dispatch

import (
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/types"
)

// AggregationGroup interface is used to expose Aggregation Groups within the dispatcher.
type AggregationGroup interface {
	Alerts() []*types.Alert
	Labels() model.LabelSet
	GroupKey() string
	RouteID() string
}
