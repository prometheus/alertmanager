package manager

import (
	"fmt"
	"sync"

	"github.com/prometheus/common/model"
)

// A State serves the Alertmanager's internal state about active silences.
type State interface {
	Silence() SilenceState
	// Config() ConfigState
	// Notify() NotifyState
	Alert() AlertState
}

type AlertState interface {
	Add(...*Alert) error
	GetAll() ([]*Alert, error)
}

type ConfigState interface{}

type NotifyState interface{}

type SilenceState interface {
	// Silences returns a list of all silences.
	GetAll() ([]*Silence, error)

	// SetSilence sets the given silence.
	Set(*Silence) error
	Del(sid string) error
	Get(sid string) (*Silence, error)
}

// memState implements the State interface based on in-memory storage.
type memState struct {
	silences *memSilences
	alerts   *memAlerts
}

func NewMemState() State {
	return &memState{
		silences: &memSilences{
			m:      map[string]*Silence{},
			nextID: 1,
		},
		alerts: &memAlerts{},
	}
}

func (s *memState) Alert() AlertState {
	return s.alerts
}

func (s *memState) Silence() SilenceState {
	return s.silences
}

type memAlerts struct {
	alerts []*Alert
	mtx    sync.RWMutex
}

func (s *memAlerts) GetAll() ([]*Alert, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	alerts := make([]*Alert, len(s.alerts))
	copy(alerts, s.alerts)

	return alerts, nil
}

func (s *memAlerts) Add(alerts ...*Alert) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.alerts = append(s.alerts, alerts...)
	return nil
}

type memSilences struct {
	m   map[string]*Silence
	mtx sync.RWMutex

	nextID uint64
}

func (s *memSilences) genID() string {
	sid := fmt.Sprintf("%x", s.nextID)
	s.nextID++
	return sid
}

func (s *memSilences) Get(sid string) (*Silence, error) {
	return nil, nil
}
func (s *memSilences) Del(sid string) error {
	if _, ok := s.m[sid]; !ok {
		return fmt.Errorf("silence with ID %s does not exist", sid)
	}
	delete(s.m, sid)
	return nil
}

func (s *memSilences) GetAll() ([]*Silence, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	sils := make([]*Silence, 0, len(s.m))
	for _, sil := range s.m {
		sils = append(sils, sil)
	}
	return sils, nil
}

func (s *memSilences) Set(sil *Silence) error {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if sil.ID == "" {
		sil.ID = s.genID()
	}

	s.m[sil.ID] = sil
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
