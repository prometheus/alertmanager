// Copyright 2022 Prometheus Team
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

package notify

type Receiver struct {
	groupName    string
	integrations []*Integration

	// A receiver is considered active if a route is using it.
	active bool
}

func (r *Receiver) Name() string {
	return r.groupName
}

func (r *Receiver) Active() bool {
	return r.active
}

func (r *Receiver) Integrations() []*Integration {
	return r.integrations
}

func NewReceiver(name string, active bool, integrations []*Integration) *Receiver {
	return &Receiver{
		groupName:    name,
		active:       active,
		integrations: integrations,
	}
}
