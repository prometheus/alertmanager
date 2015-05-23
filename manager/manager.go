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
	"strings"
	"sync"
	"time"

	"github.com/prometheus/log"
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

// Returns whether a given AggregationRule matches an Alert.
func (r *AggregationRule) Handles(l *Alert) bool {
	return r.Filters.Handles(l.Labels)
}

// An AlertAggregate tracks the latest alert received for a given alert
// fingerprint and some metadata about the alert.
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

// Ingests a received Alert into this AlertAggregate and updates metadata.
func (agg *AlertAggregate) Ingest(a *Alert) {
	agg.Alert = a
	agg.LastRefreshed = time.Now()
}

type AlertAggregates []*AlertAggregate

// Helper type for managing a heap based on LastRefreshed time.
type aggregatesByLastRefreshed struct {
	AlertAggregates
}

// Helper type for managing a heap based on NextNotification time.
type aggregatesByNextNotification struct {
	AlertAggregates
}

// Methods implementing heap.Interface.
func (aggs AlertAggregates) Len() int {
	return len(aggs)
}

func (aggs aggregatesByLastRefreshed) Less(i, j int) bool {
	return aggs.AlertAggregates[i].LastRefreshed.Before(aggs.AlertAggregates[j].LastRefreshed)
}

func (aggs aggregatesByNextNotification) Less(i, j int) bool {
	return aggs.AlertAggregates[i].NextNotification.Before(aggs.AlertAggregates[j].NextNotification)
}

// rebuildFrom rebuilds the aggregatesByNextNotification index from a provided
// authoritative AlertAggregates slice.
func (aggs *aggregatesByNextNotification) rebuildFrom(aa AlertAggregates) {
	aggs.AlertAggregates = aggs.AlertAggregates[:0]
	for _, a := range aa {
		aggs.Push(a)
	}
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

// memoryAlertManager implements the AlertManager interface and only keeps
// state in memory.
type memoryAlertManager struct {
	// The minimum interval for alert refreshes before being purged.
	minRefreshInterval time.Duration
	// Inhibitor for filtering out inhibited alerts.
	inhibitor *Inhibitor
	// Silencer for filtering out silenced alerts.
	silencer *Silencer
	// Notifier for dispatching notifications.
	notifier Notifier

	// Mutex protecting all fields below.
	mu sync.Mutex
	// Currently loaded set of AggregationRules.
	rules AggregationRules
	// Main AlertAggregates index by fingerprint.
	aggregates map[AlertFingerprint]*AlertAggregate
	// Secondary AlertAggregates index by LastRefreshed time.
	aggregatesByLastRefreshed aggregatesByLastRefreshed
	// Secondary AlertAggregates index by NextNotification time.
	aggregatesByNextNotification aggregatesByNextNotification
	// Cache of the last result of computing uninhibited/unsilenced alerts.
	filteredAlerts AlertLabelSets
	// Tracks whether a change has occurred that requires a recomputation of
	// notification outputs.
	needsNotificationRefresh bool
}

// Options for constructing a memoryAlertManager.
type MemoryAlertManagerOptions struct {
	// Inhibitor for filtering out inhibited alerts.
	Inhibitor *Inhibitor
	// Silencer for filtering out silenced alerts.
	Silencer *Silencer
	// Notifier for dispatching notifications.
	Notifier Notifier
	// The minimum interval for alert refreshes before being purged.
	MinRefreshInterval time.Duration
}

// Constructs a new memoryAlertManager.
func NewMemoryAlertManager(o *MemoryAlertManagerOptions) AlertManager {
	return &memoryAlertManager{
		aggregates: make(map[AlertFingerprint]*AlertAggregate),

		minRefreshInterval: o.MinRefreshInterval,
		inhibitor:          o.Inhibitor,
		silencer:           o.Silencer,
		notifier:           o.Notifier,
	}
}

// Receive and ingest a new list of alert messages (e.g. from the web API).
func (s *memoryAlertManager) Receive(as Alerts) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, a := range as {
		s.ingest(a)
	}
}

// Ingests an alert into the memoryAlertManager and creates a new
// AggregationInstance for it, if necessary.
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
				break
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

