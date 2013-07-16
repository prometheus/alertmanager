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
	"regexp"
)

type Filter struct {
	Name  *regexp.Regexp
	Value *regexp.Regexp

	fingerprint uint64
}

func NewFilter(namePattern string, valuePattern string) *Filter {
	summer := fnv.New64a()
	fmt.Fprintf(summer, namePattern, valuePattern)

	return &Filter{
		Name:        regexp.MustCompile(namePattern),
		Value:       regexp.MustCompile(valuePattern),
		fingerprint: summer.Sum64(),
	}
}

func (f *Filter) Handles(e *Event) bool {
	for k, v := range e.Payload {
		if f.Name.MatchString(k) && f.Value.MatchString(v) {
			return true
		}
	}

	return false
}

type Filters []*Filter

func (f Filters) Len() int {
	return len(f)
}

func (f Filters) Less(i, j int) bool {
	return f[i].fingerprint < f[j].fingerprint
}

func (f Filters) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

func (f *Filters) Push(v interface{}) {
	(*f) = append(*f, v.(*Filter))
}

func (f *Filters) Pop() interface{} {
	i := (*f)[len(*f)-1]
	(*f) = (*f)[0:len(*f)]

	return i
}

func (f Filters) Handle(e *Event) bool {
	fCount := len(f)
	fMatch := 0

	for _, filter := range f {
		if filter.Handles(e) {
			fMatch++
		}
	}

	return fCount == fMatch
}

func (f Filters) fingerprint() uint64 {
	summer := fnv.New64a()

	for i, f := range f {
		fmt.Fprintln(summer, i, f.fingerprint)
	}

	return summer.Sum64()
}
