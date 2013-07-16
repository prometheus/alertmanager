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

type Suppression struct {
	Id uint

	Description string

	Filters *Filters

	EndsAt time.Time

	CreatedBy string
	CreatedAt time.Time
}

type SuppressionRequest struct {
	Suppression Suppression

	Response chan SuppressionResponse
}

type SuppressionResponse struct {
	Err error
}

type IsInhibitedRequest struct {
	Event Event

	Response chan IsInhibitedResponse
}

type IsInhibitedResponse struct {
	Err error

	Inhibited             bool
	InhibitingSuppression *Suppression
}

type SuppressionSummaryResponse struct {
	Err error

	Suppressions Suppressions
}

type SuppressionSummaryRequest struct {
	MatchCandidates map[string]string

	Response chan<- SuppressionSummaryResponse
}

type Suppressor struct {
	Suppressions *Suppressions
}

func NewSuppressor() *Suppressor {
	suppressions := new(Suppressions)
	heap.Init(suppressions)

	return &Suppressor{
		Suppressions: suppressions,
	}
}

type Suppressions []Suppression

func (s Suppressions) Len() int {
	return len(s)
}

func (s Suppressions) Less(i, j int) bool {
	return s[i].EndsAt.Before(s[j].EndsAt)
}

func (s Suppressions) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s *Suppressions) Push(v interface{}) {
	*s = append(*s, v.(Suppression))
}

func (s *Suppressions) Pop() interface{} {
	old := *s
	n := len(old)
	item := old[n-1]
	*s = old[0 : n-1]
	return item
}

func (s *Suppressor) dispatchSuppression(r SuppressionRequest) {
	log.Println("dispatching suppression", r)

	heap.Push(s.Suppressions, r.Suppression)
	r.Response <- SuppressionResponse{}
}

func (s *Suppressor) reapSuppressions(t time.Time) {
	log.Println("readping suppression...")

	i := sort.Search(len(*s.Suppressions), func(i int) bool {
		return (*s.Suppressions)[i].EndsAt.After(t)
	})

	*s.Suppressions = (*s.Suppressions)[i:]

	// BUG(matt): Validate if strictly necessary.
	heap.Init(s.Suppressions)
}

func (s *Suppressor) generateSummary(r SuppressionSummaryRequest) {
	log.Println("Generating summary", r)
	response := SuppressionSummaryResponse{}

	for _, suppression := range *s.Suppressions {
		response.Suppressions = append(response.Suppressions, suppression)
	}

	r.Response <- response
}

func (s *Suppressor) queryInhibit(q *IsInhibitedRequest) {
	response := IsInhibitedResponse{}

	for _, s := range *s.Suppressions {
		if s.Filters.Handle(&q.Event) {
			response.Inhibited = true
			response.InhibitingSuppression = &s

			break
		}
	}

	q.Response <- response
}

func (s *Suppressor) Dispatch(suppressions <-chan SuppressionRequest, inhibitQuery <-chan *IsInhibitedRequest, summaries <-chan SuppressionSummaryRequest) {
	reaper := time.NewTicker(30 * time.Second)
	defer reaper.Stop()

	closed := 0

	for closed < 2 {
		select {
		case suppression, open := <-suppressions:
			s.dispatchSuppression(suppression)

			if !open {
				closed++
			}

		case query, open := <-inhibitQuery:
			s.queryInhibit(query)

			if !open {
				closed++
			}

		case summary, open := <-summaries:
			s.generateSummary(summary)

			if !open {
				closed++
			}

		case time := <-reaper.C:
			s.reapSuppressions(time)
		}
	}
}
