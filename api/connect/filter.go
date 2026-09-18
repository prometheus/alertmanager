// Copyright The Prometheus Authors
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

package apiconnect

import "fmt"

type valueFilter[T comparable] map[T]struct{}

func newValueFilter[T comparable](values []T, valid func(T) bool) (valueFilter[T], error) {
	filter := make(valueFilter[T], len(values))
	for index, value := range values {
		if !valid(value) {
			return nil, fmt.Errorf("invalid filter value at index %d: %v", index, value)
		}
		filter[value] = struct{}{}
	}
	return filter, nil
}

func (filter valueFilter[T]) matches(value T) bool {
	if len(filter) == 0 {
		return true
	}
	_, ok := filter[value]
	return ok
}
