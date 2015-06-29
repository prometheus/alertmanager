// Copyright 2013 Prometheus Team
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"sync"
	"time"

	"github.com/prometheus/log"

	"github.com/prometheus/alertmanager/config"
)

type SilenceID uint
type Silences []*Silence

type Silence struct {
	// The numeric ID of the silence.
	ID SilenceID
	// Name/email of the silence creator.
	CreatedBy string
	// When the silence was first created (Unix timestamp).
	CreatedAt time.Time
	// When the silence expires (Unix timestamp).
	EndsAt time.Time
	// Additional comment about the silence.
	Comment string
	// Filters that determine which alerts are silenced.
	Filters Filters
	// Timer used to trigger the deletion of the Silence after its expiry
	// time.
	expiryTimer *time.Timer
}

type ApiSilence struct {
	ID               SilenceID
	CreatedBy        string
	CreatedAtSeconds int64
	EndsAtSeconds    int64
	Comment          string
	Filters          map[string]string
}

func (s *Silence) MarshalJSON() ([]byte, error) {
	filters := map[string]string{}
	for _, f := range s.Filters {
		name := f.Name
		value := f.ValuePattern
		filters[name] = value
	}

	return json.Marshal(&ApiSilence{
		ID:               s.ID,
		CreatedBy:        s.CreatedBy,
		CreatedAtSeconds: s.CreatedAt.Unix(),
		EndsAtSeconds:    s.EndsAt.Unix(),
		Comment:          s.Comment,
		Filters:          filters,
	})
}

func (s *Silence) UnmarshalJSON(data []byte) error {
	sc := &ApiSilence{}
	json.Unmarshal(data, sc)

	filters := make(Filters, 0, len(sc.Filters))

	for label, value := range sc.Filters {
		re, err := regexp.Compile(value)
		if err != nil {
			return err
		}
		filters = append(filters, NewFilter(label, &config.Regexp{*re}))
	}

	if sc.CreatedAtSeconds == 0 {
		sc.CreatedAtSeconds = time.Now().Unix()
	}
	if sc.EndsAtSeconds == 0 {
		sc.EndsAtSeconds = time.Now().Add(time.Hour).Unix()
	}

	*s = Silence{
		ID:        sc.ID,
		CreatedBy: sc.CreatedBy,
		CreatedAt: time.Unix(sc.CreatedAtSeconds, 0).UTC(),
		EndsAt:    time.Unix(sc.EndsAtSeconds, 0).UTC(),
		Comment:   sc.Comment,
		Filters:   filters,
	}
	return nil
}

func (s Silence) Matches(l AlertLabelSet) bool {
	return s.Filters.Handles(l)
}

type Silencer struct {
	// Silences managed by this Silencer.
	Silences map[SilenceID]*Silence
	// Used to track the next Silence ID to allocate.
	lastID SilenceID
	// Tracks whether silences have changed since the last call to HasChanged.
	dirty bool

	// Mutex to protect the above.
	mu sync.Mutex
}

type IsSilencedInterrogator interface {
	IsSilenced(AlertLabelSet) (bool, *Silence)
}

func NewSilencer() *Silencer {
	return &Silencer{
		Silences: make(map[SilenceID]*Silence),
	}
}

func (s *Silencer) nextSilenceID() SilenceID {
	s.lastID++
	return s.lastID
}

func (s *Silencer) setupExpiryTimer(sc *Silence) {
	if sc.expiryTimer != nil {
		sc.expiryTimer.Stop()
	}
	expDuration := sc.EndsAt.Sub(time.Now())
	sc.expiryTimer = time.AfterFunc(expDuration, func() {
		if err := s.DelSilence(sc.ID); err != nil {
			log.Errorf("Failed to delete silence %d: %s", sc.ID, err)
		}
	})
}

func (s *Silencer) AddSilence(sc *Silence) SilenceID {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dirty = true

	if sc.ID == 0 {
		sc.ID = s.nextSilenceID()
	} else {
		if sc.ID > s.lastID {
			s.lastID = sc.ID
		}
	}

	s.setupExpiryTimer(sc)
	s.Silences[sc.ID] = sc
	return sc.ID
}

func (s *Silencer) UpdateSilence(sc *Silence) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dirty = true

	origSilence, ok := s.Silences[sc.ID]
	if !ok {
		return fmt.Errorf("Silence with ID %d doesn't exist", sc.ID)
	}
	if sc.EndsAt != origSilence.EndsAt {
		origSilence.expiryTimer.Stop()
	}
	*origSilence = *sc
	s.setupExpiryTimer(origSilence)
	return nil
}

func (s *Silencer) GetSilence(id SilenceID) (*Silence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sc, ok := s.Silences[id]
	if !ok {
		return nil, fmt.Errorf("Silence with ID %d doesn't exist", id)
	}
	return sc, nil
}

func (s *Silencer) DelSilence(id SilenceID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dirty = true

	if _, ok := s.Silences[id]; !ok {
		return fmt.Errorf("Silence with ID %d doesn't exist", id)
	}
	delete(s.Silences, id)
	return nil
}

func (s *Silencer) SilenceSummary() Silences {
	s.mu.Lock()
	defer s.mu.Unlock()

	silences := make(Silences, 0, len(s.Silences))
	for _, sc := range s.Silences {
		silences = append(silences, sc)
	}
	return silences
}

func (s *Silencer) IsSilenced(l AlertLabelSet) (bool, *Silence) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, s := range s.Silences {
		if s.Matches(l) {
			return true, s
		}
	}
	return false, nil
}

// Returns only those AlertLabelSets which are not matched by any silence.
func (s *Silencer) Filter(l AlertLabelSets) AlertLabelSets {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := l
	for _, sc := range s.Silences {
		unsilenced := AlertLabelSets{}
		for _, labels := range out {
			if !sc.Matches(labels) {
				unsilenced = append(unsilenced, labels)
			}
		}
		out = unsilenced
	}
	return out
}

// Loads a JSON representation of silences from a file.
func (s *Silencer) LoadFromFile(fileName string) error {
	silenceJson, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	silences := Silences{}
	if err = json.Unmarshal(silenceJson, &silences); err != nil {
		return err
	}
	for _, sc := range silences {
		s.AddSilence(sc)
	}
	return nil
}

// Saves a JSON representation of silences to a file.
func (s *Silencer) SaveToFile(fileName string) error {
	silenceSummary := s.SilenceSummary()

	resultBytes, err := json.Marshal(silenceSummary)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fileName, resultBytes, 0644)
}

// Returns whether silences have been added/updated/removed since the last call
// to HasChanged.
func (s *Silencer) HasChanged() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	dirty := s.dirty
	s.dirty = false
	return dirty
}

func (s *Silencer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sc := range s.Silences {
		if sc.expiryTimer != nil {
			sc.expiryTimer.Stop()
		}
	}
}
