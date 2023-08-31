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

const (
	simpleExample  = "{foo=\"bar\"}"
	complexExample = "{foo=\"bar\",bar=~\"[a-zA-Z0-9+]\"}"
)

func BenchmarkParseSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := Matchers(simpleExample); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseComplex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := Matchers(complexExample); err != nil {
			b.Fatal(err)
		}
	}
}
