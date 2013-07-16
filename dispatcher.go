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
	"strings"
)

type DestinationDispatcher interface {
	Send(*EventSummary) error
}

func DispatcherFor(destination string) DestinationDispatcher {
	switch {
	case strings.HasPrefix(destination, "IRC"):
	case strings.HasPrefix(destination, "TRELLO"):
	case strings.HasPrefix(destination, "MAIL"):
	case strings.HasPrefix(destination, "PAGERDUTY"):
	}
	return nil
}

type EventSummary struct {
	Rule *AggregationRule

	Events Events

	Destination string
}

type EventSummaries []EventSummary

type SummaryDispatcher struct{}

func (d *SummaryDispatcher) dispatchSummary(s EventSummary, i chan<- *IsInhibitedRequest) {
	log.Println("dispatching summary", s)
	r := &IsInhibitedRequest{
		Response: make(chan IsInhibitedResponse),
	}
	i <- r
	resp := <-r.Response
	log.Println(resp)
}

func (d *SummaryDispatcher) Dispatch(s <-chan EventSummary, i chan<- *IsInhibitedRequest) {
	for summary := range s {
		d.dispatchSummary(summary, i)
		//		fmt.Println("Summary for", summary.Rule, "with", summary.Events, "@", len(summary.Events))
	}
}
