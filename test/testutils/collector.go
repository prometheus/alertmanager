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
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/api/v2/models"
)

// Collector gathers alerts received by a notification receiver
// and verifies whether all arrived and within the correct time boundaries.
type Collector struct {
	t    *testing.T
	name string
	opts *AcceptanceOpts

	collected map[float64][]models.GettableAlerts
	expected  map[Interval][]models.GettableAlerts

	mtx sync.RWMutex
}

// NewCollector creates a new Collector with the given parameters.
func NewCollector(t *testing.T, name string, opts *AcceptanceOpts) *Collector {
	return &Collector{
		t:         t,
		name:      name,
		opts:      opts,
		collected: map[float64][]models.GettableAlerts{},
		expected:  map[Interval][]models.GettableAlerts{},
	}
}

func (c *Collector) String() string {
	return c.name
}

// Opts returns the acceptance options for this collector.
func (c *Collector) Opts() *AcceptanceOpts {
	return c.opts
}

// Collected returns a map of alerts collected by the collector indexed with the
// receive timestamp.
func (c *Collector) Collected() map[float64][]models.GettableAlerts {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
	return c.collected
}

func batchesEqual(as, bs models.GettableAlerts, opts *AcceptanceOpts) bool {
	if len(as) != len(bs) {
		return false
	}

	for _, a := range as {
		found := false
		for _, b := range bs {
			if EqualAlerts(a, b, opts) {
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

// Latest returns the latest relative point in time where a notification is
// expected.
func (c *Collector) Latest() float64 {
	c.mtx.RLock()
	defer c.mtx.RUnlock()
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
	c.mtx.Lock()
	defer c.mtx.Unlock()
	var nas models.GettableAlerts
	for _, a := range alerts {
		nas = append(nas, a.NativeAlert(c.opts))
	}

	c.expected[iv] = append(c.expected[iv], nas)
}

// Add the given alerts to the collected alerts.
// This is exported so it can be used by MockWebhook implementations.
func (c *Collector) Add(alerts ...*models.GettableAlert) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	arrival := c.opts.RelativeTime(time.Now())

	c.collected[arrival] = append(c.collected[arrival], models.GettableAlerts(alerts))
}

func (c *Collector) Check() string {
	var report strings.Builder
	fmt.Fprintf(&report, "\ncollector %q:\n\n", c)

	c.mtx.RLock()
	defer c.mtx.RUnlock()
	for iv, expected := range c.expected {
		fmt.Fprintf(&report, "interval %v\n", iv)

		var alerts []models.GettableAlerts
		for at, got := range c.collected {
			if iv.contains(at) {
				alerts = append(alerts, got...)
			}
		}

		for _, exp := range expected {
			found := len(exp) == 0 && len(alerts) == 0

			report.WriteString("---\n")

			for _, e := range exp {
				fmt.Fprintf(&report, "- %v\n", c.opts.AlertString(e))
			}

			for _, a := range alerts {
				if batchesEqual(exp, a, c.opts) {
					found = true
					break
				}
			}

			if found {
				report.WriteString("  [ ✓ ]\n")
			} else {
				c.t.Fail()
				report.WriteString("  [ ✗ ]\n")
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
		fmt.Fprintf(&report, "\nExpected total of %d alerts, got %d", totalExp, totalAct)
	}

	if c.t.Failed() {
		report.WriteString("\nreceived:\n")

		for at, col := range c.collected {
			for _, alerts := range col {
				fmt.Fprintf(&report, "@ %v\n", at)
				for _, a := range alerts {
					fmt.Fprintf(&report, "- %v\n", c.opts.AlertString(a))
				}
			}
		}
	}

	return report.String()
}

// alertsToString returns a string representation of the given Alerts. Use for
// debugging.
func alertsToString(as []*models.GettableAlert) (string, error) {
	b, err := json.Marshal(as)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// CompareCollectors compares two collectors based on their collected alerts.
func CompareCollectors(a, b *Collector, opts *AcceptanceOpts) (bool, error) {
	f := func(collected map[float64][]models.GettableAlerts) []*models.GettableAlert {
		result := []*models.GettableAlert{}
		for _, batches := range collected {
			for _, batch := range batches {
				for _, alert := range batch {
					result = append(result, alert)
				}
			}
		}
		return result
	}

	aAlerts := f(a.Collected())
	bAlerts := f(b.Collected())

	if len(aAlerts) != len(bAlerts) {
		aAsString, err := alertsToString(aAlerts)
		if err != nil {
			return false, err
		}
		bAsString, err := alertsToString(bAlerts)
		if err != nil {
			return false, err
		}

		err = fmt.Errorf(
			"first collector has %v alerts, second collector has %v alerts\n%v\n%v",
			len(aAlerts), len(bAlerts),
			aAsString, bAsString,
		)
		return false, err
	}

	for _, aAlert := range aAlerts {
		found := false
		for _, bAlert := range bAlerts {
			if EqualAlerts(aAlert, bAlert, opts) {
				found = true
				break
			}
		}

		if !found {
			aAsString, err := alertsToString([]*models.GettableAlert{aAlert})
			if err != nil {
				return false, err
			}
			bAsString, err := alertsToString(bAlerts)
			if err != nil {
				return false, err
			}

			err = fmt.Errorf(
				"could not find matching alert for alert from first collector\n%v\nin alerts of second collector\n%v",
				aAsString, bAsString,
			)

			return false, err
		}
	}

	return true, nil
}
