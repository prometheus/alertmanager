// Copyright 2023 The Prometheus Authors
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

package parse

import (
	"testing"
)

// FuzzParse fuzz tests the parser to see if we can make it panic.
func FuzzParse(f *testing.F) {
	f.Add("{foo=bar,bar=~[a-zA-Z]+,baz!=qux,qux!~[0-9]+")
	f.Fuzz(func(t *testing.T, s string) {
		matchers, err := Matchers(s)
		if matchers != nil && err != nil {
			t.Errorf("Unexpected matchers and err: %v %s", matchers, err)
		}
	})
}
