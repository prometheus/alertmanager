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
	"errors"
	"log"
	"sync"
	"time"
)

const (
	minimumRepeatRate       = 5 * time.Minute
	minimumRefreshPeriod    = 5 * time.Minute
	notificationRetryPeriod = 1 * time.Minute
)

// AggregationRule creates and manages the scope for received events.
type AggregationRule struct {
	Filters Filters

	RepeatRate time.Duration
}

type AggregationInstances []*AggregationInstance

type AggregationInstance struct {
	Rule  *AggregationRule
	Event *Event

	// When was this AggregationInstance created?
	Created time.Time
	// When was the last refresh received into this AggregationInstance?
	LastRefreshed time.Time

	// When was the last successful notification sent out for this
	// AggregationInstance?
	lastNotificationSent time.Time
	// Timer used to trigger a notification retry/resend.
	notificationResendTimer *time.Timer
	// Timer used to trigger the deletion of the AggregationInstance after it
	// hasn't been refreshed for too long.
	expiryTimer *time.Timer
}

func (r *AggregationRule) Handles(e *Event) bool {
	return r.Filters.Handles(e)
}

func (r *AggregationInstance) Ingest(e *Event) {
	r.Event = e
	r.LastRefreshed = time.Now()

	r.expiryTimer.Reset(minimumRefreshPeriod)
}

func (r *AggregationInstance) SendNotification(s SummaryReceiver) {
	if time.Since(r.lastNotificationSent) < r.Rule.RepeatRate {
		return
	}

	err := s.Receive(&EventSummary{
		Rule:  r.Rule,
		Event: r.Event,
	})
	if err != nil {
		log.Printf("Error while sending notification: %s, retrying in %v", err, notificationRetryPeriod)
		r.resendNotificationAfter(notificationRetryPeriod, s)
		return
	}

	r.resendNotificationAfter(r.Rule.RepeatRate, s)
	r.lastNotificationSent = time.Now()
}

func (r *AggregationInstance) resendNotificationAfter(d time.Duration, s SummaryReceiver) {
	r.notificationResendTimer = time.AfterFunc(d, func() {
		r.SendNotification(s)
	})
}

func (r *AggregationInstance) Close() {
	if r.notificationResendTimer != nil {
		r.notificationResendTimer.Stop()
	}
	if r.expiryTimer != nil {
		r.expiryTimer.Stop()
	}
}

type AggregationRules []*AggregationRule

type Aggregator struct {
	Rules           AggregationRules
	Aggregates      map[EventFingerprint]*AggregationInstance
	SummaryReceiver SummaryReceiver

	// Mutex to protect the above.
	mu sync.Mutex
}

func NewAggregator(s SummaryReceiver) *Aggregator {
	return &Aggregator{
		Aggregates:      make(map[EventFingerprint]*AggregationInstance),
		SummaryReceiver: s,
	}
}

func (a *Aggregator) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, agg := range a.Aggregates {
		agg.Close()
	}
}

func (a *Aggregator) Receive(events Events) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.Rules) == 0 {
		return errors.New("No aggregation rules")
	}
	for _, e := range events {
		for _, r := range a.Rules {
			if r.Handles(e) {
				fp := e.Fingerprint()
				aggregation, ok := a.Aggregates[fp]
				if !ok {
					expTimer := time.AfterFunc(minimumRefreshPeriod, func() {
						a.removeAggregate(fp)
					})

					aggregation = &AggregationInstance{
						Rule:        r,
						Created:     time.Now(),
						expiryTimer: expTimer,
					}

					a.Aggregates[fp] = aggregation
				}

				aggregation.Ingest(e)
				aggregation.SendNotification(a.SummaryReceiver)
				break
			}
		}
	}
	return nil
}

func (a *Aggregator) SetRules(rules AggregationRules) {
	a.mu.Lock()
	defer a.mu.Unlock()

	log.Println("Replacing", len(rules), "aggregator rules...")

	for _, rule := range rules {
		if rule.RepeatRate < minimumRepeatRate {
			log.Println("Rule repeat rate too low, setting to minimum value")
			rule.RepeatRate = minimumRepeatRate
		}
	}
	a.Rules = rules
}

func (a *Aggregator) AlertAggregates() AggregationInstances {
	a.mu.Lock()
	defer a.mu.Unlock()

	aggs := make(AggregationInstances, 0, len(a.Aggregates))
	for _, agg := range a.Aggregates {
		aggs = append(aggs, agg)
	}
	return aggs
}

func (a *Aggregator) removeAggregate(fp EventFingerprint) {
	a.mu.Lock()
	defer a.mu.Unlock()

	log.Println("Deleting expired aggregation instance", a)
	a.Aggregates[fp].Close()
	delete(a.Aggregates, fp)
}
