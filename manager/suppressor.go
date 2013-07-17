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

type suppressionRequest struct {
	Suppression Suppression

	Response chan *suppressionResponse
}

type suppressionResponse struct {
	Err error
}

type isInhibitedRequest struct {
	Event *Event

	Response chan *isInhibitedResponse
}

type isInhibitedResponse struct {
	Err error

	Inhibited             bool
	InhibitingSuppression *Suppression
}

type suppressionSummaryResponse struct {
	Err error

	Suppressions Suppressions
}

type suppressionSummaryRequest struct {
	MatchCandidates map[string]string

	Response chan *suppressionSummaryResponse
}

type Suppressor struct {
	Suppressions *Suppressions

	suppressionReqs        chan *suppressionRequest
	suppressionSummaryReqs chan *suppressionSummaryRequest
	isInhibitedReqs        chan *isInhibitedRequest
}

type IsInhibitedInterrogator interface {
	IsInhibited(*Event) bool
}

func NewSuppressor() *Suppressor {
	suppressions := new(Suppressions)

	heap.Init(suppressions)

	return &Suppressor{
		Suppressions: suppressions,

		suppressionReqs:        make(chan *suppressionRequest),
		suppressionSummaryReqs: make(chan *suppressionSummaryRequest),
		isInhibitedReqs:        make(chan *isInhibitedRequest),
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

func (s *Suppressor) dispatchSuppression(r *suppressionRequest) {
	log.Println("dispatching suppression", r)

	heap.Push(s.Suppressions, r.Suppression)
	r.Response <- &suppressionResponse{}
	close(r.Response)
}

func (s *Suppressor) reapSuppressions(t time.Time) {
	log.Println("reaping suppression...")

	i := sort.Search(len(*s.Suppressions), func(i int) bool {
		return (*s.Suppressions)[i].EndsAt.After(t)
	})

	*s.Suppressions = (*s.Suppressions)[i:]

	// BUG(matt): Validate if strictly necessary.
	heap.Init(s.Suppressions)
}

func (s *Suppressor) generateSummary(r *suppressionSummaryRequest) {
	log.Println("Generating summary", r)
	response := new(suppressionSummaryResponse)

	for _, suppression := range *s.Suppressions {
		response.Suppressions = append(response.Suppressions, suppression)
	}

	r.Response <- response
	close(r.Response)
}

func (s *Suppressor) IsInhibited(e *Event) bool {
	req := &isInhibitedRequest{
		Event:    e,
		Response: make(chan *isInhibitedResponse),
	}

	s.isInhibitedReqs <- req

	resp := <-req.Response

	return resp.Inhibited
}

func (s *Suppressor) queryInhibit(q *isInhibitedRequest) {
	response := new(isInhibitedResponse)

	for _, s := range *s.Suppressions {
		if s.Filters.Handles(q.Event) {
			response.Inhibited = true
			response.InhibitingSuppression = &s

			break
		}
	}

	q.Response <- response
	close(q.Response)
}

func (s *Suppressor) Close() {
	close(s.suppressionReqs)
	close(s.suppressionSummaryReqs)
	close(s.isInhibitedReqs)
}

func (s *Suppressor) Dispatch() {
	// BUG: Accomplish this more intelligently by creating a timer for the least-
	//      likely-to-tenure item.
	reaper := time.NewTicker(30 * time.Second)
	defer reaper.Stop()

	closed := 0

	for closed < 2 {
		select {
		case suppression, open := <-s.suppressionReqs:
			s.dispatchSuppression(suppression)

			if !open {
				closed++
			}

		case query, open := <-s.isInhibitedReqs:
			s.queryInhibit(query)

			if !open {
				closed++
			}

		case summary, open := <-s.suppressionSummaryReqs:
			s.generateSummary(summary)

			if !open {
				closed++
			}

		case time := <-reaper.C:
			s.reapSuppressions(time)
		}
	}
}
