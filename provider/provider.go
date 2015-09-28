// Copyright 2015 Prometheus Team
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

package provider

import (
	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

type Iterator interface {
	Err() error
	Close()
}

type AlertIterator interface {
	Iterator
	Next() <-chan *types.Alert
}

// Alerts gives access to a set of alerts.
type Alerts interface {
	// IterActive returns an iterator over active alerts from the
	// beginning of time. They are not guaranteed to be in chronological order.
	IterActive() AlertIterator
	// All returns a list of all existing alerts.
	// TODO(fabxc): this is not a scalable solution
	All() ([]*types.Alert, error)
	// Get returns the alert for a given fingerprint.
	Get(model.Fingerprint) (*types.Alert, error)
	// Put adds the given alert to the set.
	Put(...*types.Alert) error
}

// Silences gives access to silences.
type Silences interface {
	// The Silences provider must implement the Muter interface
	// for all its silences. The data provider may have access to an
	// optimized view of the data to perform this evaluation.
	types.Muter

	// All returns all existing silences.
	All() ([]*types.Silence, error)
	// Set a new silence.
	Set(*types.Silence) error
	// Del removes a silence.
	Del(model.Fingerprint) error
	// Get a silence associated with a fingerprint.
	Get(model.Fingerprint) (*types.Silence, error)
}

// Notifies provides information about pending and successful
// notifications.
type Notifies interface {
	Get(dest string, fps ...model.Fingerprint) ([]*types.Notify, error)
	// Set several notifies at once. All or none must succeed.
	Set(dest string, ns ...*types.Notify) error
}

// Reloadable is a component that can change its state based
// on a new configuration.
type Reloadable interface {
	ApplyConfig(*config.Config)
}

type Config interface {
	// Reload initiates a configuration reload.
	Reload(...Reloadable) error
}
