// Copyright 2015 Prometheus Team
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

package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

// Collector gathers alerts received by a notification receiver
// and verifies whether all arrived and within the correct time boundaries.
type Collector struct {
	t    *testing.T
	name string
	opts *AcceptanceOpts

	collected map[float64][]model.Alerts
	expected  map[Interval][]model.Alerts
}

func (c *Collector) String() string {
	return c.name
}

func batchesEqual(as, bs model.Alerts, opts *AcceptanceOpts) bool {
	if len(as) != len(bs) {
		return false
	}

	for _, a := range as {
		found := false
		for _, b := range bs {
			if equalAlerts(a, b, opts) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// latest returns the latest relative point in time where a notification is
// expected.
func (c *Collector) latest() float64 {
	var latest float64
	for iv := range c.expected {
		if iv.end > latest {
			latest = iv.end
		}
	}
	return latest
}

// Want declares that the Collector expects to receive the given alerts
// within the given time boundaries.
func (c *Collector) Want(iv Interval, alerts ...*TestAlert) {
	var nas model.Alerts
	for _, a := range alerts {
		nas = append(nas, a.nativeAlert(c.opts))
	}

	c.expected[iv] = append(c.expected[iv], nas)
}

// add the given alerts to the collected alerts.
func (c *Collector) add(alerts ...*model.Alert) {
	arrival := c.opts.relativeTime(time.Now())

	c.collected[arrival] = append(c.collected[arrival], model.Alerts(alerts))
}

func (c *Collector) check() string {
	report := fmt.Sprintf("\ncollector %q:\n\n", c)

	for iv, expected := range c.expected {
		report += fmt.Sprintf("interval %v\n", iv)

		for _, exp := range expected {
			var found model.Alerts

			report += fmt.Sprintf("---\n")

			for _, e := range exp {
				report += fmt.Sprintf("- %v\n", c.opts.alertString(e))
			}

			for at, got := range c.collected {
				if !iv.contains(at) {
					continue
				}
				for _, a := range got {
					if batchesEqual(exp, a, c.opts) {
						found = a
						break
					}
				}
				if found != nil {
					break
				}
			}

			if found != nil {
				report += fmt.Sprintf("  [ ✓ ]\n")
			} else {
				c.t.Fail()
				report += fmt.Sprintf("  [ ✗ ]\n")
			}
		}
	}

	// Detect unexpected notifications.
	var totalExp, totalAct int
	for _, exp := range c.expected {
		for _, e := range exp {
			totalExp += len(e)
		}
	}
	for _, act := range c.collected {
		for _, a := range act {
			if len(a) == 0 {
				c.t.Error("received empty notifications")
			}
			totalAct += len(a)
		}
	}
	if totalExp != totalAct {
		c.t.Fail()
		report += fmt.Sprintf("\nExpected total of %d alerts, got %d", totalExp, totalAct)
	}

	if c.t.Failed() {
		report += "\nreceived:\n"

		for at, col := range c.collected {
			for _, alerts := range col {
				report += fmt.Sprintf("@ %v\n", at)
				for _, a := range alerts {
					report += fmt.Sprintf("- %v\n", c.opts.alertString(a))
				}
			}
		}
	}

	return report
}
