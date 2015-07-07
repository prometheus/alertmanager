package manager

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"

	"github.com/prometheus/alertmanager/crdt"
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

	Iter() <-chan *Alert
}

type ConfigState interface {
	Set(*Config) error
	Get() (*Config, error)
}

type NotifyState interface {
}

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
	alerts   *crdtAlerts
	config   *memConfig
}

func NewSimpleState() State {
	state := &simpleState{
		silences: &memSilences{
			sils:   map[string]*Silence{},
			nextID: 1,
		},
		alerts: newCRDTAlerts(crdt.NewMemStorage()),
		// alerts: &memAlerts{
		// 	alerts:  map[model.Fingerprint]*Alert{},
		// 	updates: make(chan *Alert, 100),
		// },
		config: &memConfig{},
	}

	go state.alerts.run()

	return state
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

type crdtAlerts struct {
	set crdt.Set

	updates chan *Alert
	subs    []chan *Alert
	mtx     sync.RWMutex
}

func newCRDTAlerts(storage crdt.Storage) *crdtAlerts {
	return &crdtAlerts{
		set:     crdt.NewLWW(storage),
		updates: make(chan *Alert, 100),
	}
}

func (s *crdtAlerts) run() {
	for a := range s.updates {
		s.mtx.RLock()

		for _, sub := range s.subs {
			select {
			case <-time.After(100 * time.Millisecond):
				log.Errorf("dropped alert %s for subscriber", a)
			case sub <- a:
				// Success
			}
		}

		s.mtx.RUnlock()
	}
}

func (s *crdtAlerts) Add(alerts ...*Alert) error {
	for _, a := range alerts {
		err := s.set.Add(a.Fingerprint().String(), uint64(a.Timestamp.UnixNano()/1e6), a)
		if err != nil {
			return err
		}

		s.updates <- a
	}
	return nil
}

func (s *crdtAlerts) Get(fp model.Fingerprint) (*Alert, error) {
	e, err := s.set.Get(fp.String())
	if err != nil {
		return nil, err
	}

	alert := e.Value.(*Alert)

	return alert, nil
}

func (s *crdtAlerts) GetAll() ([]*Alert, error) {
	list, err := s.set.List()
	if err != nil {
		return nil, err
	}

	var alerts []*Alert
	for _, e := range list {
		alerts = append(alerts, e.Value.(*Alert))
	}
	return alerts, nil
}

func (s *crdtAlerts) Iter() <-chan *Alert {
	ch := make(chan *Alert, 100)

	// As we append the channel to the subcription channels
	// before sending the current list of all alerts, no alert is lost.
	// Handling the some alert twice is effectively a noop.
	s.mtx.Lock()
	s.subs = append(s.subs, ch)
	s.mtx.Unlock()

	prev, err := s.GetAll()
	if err != nil {
		log.Error(err)
	}

	go func() {
		for _, alert := range prev {
			ch <- alert
		}
	}()

	return ch
}

type memAlerts struct {
	alerts  map[model.Fingerprint]*Alert
	updates chan *Alert
	subs    []chan *Alert
	mtx     sync.RWMutex
}

func (s *memAlerts) run() {
	for a := range s.updates {
		s.mtx.RLock()

		for _, sub := range s.subs {
			select {
			case <-time.After(100 * time.Millisecond):
				log.Errorf("dropped alert %s for subscriber", a)
			case sub <- a:
				// Success
			}
		}

		s.mtx.RUnlock()
	}
}

func (s *memAlerts) GetAll() ([]*Alert, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	alerts := make([]*Alert, 0, len(s.alerts))
	for _, a := range s.alerts {
		alerts = append(alerts, a)
	}

	// TODO(fabxc): specify whether time sorting is an interface
	// requirement.
	sort.Sort(alertTimeline(alerts))

	return alerts, nil
}

func (s *memAlerts) Add(alerts ...*Alert) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, alert := range alerts {
		fp := alert.Fingerprint()

		// Last write wins.
		if prev, ok := s.alerts[fp]; !ok || !prev.Timestamp.After(alert.Timestamp) {
			s.alerts[fp] = alert
		}

		s.updates <- alert
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

func (s *memAlerts) Del(fp model.Fingerprint) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	delete(s.alerts, fp)
	return nil
}

func (s *memAlerts) Iter() <-chan *Alert {
	ch := make(chan *Alert, 100)

	s.mtx.Lock()
	s.subs = append(s.subs, ch)
	s.mtx.Unlock()

	go func() {
		prev, _ := s.GetAll()

		for _, alert := range prev {
			ch <- alert
		}
	}()

	return ch
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
