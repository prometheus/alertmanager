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

package manager

import (
	"github.com/prometheus/common/model"
)

// AlertProvider gives access to a set of alerts.
type AlertProvider interface {
	// Iter returns a channel on which all active alerts from the
	// beginning of time are sent. They are not guaranteed to be in
	// chronological order.
	IterActive() <-chan *Alert
	// Get returns the alert for a given fingerprint.
	Get(model.Fingerprint) (*Alert, error)
	// Put adds the given alert to the set.
	Put(*Alert) error
	// Del deletes the alert for the given fingerprint.
	Del(model.Fingerprint) error
}

// SilenceProvider gives access to silences.
type SilenceProvider interface {
	Silencer

	// All returns all existing silences.
	All() []*Silence
	// Set a new silence.
	Set(*Silence) error
	// Del removes a silence.
	Del(model.Fingerprint) error
	// Get a silence associated with a fingerprint.
	Get(model.Fingerprint) (*Silence, error)
}

type ConfigProvider interface {
	// Reload initiates a configuration reload.
	Reload() error
	// Get returns the current configuration.
	Get() *Config
}
