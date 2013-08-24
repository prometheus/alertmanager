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

const AlertNameLabel = "alertname"

type AlertFingerprint uint64

type AlertLabels map[string]string
type AlertPayload map[string]string

type Alerts []*Alert

// Alert models an action triggered by Prometheus.
type Alert struct {
	// Short summary of alert.
	Summary string
	// Long description of alert.
	Description string
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels AlertLabels
	// Extra key/value information which is not used for aggregation.
	Payload AlertPayload
}

func (a *Alert) Name() string {
	return a.Labels[AlertNameLabel]
}

func (a *Alert) Fingerprint() AlertFingerprint {
	keys := []string{}

	for k := range a.Labels {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	summer := fnv.New64a()

	separator := string([]byte{0})
	for _, k := range keys {
		fmt.Fprintf(summer, "%s%s%s%s", k, separator, a.Labels[k], separator)
	}

	return AlertFingerprint(summer.Sum64())
}

func (a *Alert) MatchOnLabels(o *Alert, labels []string) bool {
	for _, l := range labels {
		if a.Labels[l] != o.Labels[l] {
			return false
		}
	}
	return true
}
