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

package common

import "fmt"

// MuteAction decides what a receiver is told about an alert group that muting
// has emptied, so that an integration which deduplicates can close what it
// opened rather than leaving it open for as long as the mute lasts.
//
// The actions nest: MuteActionTreatMuteAsResolved delivers everything
// MuteActionSendResolvedWhenMuted does, and the close of a group that is still
// firing on top of it.
type MuteAction string

const (
	// MuteActionIgnore says nothing about a muted group, which is how
	// Alertmanager has always behaved. It is the default.
	MuteActionIgnore MuteAction = "ignore"
	// MuteActionSendResolvedWhenMuted delivers the group as resolved once every
	// alert in it has resolved, even though muting kept the receiver from ever
	// being shown them. A group still holding a firing alert is not resolved.
	MuteActionSendResolvedWhenMuted MuteAction = "send_resolved_when_muted"
	// MuteActionTreatMuteAsResolved delivers the group as resolved as soon as
	// every alert in it is muted, even if those alerts are still firing.
	MuteActionTreatMuteAsResolved MuteAction = "treat_mute_as_resolved"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *MuteAction) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	switch MuteAction(s) {
	case "", MuteActionIgnore, MuteActionSendResolvedWhenMuted, MuteActionTreatMuteAsResolved:
		*a = MuteAction(s)
		return nil
	default:
		return fmt.Errorf("invalid mute_action %q: expected %q, %q or %q", s,
			MuteActionIgnore, MuteActionSendResolvedWhenMuted, MuteActionTreatMuteAsResolved)
	}
}

// NotifierConfig contains base options common across all notifier configurations.
type NotifierConfig struct {
	VSendResolved bool `yaml:"send_resolved" json:"send_resolved"`
	// VMuteAction decides what this receiver is told about a group that muting
	// has emptied. It needs the muted-alerts-in-nflog feature, which is what
	// lets the notification pipeline see such a group at all.
	VMuteAction MuteAction `yaml:"mute_action,omitempty" json:"mute_action,omitempty"`
}

func (nc *NotifierConfig) SendResolved() bool {
	return nc.VSendResolved
}

// SendsResolvedWhenMuted implements the notify.MuteActioner interface.
func (nc *NotifierConfig) SendsResolvedWhenMuted() bool {
	return nc.VMuteAction == MuteActionSendResolvedWhenMuted || nc.VMuteAction == MuteActionTreatMuteAsResolved
}

// TreatsMuteAsResolved implements the notify.MuteActioner interface.
func (nc *NotifierConfig) TreatsMuteAsResolved() bool {
	return nc.VMuteAction == MuteActionTreatMuteAsResolved
}

// Validator is the interface that wraps the Validate method for notifier
// configurations. Notifier config types implement this interface to provide
// per-type validation logic that can be called independently of YAML unmarshaling.
type Validator interface {
	Validate() error
}
