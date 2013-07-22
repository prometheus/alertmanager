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
	"fmt"
	"log"
	"sync"
	"time"
)

type SuppressionId uint

type Suppression struct {
	// The numeric ID of the suppression.
	Id SuppressionId
	// Name/email of the suppression creator.
	CreatedBy string
	// When the suppression was first created (Unix timestamp).
	CreatedAt time.Time
	// When the suppression expires (Unix timestamp).
	EndsAt time.Time
	// Additional comment about the suppression.
	Comment string
	// Filters that determine which events are suppressed.
	Filters Filters
	// Timer used to trigger the deletion of the Suppression after its expiry
	// time.
	expiryTimer *time.Timer
}

type Suppressions []*Suppression

type Suppressor struct {
	// Suppressions managed by this Suppressor.
	Suppressions map[SuppressionId]*Suppression
	// Used to track the next Suppression Id to allocate.
	nextId SuppressionId

	// Mutex to protect the above.
	mu sync.Mutex
}

type IsInhibitedInterrogator interface {
	IsInhibited(*Event) (bool, *Suppression)
}

func NewSuppressor() *Suppressor {
	return &Suppressor{
		Suppressions: make(map[SuppressionId]*Suppression),
	}
}

func (s *Suppressor) nextSuppressionId() SuppressionId {
	// BUG: Build proper ID management. For now, as we are only keeping
	//      data in memory anyways, this is enough.
	s.nextId++
	return s.nextId
}

func (s *Suppressor) setupExpiryTimer(sup *Suppression) {
	if sup.expiryTimer != nil {
		sup.expiryTimer.Stop()
	}
	expDuration := sup.EndsAt.Sub(time.Now())
	sup.expiryTimer = time.AfterFunc(expDuration, func() {
		if err := s.DelSuppression(sup.Id); err != nil {
			log.Printf("Failed to delete suppression %d: %s", sup.Id, err)
		}
	})
}

func (s *Suppressor) AddSuppression(sup *Suppression) SuppressionId {
	s.mu.Lock()
	defer s.mu.Unlock()

	sup.Id = s.nextSuppressionId()
	s.setupExpiryTimer(sup)
	s.Suppressions[sup.Id] = sup
	return sup.Id
}

func (s *Suppressor) UpdateSuppression(sup *Suppression) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	origSup, ok := s.Suppressions[sup.Id]
	if !ok {
		return fmt.Errorf("Suppression with ID %d doesn't exist", sup.Id)
	}
	if sup.EndsAt != origSup.EndsAt {
		origSup.expiryTimer.Stop()
	}
	*origSup = *sup
	s.setupExpiryTimer(origSup)
	return nil
}

func (s *Suppressor) GetSuppression(id SuppressionId) (*Suppression, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sup, ok := s.Suppressions[id]
	if !ok {
		return nil, fmt.Errorf("Suppression with ID %d doesn't exist", id)
	}
	return sup, nil
}

func (s *Suppressor) DelSuppression(id SuppressionId) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.Suppressions[id]; !ok {
		return fmt.Errorf("Suppression with ID %d doesn't exist", id)
	}
	delete(s.Suppressions, id)
	return nil
}

func (s *Suppressor) SuppressionSummary() Suppressions {
	s.mu.Lock()
	defer s.mu.Unlock()

	suppressions := make(Suppressions, 0, len(s.Suppressions))
	for _, sup := range s.Suppressions {
		suppressions = append(suppressions, sup)
	}
	return suppressions
}

func (s *Suppressor) IsInhibited(e *Event) (bool, *Suppression) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, s := range s.Suppressions {
		if s.Filters.Handles(e) {
			return true, s
		}
	}
	return false, nil
}

func (s *Suppressor) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sup := range s.Suppressions {
		if sup.expiryTimer != nil {
			sup.expiryTimer.Stop()
		}
	}
}
