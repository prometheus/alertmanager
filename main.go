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
)

type Main struct {
	SuppressionRequests chan SuppressionRequest
	InhibitQueries      chan *IsInhibitedRequest
	Summaries           chan SuppressionSummaryRequest
	AggregateEvents     chan *AggregateEventsRequest
	EventSummary        chan EventSummary
	Rules               chan *AggregatorResetRulesRequest
}

func (m *Main) close() {
	close(m.SuppressionRequests)
	close(m.InhibitQueries)
	close(m.Summaries)
	close(m.AggregateEvents)
	close(m.EventSummary)
	close(m.Rules)
}

func main() {
	main := &Main{
		SuppressionRequests: make(chan SuppressionRequest),
		InhibitQueries:      make(chan *IsInhibitedRequest),
		Summaries:           make(chan SuppressionSummaryRequest),
		AggregateEvents:     make(chan *AggregateEventsRequest),
		EventSummary:        make(chan EventSummary),
		Rules:               make(chan *AggregatorResetRulesRequest),
	}
	defer main.close()

	log.Print("Starting event suppressor...")
	suppressor := &Suppressor{
		Suppressions: new(Suppressions),
	}
	go suppressor.Dispatch(main.SuppressionRequests, main.InhibitQueries, main.Summaries)
	log.Println("Done.")

	log.Println("Starting event aggregator...")
	aggregator := NewAggregator()
	go aggregator.Dispatch(main.AggregateEvents, main.Rules, main.EventSummary)
	log.Println("Done.")

	done := make(chan bool)
	go func() {
		ar := make(chan *AggregatorResetRulesResponse)
		agg := &AggregatorResetRulesRequest{
			Rules: AggregationRules{
				NewAggregationRule(NewFilter("service", "discovery")),
			},
			Response: ar,
		}

		main.Rules <- agg
		log.Println("aggResult", <-ar)

		r := make(chan *AggregateEventsResponse)
		aer := &AggregateEventsRequest{
			Events: Events{
				&Event{
					Payload: map[string]string{
						"service": "discovery",
					},
				},
			},
			Response: r,
		}

		main.AggregateEvents <- aer

		log.Println("Response", r)

		done <- true
	}()
	<-done

	log.Println("Running summary dispatcher...")
	summarizer := new(SummaryDispatcher)
	summarizer.Dispatch(main.EventSummary, main.InhibitQueries)
}
