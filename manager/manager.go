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
	"container/heap"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// AlertManager stores Alerts and removes them upon expiry.
type AlertManager interface {
	// Ingests a new alert entry into the store. If an alert with the same
	// fingerprint already exists, it only updates the existing entry's metadata.
	Receive(Alerts)
	// Retrieves all alerts from the store that match the provided Filters.
	GetAll(Filters) AlertAggregates
	// Sets the AggregationRules to associate with alerts.
	SetAggregationRules(AggregationRules)
	// Runs the AlertManager dispatcher loop.
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
	Rule  *AggregationRule

	// When was this AggregationInstance created?
	Created time.Time
	// When was the last refresh received into this AlertAggregate?
	LastRefreshed time.Time
	// When was the last notification sent out for this AlertAggregate?
	LastNotification time.Time
	// When should the next notification be sent according to the current Rule's
	// RepeatRate?
	NextNotification time.Time
}

func (agg *AlertAggregate) Ingest(a *Alert) {
	agg.Alert = a
	agg.LastRefreshed = time.Now()
}

type AlertAggregates []*AlertAggregate

type aggregatesByLastRefreshed struct {
	AlertAggregates
}

type aggregatesByNextNotification struct {
	AlertAggregates
}

func (aggs AlertAggregates) Len() int {
	return len(aggs)
}

func (aggs aggregatesByLastRefreshed) Less(i, j int) bool {
	return aggs.AlertAggregates[i].LastRefreshed.Before(aggs.AlertAggregates[j].LastRefreshed)
}

func (aggs aggregatesByNextNotification) Less(i, j int) bool {
	return aggs.AlertAggregates[i].NextNotification.Before(aggs.AlertAggregates[j].NextNotification)
}

func (aggs AlertAggregates) Swap(i, j int) {
	aggs[i], aggs[j] = aggs[j], aggs[i]
}

func (aggs *AlertAggregates) Push(agg interface{}) {
	*aggs = append(*aggs, agg.(*AlertAggregate))
}

func (aggs *AlertAggregates) Pop() interface{} {
	old := *aggs
	n := len(old)
	item := old[n-1]
	*aggs = old[:n-1]
	return item
}

type memoryAlertManager struct {
	mu sync.Mutex

	rules                        AggregationRules
	aggregates                   map[AlertFingerprint]*AlertAggregate
	aggregatesByLastRefreshed    aggregatesByLastRefreshed
	aggregatesByNextNotification aggregatesByNextNotification
	filteredCache                AlertLabelSets
	needsNotificationRefresh     bool
	minRefreshInterval           time.Duration

	inhibitor *Inhibitor
	silencer  *Silencer
	notifier  Notifier
}

func NewMemoryAlertManager(ri time.Duration, i *Inhibitor, s *Silencer, n Notifier) AlertManager {
	return &memoryAlertManager{
		aggregates:         make(map[AlertFingerprint]*AlertAggregate),
		minRefreshInterval: ri,

		inhibitor: i,
		silencer:  s,
		notifier:  n,
	}
}

func (s *memoryAlertManager) Receive(as Alerts) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, a := range as {
		s.ingest(a)
	}
}

func (s *memoryAlertManager) ingest(a *Alert) {
	fp := a.Fingerprint()
	agg, ok := s.aggregates[fp]
	if !ok {
		agg = &AlertAggregate{
			Created: time.Now(),
		}
		agg.Ingest(a)

		for _, r := range s.rules {
			if r.Handles(agg.Alert) {
				agg.Rule = r
			}
		}

		s.aggregates[fp] = agg
		heap.Push(&s.aggregatesByLastRefreshed, agg)
		heap.Push(&s.aggregatesByNextNotification, agg)

		s.needsNotificationRefresh = true
	} else {
		agg.Ingest(a)
		heap.Init(&s.aggregatesByLastRefreshed)
	}
}

func (s memoryAlertManager) GetAll(f Filters) AlertAggregates {
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

func (s memoryAlertManager) Get(fp AlertFingerprint) *AlertAggregate {
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

func (s *memoryAlertManager) SetAggregationRules(rules AggregationRules) {
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
				agg.NextNotification = agg.LastNotification.Add(r.RepeatRate)
				break
			}
		}
	}
	heap.Init(&s.aggregatesByNextNotification)
	s.needsNotificationRefresh = true
}

