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
  "container/heap"
	"log"
	"sort"
	"time"
)

type aggregationState int

const (
	aggIdle aggregationState = iota
	aggEmitting
)

type AggregationRule struct {
	Filters *Filters

	// BUG(matt): Unsupported.
	RepeatRate time.Duration

	fingerprint uint64
}

func NewAggregationRule(filters ...*Filter) *AggregationRule {
	f := new(Filters)
	heap.Init(f)
	for _, filter := range filters {
		heap.Push(f, filter)
	}

	return &AggregationRule{
		Filters:     f,
		fingerprint: f.fingerprint(),
	}
}

type AggregationInstance struct {
	Rule   *AggregationRule
	Events Events

	// BUG(matt): Unsupported.
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

func (r *AggregationInstance) Summarize(s chan<- EventSummary) {
	if r.state != aggIdle {
		return
	}
	if len(r.Events) == 0 {
		return
	}

	r.state = aggEmitting

	s <- EventSummary{
		Rule:   r.Rule,
		Events: r.Events,
	}
}

type AggregationRules []*AggregationRule

func (r AggregationRules) Len() int {
	return len(r)
}

func (r AggregationRules) Less(i, j int) bool {
	return r[i].fingerprint < r[j].fingerprint
}

func (r AggregationRules) Swap(i, j int) {
	r[i], r[j] = r[i], r[j]
}

type Aggregator struct {
	Rules      AggregationRules
	Aggregates map[uint64]*AggregationInstance
}

func NewAggregator() *Aggregator {
	return &Aggregator{
		Aggregates: make(map[uint64]*AggregationInstance),
	}
}

type AggregateEventsResponse struct {
	Err error
}

type AggregateEventsRequest struct {
	Events Events

	Response chan *AggregateEventsResponse
}

func (a *Aggregator) aggregate(r *AggregateEventsRequest, s chan<- EventSummary) {
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

	r.Response <- new(AggregateEventsResponse)
}

type AggregatorResetRulesResponse struct {
	Err error
}
type AggregatorResetRulesRequest struct {
	Rules AggregationRules

	Response chan *AggregatorResetRulesResponse
}

func (a *Aggregator) replaceRules(r *AggregatorResetRulesRequest) {
	newRules := AggregationRules{}
	for _, rule := range r.Rules {
		newRules = append(newRules, rule)
	}

	sort.Sort(newRules)

	a.Rules = newRules

	r.Response <- new(AggregatorResetRulesResponse)
}

func (a *Aggregator) Dispatch(reqs <-chan *AggregateEventsRequest, rules <-chan *AggregatorResetRulesRequest, s chan<- EventSummary) {
	t := time.NewTicker(time.Second)
	defer t.Stop()

	closed := 0

	for closed < 1 {
		select {
		case req, open := <-reqs:
			a.aggregate(req, s)

			if !open {
				closed++
			}

		case rules, open := <-rules:
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
}
