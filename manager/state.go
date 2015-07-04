package manager

import (
	"fmt"
	"sync"

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
	Get(model.Fingerprint) (*Alert, error)
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
			sils:   map[string]*Silence{},
			nextID: 1,
		},
		alerts: &memAlerts{
			alerts: map[model.Fingerprint]*Alert{},
			ch:     make(chan *Alert, 100),
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
	alerts map[model.Fingerprint]*Alert
	ch     chan *Alert
	mtx    sync.RWMutex
}

func (s *memAlerts) GetAll() ([]*Alert, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	alerts := make([]*Alert, len(s.alerts))
	for i, a := range s.alerts {
		alerts[i] = a
	}

	return alerts, nil
}

func (s *memAlerts) Add(alerts ...*Alert) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, alert := range alerts {
		fp := alert.Fingerprint()

		// Last write wins.
		if prev, ok := s.alerts[fp]; !ok || alert.Timestamp.After(prev.Timestamp) {
			s.alerts[fp] = alert
		}

		// TODO(fabxc): remove this as it blocks if the channel is full.
		s.ch <- alert
	}

	return nil
}

func (s *memAlerts) Get(fp model.Fingerprint) (*Alert, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if a, ok := s.alerts[fp]; ok {
		return a, nil
	}

	return nil, fmt.Errorf("alert with fingerprint %s does not exist", fp)
}

func (s *memAlerts) Next() *Alert {
	return <-s.ch
}

type memSilences struct {
	sils map[string]*Silence

	mtx    sync.RWMutex
	nextID uint64
}

func (s *memSilences) genID() string {
	sid := fmt.Sprintf("%x", s.nextID)
	s.nextID++
	return sid
}

func (s *memSilences) Get(sid string) (*Silence, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if sil, ok := s.sils[sid]; ok {
		return sil, nil
	}

	return nil, fmt.Errorf("silence with ID %s does not exist", sid)
}

func (s *memSilences) Del(sid string) error {
	if _, ok := s.sils[sid]; !ok {
		return fmt.Errorf("silence with ID %s does not exist", sid)
	}

	delete(s.sils, sid)
	return nil
}

func (s *memSilences) GetAll() ([]*Silence, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	sils := make([]*Silence, 0, len(s.sils))
	for _, sil := range s.sils {
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

	s.sils[sil.ID] = sil
	return nil
}