func (s *memoryAlertManager) removeExpiredAggregates() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		if len(s.aggregatesByLastRefreshed.AlertAggregates) == 0 {
			return
		}

		agg := heap.Pop(&s.aggregatesByLastRefreshed).(*AlertAggregate)

		if time.Since(agg.LastRefreshed) > s.minRefreshInterval {
			delete(s.aggregates, agg.Alert.Fingerprint())

			// Also remove the aggregate from the last-notification-time index.
			n := len(s.aggregatesByNextNotification.AlertAggregates)
			i := sort.Search(n, func(i int) bool {
				return !agg.NextNotification.After(s.aggregatesByNextNotification.AlertAggregates[i].NextNotification)
			})
			if i == n {
				panic("Missing alert aggregate in aggregatesByNextNotification index")
			} else {
				for j := i; j < n; j++ {
					if s.aggregatesByNextNotification.AlertAggregates[j] == agg {
						heap.Remove(&s.aggregatesByNextNotification, j)
						break
					}
				}
			}

			s.needsNotificationRefresh = true
		} else {
			heap.Push(&s.aggregatesByLastRefreshed, agg)
			return
		}
	}
}

// Check whether one of the filtered (uninhibited, unsilenced) alerts should
// trigger a new notification.
func (s *memoryAlertManager) checkNotificationRepeats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	f := s.filteredLabelSets(true)
	for _, agg := range s.aggregatesByNextNotification.AlertAggregates {
		for _, fl := range f {
			if agg.Alert.Labels.Equal(fl) && agg.NextNotification.Before(now) {
				s.needsNotificationRefresh = true
				return
			}
		}
	}
}

// Returns all active AlertLabelSets that are neither inhibited nor silenced.
func (s *memoryAlertManager) filteredLabelSets(useCache bool) AlertLabelSets {
	if useCache && s.filteredCache != nil {
		return s.filteredCache
	}

	l := make(AlertLabelSets, 0, len(s.aggregates))
	for _, agg := range s.aggregates {
		l = append(l, agg.Alert.Labels)
	}

	l = s.inhibitor.Filter(l)
	s.filteredCache = s.silencer.Filter(l)
	return s.filteredCache
}

func (s *memoryAlertManager) refreshNotifications() {
	s.mu.Lock()
	s.mu.Unlock()

	s.needsNotificationRefresh = false

	l := s.filteredLabelSets(false)

	numSent := 0
	for _, lb := range l {
		agg := s.aggregates[lb.Fingerprint()]
		if agg.NextNotification.After(time.Now()) {
			continue
		}
		if agg.Rule != nil {
			s.notifier.QueueNotification(agg.Alert, agg.Rule.NotificationConfigName)
			agg.LastNotification = time.Now()
			agg.NextNotification = agg.LastNotification.Add(agg.Rule.RepeatRate)
			numSent++
		}
	}
	if numSent > 0 {
		log.Printf("Sent %d notifications", numSent)
		heap.Init(&s.aggregatesByNextNotification)
	}
}

func (s *memoryAlertManager) refreshNeeded() (bool, []string) {
	s.mu.Lock()
	s.mu.Unlock()

	needsRefresh := false
	reasons := []string{}
	if s.needsNotificationRefresh {
		needsRefresh = true
		reasons = append(reasons, "active alerts have changed")
	}
	if s.inhibitor.HasChanged() {
		needsRefresh = true
		reasons = append(reasons, "inhibit rules have changed")
	}
	if s.silencer.HasChanged() {
		needsRefresh = true
		reasons = append(reasons, "silences have changed")
	}
	return needsRefresh, reasons
}

func (s *memoryAlertManager) runIteration() {
	s.removeExpiredAggregates()
	s.checkNotificationRepeats()
	if refresh, reasons := s.refreshNeeded(); refresh {
		log.Printf("Recomputing notification outputs (%s)", strings.Join(reasons, ", "))
		s.refreshNotifications()
	}
}

func (s *memoryAlertManager) checkSanity() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.aggregates) != len(s.aggregatesByLastRefreshed.AlertAggregates) {
		panic("len(aggregates) != len(aggregatesByLastRefreshed)")
	}
	if len(s.aggregates) != len(s.aggregatesByNextNotification.AlertAggregates) {
		panic("len(aggregates) != len(aggregatesByNextNotification)")
	}
}

func (s *memoryAlertManager) Run() {
	iterationTicker := time.NewTicker(time.Second)
	for _ = range iterationTicker.C {
		s.checkSanity()
		s.runIteration()
	}
}
