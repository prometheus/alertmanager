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

package types

import (
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

func TestMatcher(t *testing.T) {
	m := NewMatcher("foo", "bar")

	if m.String() != "foo=\"bar\"" {
		t.Errorf("unexpected matcher string %#v", m.String())
	}

	re, err := regexp.Compile(".*")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	m = NewRegexMatcher("foo", re)

	if m.String() != "foo=~\".*\"" {
		t.Errorf("unexpected matcher string %#v", m.String())
	}
}

func TestMatchers(t *testing.T) {
	m1 := NewMatcher("foo", "bar")

	re, err := regexp.Compile(".*")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	m2 := NewRegexMatcher("bar", re)

	matchers := NewMatchers(m1, m2)

	if matchers.String() != "{bar=~\".*\",foo=\"bar\"}" {
		t.Errorf("unexpected matcher string %#v", matchers.String())
	}
}

func TestAlertMerge(t *testing.T) {
	now := time.Now()

	pairs := []struct {
		A, B, Res *Alert
	}{
		{
			A: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(2 * time.Minute),
				},
				UpdatedAt: now,
				Timeout:   true,
			},
			B: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
				Timeout:   true,
			},
			Res: &Alert{
				Alert: model.Alert{
					StartsAt: now.Add(-time.Minute),
					EndsAt:   now.Add(3 * time.Minute),
				},
				UpdatedAt: now.Add(time.Minute),
				Timeout:   true,
			},
		},
	}

	for _, p := range pairs {
		if res := p.A.Merge(p.B); !reflect.DeepEqual(p.Res, res) {
			t.Errorf("unexpected merged alert %#v", res)
		}
	}
}
