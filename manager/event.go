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
	"fmt"
	"hash/fnv"
	"sort"
)

const (
	eventNameLabel = "name"
)

type EventFingerprint uint64

type EventLabels map[string]string
type EventPayload map[string]string

// Event models an action triggered by Prometheus.
type Event struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include a "name" label.
	Labels EventLabels
	// Extra key/value information which is not used for aggregation.
	Payload EventPayload
}

func (e Event) Name() string {
	// BUG: ensure in a proper place that all events have a name?
	return e.Labels[eventNameLabel]
}

func (e Event) Fingerprint() EventFingerprint {
	keys := []string{}

	for k := range e.Labels {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	summer := fnv.New64a()

	for _, k := range keys {
		fmt.Fprintf(summer, k, e.Labels[k])
	}

	return EventFingerprint(summer.Sum64())
}

type Events []*Event
