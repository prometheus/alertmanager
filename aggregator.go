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

package main

import (
	"log"
	"time"
)

type aggregationState int

const (
	aggIdle aggregationState = iota
	aggEmitting
)

// AggregationRule creates and manages the scope for received events.
type AggregationRule struct {
	Filters Filters

	RepeatRate time.Duration
}

type AggregationInstance struct {
	Rule   *AggregationRule
	Events Events

	EndsAt time.Time

	state aggregationState
}

func (r *AggregationRule) Handles(e *Event) bool {
	return r.Filters.Handle(e)
}

func (r *AggregationInstance) Ingest(e *Event) {
	r.Events = append(r.Events, e)
}

func (r *AggregationInstance) Tidy() {
	// BUG(matt): Drop this in favor of having the entire AggregationInstance
	// being dropped when too old.
	log.Println("Tidying...")
	if len(r.Events) == 0 {
		return
	}

	events := Events{}

	t := time.Now()
	for _, e := range r.Events {
		if t.Before(e.CreatedAt) {
			events = append(events, e)
		}
	}

	if len(events) == 0 {
		r.state = aggIdle
	}

	r.Events = events
}

func (r *AggregationInstance) Summarize(s SummaryReceiver) {
	if r.state != aggIdle {
		return
	}
	if len(r.Events) == 0 {
		return
	}

	r.state = aggEmitting

	s.Receive(&EventSummary{
		Rule:   r.Rule,
		Events: r.Events,
	})

}

type AggregationRules []*AggregationRule

type Aggregator struct {
	Rules      AggregationRules
	Aggregates map[uint64]*AggregationInstance

	aggRequests   chan *aggregateEventsRequest
	rulesRequests chan *aggregatorResetRulesRequest
	closed        chan bool
}

func NewAggregator() *Aggregator {
	return &Aggregator{
		Aggregates: make(map[uint64]*AggregationInstance),

		aggRequests:   make(chan *aggregateEventsRequest),
		rulesRequests: make(chan *aggregatorResetRulesRequest),
		closed:        make(chan bool),
	}
}

func (a *Aggregator) Close() {
	close(a.rulesRequests)
	close(a.aggRequests)

	<-a.closed
	close(a.closed)
}

type aggregateEventsResponse struct {
	Err error
}

type aggregateEventsRequest struct {
	Events Events

	Response chan *aggregateEventsResponse
}

func (a *Aggregator) aggregate(r *aggregateEventsRequest, s SummaryReceiver) {
	log.Println("aggregating", *r)
	for _, element := range r.Events {
		fp := element.Fingerprint()
		for _, r := range a.Rules {
			log.Println("Checking rule", r, r.Handles(element))
			if r.Handles(element) {
				aggregation, ok := a.Aggregates[fp]
				if !ok {
					aggregation = &AggregationInstance{
						Rule: r,
					}

					a.Aggregates[fp] = aggregation
				}

				aggregation.Ingest(element)
				aggregation.Summarize(s)
				break
			}
		}
	}

	r.Response <- new(aggregateEventsResponse)
	close(r.Response)
}

type aggregatorResetRulesResponse struct{}

type aggregatorResetRulesRequest struct {
	Rules AggregationRules

	Response chan *aggregatorResetRulesResponse
}

func (a *Aggregator) replaceRules(r *aggregatorResetRulesRequest) {
	log.Println("Replacing", len(r.Rules), "aggregator rules...")
	newRules := make(AggregationRules, len(r.Rules))
	copy(newRules, r.Rules)
	a.Rules = newRules

	r.Response <- new(aggregatorResetRulesResponse)
	close(r.Response)
}

func (a *Aggregator) Receive(e Events) error {
	req := &aggregateEventsRequest{
		Events:   e,
		Response: make(chan *aggregateEventsResponse),
	}

	a.aggRequests <- req

	result := <-req.Response

	return result.Err
}

func (a *Aggregator) SetRules(r AggregationRules) error {
	req := &aggregatorResetRulesRequest{
		Rules:    r,
		Response: make(chan *aggregatorResetRulesResponse),
	}

	a.rulesRequests <- req

	_ = <-req.Response

	return nil
}

func (a *Aggregator) Dispatch(s SummaryReceiver) {
	t := time.NewTicker(time.Second)
	defer t.Stop()

	closed := 0

	for closed < 2 {
		select {
		case req, open := <-a.aggRequests:
			a.aggregate(req, s)

			if !open {
				closed++
			}

		case rules, open := <-a.rulesRequests:
			a.replaceRules(rules)

			if !open {
				closed++
			}

		case <-t.C:
			for _, a := range a.Aggregates {
				a.Tidy()
			}
		}
	}

	a.closed <- true
}
