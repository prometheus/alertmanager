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
	"time"

	_ "github.com/prometheus/alertmanager/config/generated"
)

type InhibitRules []*InhibitRule

type InhibitRule struct {
	SourceFilters Filters
	TargetFilters Filters
	MatchOn []string
	BeforeAllowance time.Duration
	AfterAllowance time.Duration
}

func (i InhibitRules) Filter(a Alerts) Alerts {
	out := a
	for _, r := range i {
		out = r.Filter(a, out)
	}
	return out
}

func (i *InhibitRule) Filter(s Alerts, t Alerts) Alerts {
	s = i.SourceFilters.Filter(s)
	t = i.TargetFilters.Filter(t)
	out := Alerts{}
	for _, ta := range s {
		inhibited := true
		for _, sa := range t {
			if !ta.MatchOnLabels(sa, i.MatchOn) {
				inhibited = false
			}
		}
		if !inhibited {
			out = append(out, ta)
		}
	}
	return out
}
