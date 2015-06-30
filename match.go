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

package main

import (
	"regexp"

	"github.com/prometheus/common/model"
)

// Matcher defines a matching rule for the value of a given label.
type Matcher struct {
	Name  model.LabelName
	Value string

	isRegex bool
	regex   *regexp.Regexp
}

func NewMatcher(name model.LabelName, value string) *Matcher {
	return &Matcher{
		Name:  name,
		Value: value,
	}
}

func NewRegexMatcher(name model.LabelName, value string) (*Matcher, error) {
	re, err := regexp.Compile(value)
	if err != nil {
		return nil, err
	}
	m := &Matcher{
		Name:    name,
		Value:   value,
		isRegex: true,
		regex:   re,
	}
	return m, nil
}

type Matchers []*Matcher
