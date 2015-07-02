package manager

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	// "github.com/prometheus/log"
)

// A State serves the Alertmanager's internal state about active silences.
type State interface {
	Silence() SilenceState
	Config() ConfigState
	// Notify() NotifyState
	Alert() AlertState
}

type AlertState interface {
	Add(...*Alert) error
	GetAll() ([]*Alert, error)

	Next() *Alert
}

type ConfigState interface {
	Set(*Config) error
	Get() (*Config, error)
}

type NotifyState interface{}

type SilenceState interface {
	// Silences returns a list of all silences.
	GetAll() ([]*Silence, error)

	// SetSilence sets the given silence.
	Set(*Silence) error
	Del(sid string) error
	Get(sid string) (*Silence, error)
}

// simpleState implements the State interface based on in-memory storage.
type simpleState struct {
	silences *memSilences
	alerts   *memAlerts
	config   *memConfig
}

func NewSimpleState() State {
	return &simpleState{
		silences: &memSilences{
			m:      map[string]*Silence{},
			nextID: 1,
		},
		alerts: &memAlerts{
			ch: make(chan *Alert, 100),
		},
		config: &memConfig{},
	}
}

func (s *simpleState) Alert() AlertState {
	return s.alerts
}

func (s *simpleState) Silence() SilenceState {
	return s.silences
}

func (s *simpleState) Config() ConfigState {
	return s.config
}

type memConfig struct {
	config *Config
	mtx    sync.RWMutex
}

func (c *memConfig) Set(conf *Config) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.config = conf
	return nil
}

func (c *memConfig) Get() (*Config, error) {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	return c.config, nil
}

type memAlerts struct {
	alerts []*Alert
	ch     chan *Alert
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

	// TODO(fabxc): remove this as it blocks if the channel is full.
	for _, alert := range alerts {
		s.ch <- alert
	}
	return nil
}

func (s *memAlerts) Next() *Alert {
	return <-s.ch
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
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels model.LabelSet `json:"labels"`

	// Extra key/value information which is not used for aggregation.
	Payload map[string]string `json:"payload"`

	// Short summary of alert.
	Summary string `json:"summary"`

	// Long description of alert.
	Description string `json:"description"`

	// Runbook link or reference for the alert.
	Runbook string `json:"runbook"`

	// When the alert was reported.
	Timestamp time.Time `json:"-"`
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
