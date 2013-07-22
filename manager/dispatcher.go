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

	Event *Event

	Destination string
}

type SummaryDispatcher struct {
	summaryReqs chan *summaryDispatchRequest

	closed chan bool
}

type summaryDispatchRequest struct {
	Summary *EventSummary

	Response chan *summaryDispatchResponse
}

type Disposition int

const (
	UNHANDLED Disposition = iota
	DISPATCHED
	SUPPRESSED
)

type summaryDispatchResponse struct {
	Disposition Disposition
	Err         RemoteError
}

func (s *SummaryDispatcher) Close() {
	close(s.summaryReqs)
	<-s.closed
}

func NewSummaryDispatcher() *SummaryDispatcher {
	return &SummaryDispatcher{
		summaryReqs: make(chan *summaryDispatchRequest),
		closed:      make(chan bool),
	}
}

type RemoteError interface {
	error

	Retryable() bool
}

type remoteError struct {
	error

	retryable bool
}

func (e *remoteError) Retryable() bool {
	return e.retryable
}

func NewRemoteError(err error, retryable bool) RemoteError {
	return &remoteError{
		err,
		retryable,
	}
}

type SummaryReceiver interface {
	Receive(*EventSummary) RemoteError
}

func (d *SummaryDispatcher) Receive(s *EventSummary) RemoteError {
	req := &summaryDispatchRequest{
		Summary:  s,
		Response: make(chan *summaryDispatchResponse),
	}

	d.summaryReqs <- req
	resp := <-req.Response

	return resp.Err
}

func (d *SummaryDispatcher) dispatchSummary(r *summaryDispatchRequest, i IsInhibitedInterrogator) {
	if inhibited, _ := i.IsInhibited(r.Summary.Event); inhibited {
		r.Response <- &summaryDispatchResponse{
			Disposition: SUPPRESSED,
		}
		return
	}

	// BUG: Perform sending of summaries.
	r.Response <- &summaryDispatchResponse{
		Disposition: DISPATCHED,
	}
}

func (d *SummaryDispatcher) Dispatch(i IsInhibitedInterrogator) {
	for req := range d.summaryReqs {
		d.dispatchSummary(req, i)
	}

	d.closed <- true
}
