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
	"log"
	"sync"
	"time"
	"container/heap"
)

type Callback func()

// AlertStore stores Alerts and removes them upon expiry.
type AlertStore interface {
	// Ingests a new alert entry into the store. If an alert with the same
	// fingerprint already exists, it only updates the existing entry's metadata.
	Receive(Alerts)
	// Retrieves all alerts from the store that match the provided Filters.
	GetAll(Filters) AlertAggregates
	// Retrieves a specific alert by its fingerprint.
	Get(AlertFingerprint) *AlertAggregate
	// Sets an AlertReceiverNode to notify of any additions/removals of alerts.
	SetOutputNode(AlertReceiverNode)
	// Sets the AggregationRules to associate with alerts.
	SetAggregationRules(AggregationRules)
	// Records that a notification for the specified alert has occurred.
	RecordNotification(*Alert)
	// Runs the AlertStore maintenance loop. This e.g. removes expired alerts.
	Run()
}

type AggregationRules []*AggregationRule

// AggregationRule creates and manages the scope for received events.
type AggregationRule struct {
	Filters                Filters
	RepeatRate             time.Duration
	NotificationConfigName string
}

func (r *AggregationRule) Handles(l *Alert) bool {
	return r.Filters.Handles(l.Labels)
}

type AlertAggregate struct {
	Alert *Alert
	Rule *AggregationRule

	// When was this AggregationInstance created?
	Created time.Time
	// When was the last refresh received into this AlertAggregate?
	LastRefreshed time.Time
	// When was the last notification sent out for this AlertAggregate?
	LastNotification time.Time
}

func (agg *AlertAggregate) Ingest(a *Alert) {
	agg.Alert = a
	agg.LastRefreshed = time.Now()
}

type AlertAggregates []*AlertAggregate

func (aggs AlertAggregates) Len() int {
	return len(aggs)
}

func (aggs AlertAggregates) Less(i, j int) bool {
	return aggs[i].LastRefreshed.Before(aggs[j].LastRefreshed)
}

func (aggs AlertAggregates) Swap(i, j int) {
	aggs[i], aggs[j] = aggs[j], aggs[i]
}

func (aggs AlertAggregates) Push(agg interface{}) {
	// TODO: check whether this needs to use *AlertAggregates.
	aggs = append(aggs, agg.(*AlertAggregate))
}

func (aggs AlertAggregates) Pop() interface{} {
	// TODO: check whether this needs to use *AlertAggregates.
	n := len(aggs)
	head, aggs := aggs[n-1], aggs[:n-1]
	return head
}

type memoryAlertStore struct {
	mu sync.Mutex

	rules      AggregationRules
	aggregates map[AlertFingerprint]*AlertAggregate
	aggregatesRefreshIndex AlertAggregates
	minRefreshInterval time.Duration
	output AlertReceiverNode
}

func NewMemoryAlertStore(ri time.Duration) AlertStore {
	return &memoryAlertStore{
		aggregates: make(map[AlertFingerprint]*AlertAggregate),
		minRefreshInterval: ri,
	}
}

func (s *memoryAlertStore) Receive(as Alerts) {
	s.mu.Lock()
	defer s.mu.Unlock()

	needsRefresh := false
	for _, a := range as {
		if s.ingest(a) {
			needsRefresh = true
		}
	}

	heap.Init(s.aggregatesRefreshIndex)

	if needsRefresh {
		s.refreshOutput()
	}
}

func (s *memoryAlertStore) ingest(a *Alert) (needsRefresh bool) {
	fp := a.Fingerprint()
	agg, ok := s.aggregates[fp]
	if !ok {
		agg = &AlertAggregate{
			Created: time.Now(),
		}
		s.aggregates[fp] = agg
		heap.Push(s.aggregatesRefreshIndex, agg)
	}
	agg.Ingest(a)

	return !ok
}

func (s memoryAlertStore) GetAll(f Filters) AlertAggregates {
	s.mu.Lock()
	defer s.mu.Unlock()

	aggs := make(AlertAggregates, 0, len(s.aggregates))
	for _, agg := range s.aggregates {
		if f.Handles(agg.Alert.Labels) {
			aggs = append(aggs, agg)
		}
	}
	return aggs
}

func (s memoryAlertStore) Get(fp AlertFingerprint) *AlertAggregate {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a deep copy of the AggregationRule so we can safely pass it to the
	// outside.
	agg := s.aggregates[fp]
	if agg.Rule != nil {
		rule := *agg.Rule
		agg.Rule = &rule
	}
	alert := *agg.Alert
	agg.Alert = &alert
	return s.aggregates[fp]
}

func (s *memoryAlertStore) SetOutputNode(n AlertReceiverNode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.output = n
}

func (s *memoryAlertStore) SetAggregationRules(rules AggregationRules) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("Replacing aggregator rules (old: %d, new: %d)...", len(s.rules), len(rules))
	s.rules = rules

	// Reassign AlertAggregates to the first new matching rule, set the rule to
	// nil if there is no matching rule.
	for _, agg := range s.aggregates {
		agg.Rule = nil

		for _, r := range s.rules {
			if r.Handles(agg.Alert) {
				agg.Rule = r
				break
			}
		}
	}
}

func (s *memoryAlertStore) RecordNotification(a *Alert) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fp := a.Fingerprint()
	if agg, ok := s.aggregates[fp]; !ok {
		return
	} else {
		agg.LastNotification = time.Now()
	}
}

func (s *memoryAlertStore) removeExpiredAggregates() {
	s.mu.Lock()
	defer s.mu.Unlock()

	needsRefresh := false
	for {
		agg := heap.Pop(s.aggregatesRefreshIndex).(*AlertAggregate)
		if time.Since(agg.LastRefreshed) > s.minRefreshInterval {
			delete(s.aggregates, agg.Alert.Fingerprint())
			needsRefresh = true
		} else {
			heap.Push(s.aggregatesRefreshIndex, agg)
			return
		}
	}
	if needsRefresh {
		s.refreshOutput()
	}
}

func (s *memoryAlertStore) removeAggregate(fp AlertFingerprint) {
	delete(s.aggregates, fp)
}

// refreshOutput needs to be called with lock held.
func (s *memoryAlertStore) refreshOutput() {
	l := make([]AlertLabels, len(s.aggregates))
	for _, agg := range s.aggregates {
		l = append(l, agg.Alert.Labels)
	}

	s.output.SetInput(l)
}

func (s *memoryAlertStore) Run() {
	expiryTicker := time.NewTicker(time.Second)
	for _ = range expiryTicker.C {
		s.removeExpiredAggregates()
	}
}