// Get all AlertAggregates that match a given set of Filters.
func (s memoryAlertManager) GetAll(f Filters) AlertAggregates {
	s.mu.Lock()
	defer s.mu.Unlock()

	aggs := make(AlertAggregates, 0, len(s.aggregates))
	for _, agg := range s.aggregates {
		if f.Handles(agg.Alert.Labels) {
			// Make a deep copy of the AggregationRule so we can safely pass it to the
			// outside.
			aggCopy := *agg
			if agg.Rule != nil {
				rule := *agg.Rule
				aggCopy.Rule = &rule
			}
			alert := *agg.Alert
			aggCopy.Alert = &alert

			aggs = append(aggs, &aggCopy)
		}
	}
	return aggs
}

// Replace the current set of loaded AggregationRules by another.
func (s *memoryAlertManager) SetAggregationRules(rules AggregationRules) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Infof("Replacing aggregator rules (old: %d, new: %d)...", len(s.rules), len(rules))
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

// Check for any expired AlertAggregates and remove them from all indexes.
func (s *memoryAlertManager) removeExpiredAggregates() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// This loop is interrupted if either the heap is empty or only non-expired
	// aggregates remain in the heap.
	for {
		if len(s.aggregatesByLastRefreshed.AlertAggregates) == 0 {
			break
		}

		agg := heap.Pop(&s.aggregatesByLastRefreshed).(*AlertAggregate)

		if time.Since(agg.LastRefreshed) > s.minRefreshInterval {
			delete(s.aggregates, agg.Alert.Fingerprint())
			s.notifier.QueueNotification(agg.Alert, notificationOpResolve, agg.Rule.NotificationConfigName)
			s.needsNotificationRefresh = true
		} else {
			heap.Push(&s.aggregatesByLastRefreshed, agg)
			break
		}
	}

	if s.needsNotificationRefresh {
		s.aggregatesByNextNotification.rebuildFrom(s.aggregatesByLastRefreshed.AlertAggregates)
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
	if useCache && s.filteredAlerts != nil {
		return s.filteredAlerts
	}

	l := make(AlertLabelSets, 0, len(s.aggregates))
	for _, agg := range s.aggregates {
		l = append(l, agg.Alert.Labels)
	}

	l = s.inhibitor.Filter(l)
	s.filteredAlerts = s.silencer.Filter(l)
	return s.filteredAlerts
}

// Recomputes all currently uninhibited/unsilenced alerts and queues
// notifications for them according to their RepeatRate.
func (s *memoryAlertManager) refreshNotifications() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.needsNotificationRefresh = false

	l := s.filteredLabelSets(false)

	numSent := 0
	for _, lb := range l {
		agg := s.aggregates[lb.Fingerprint()]
		if agg.NextNotification.After(time.Now()) {
			continue
		}
		if agg.Rule != nil {
			s.notifier.QueueNotification(agg.Alert, notificationOpTrigger, agg.Rule.NotificationConfigName)
			agg.LastNotification = time.Now()
			agg.NextNotification = agg.LastNotification.Add(agg.Rule.RepeatRate)
			numSent++
		}
	}
	if numSent > 0 {
		log.Infof("Sent %d notifications", numSent)
		heap.Init(&s.aggregatesByNextNotification)
	}
}

// Reports whether a notification recomputation is required.
func (s *memoryAlertManager) refreshNeeded() (bool, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

// Perform some cheap state sanity checks.
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

// Run a single memoryAlertManager iteration.
func (s *memoryAlertManager) runIteration() {
	s.removeExpiredAggregates()
	s.checkNotificationRepeats()
	if refresh, reasons := s.refreshNeeded(); refresh {
		log.Infof("Recomputing notification outputs (%s)", strings.Join(reasons, ", "))
		s.refreshNotifications()
	}
}

// Run the memoryAlertManager's main dispatcher loop.
func (s *memoryAlertManager) Run() {
	iterationTicker := time.NewTicker(time.Second)
	for _ = range iterationTicker.C {
		s.checkSanity()
		s.runIteration()
	}
}
