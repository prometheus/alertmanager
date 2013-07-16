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
	"fmt"
	"hash/fnv"
	"sort"
	"time"
)

// Event models an action triggered by Prometheus.
type Event struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include a "name" label.
	Labels map[string]string

	// CreatedAt indicates when the event was created.
	CreatedAt time.Time

	// ExpiresAt is the allowed lifetime for this event before it is reaped.
	ExpiresAt time.Time

	Payload map[string]string
}

func (e Event) Fingerprint() uint64 {
	keys := []string{}

	for k := range e.Payload {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	summer := fnv.New64a()

	for _, k := range keys {
		fmt.Fprintf(summer, k, e.Payload[k])
	}

	return summer.Sum64()
}

type Events []*Event
