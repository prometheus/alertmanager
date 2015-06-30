package main

import (
	"github.com/prometheus/common/model"
)

// Manager handles active alerts
type Manager struct {
	state State
}

func New(s State) *Manager {
	return &Manager{
		state: s,
	}
}

// Run starts the processing of the manager and blocks.
func (m *Manager) Run() {

}

// A State serves the Alertmanager's internal state about active silences.
type State interface {
	AlertState
	ConfigState
	NotifyState
	SilenceState
}

type AlertState interface{}

type ConfigState interface{}

type NotifyState interface{}

type SilenceState interface {
	// Silences returns a list of all silences.
	Silences() ([]*Silence, error)

	// SetSilence sets the given silence.
	SetSilence(*Silence) error
}

// memState implements the State interface based on in-memory storage.
type memState struct {
	silences map[uint64]*Silence
}

func NewMemState() State {
	return &memState{
		silences: map[uint64]*Silence{},
	}
}

func (s *memState) Silences() ([]*Silence, error) {
	sils := make([]*Silence, 0, len(s.silences))
	for _, sil := range s.silences {
		sils = append(sils, sil)
	}
	return sils, nil
}

func (s *memState) SetSilence(sil *Silence) error {
	s.silences[sil.ID] = sil
	return nil
}

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

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return string(a.Labels[model.AlertNameLabel])
}

// Fingerprint returns a unique hash for the alert. It is equivalent to
// the fingerprint of the alert's label set.
func (a *Alert) Fingerprint() model.Fingerprint {
	return a.Labels.Fingerprint()
}
