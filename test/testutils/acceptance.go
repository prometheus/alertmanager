// Copyright 2018 Prometheus Team
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

package testutils

import (
	"fmt"
	"net"
	"time"

	"github.com/prometheus/alertmanager/api/v2/models"
)

// AcceptanceOpts defines configuration parameters for an acceptance test.
type AcceptanceOpts struct {
	FeatureFlags []string
	RoutePrefix  string
	Tolerance    time.Duration
	baseTime     time.Time
}

// AlertString formats an alert for display with relative times.
func (opts *AcceptanceOpts) AlertString(a *models.GettableAlert) string {
	if a.EndsAt == nil || time.Time(*a.EndsAt).IsZero() {
		return fmt.Sprintf("%v[%v:]", a, opts.RelativeTime(time.Time(*a.StartsAt)))
	}
	return fmt.Sprintf("%v[%v:%v]", a, opts.RelativeTime(time.Time(*a.StartsAt)), opts.RelativeTime(time.Time(*a.EndsAt)))
}

// ExpandTime returns the absolute time for the relative time
// calculated from the test's base time.
func (opts *AcceptanceOpts) ExpandTime(rel float64) time.Time {
	return opts.baseTime.Add(time.Duration(rel * float64(time.Second)))
}

// RelativeTime returns the relative time for the given time
// calculated from the test's base time.
func (opts *AcceptanceOpts) RelativeTime(act time.Time) float64 {
	return float64(act.Sub(opts.baseTime)) / float64(time.Second)
}

// SetBaseTime sets the base time for relative time calculations.
func (opts *AcceptanceOpts) SetBaseTime(t time.Time) {
	opts.baseTime = t
}

// FreeAddress returns a new listen address not currently in use.
func FreeAddress() string {
	// Let the OS allocate a free address, close it and hope
	// it is still free when starting Alertmanager.
	l, err := net.Listen("tcp4", "localhost:0")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			panic(err)
		}
	}()

	return l.Addr().String()
}
