// Copyright 2016 Prometheus Team
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

package notify

import "testing"

func TestTruncate(t *testing.T) {
	cases := []struct {
		in, out string
		len     int
	}{
		{in: "a", len: 0, out: ""},
		{in: "0123456789abcdef", len: 10, out: "0123456789"},
		{in: "0123456789abcdef", len: 11, out: "01234567..."},
	}
	for _, c := range cases {
		if have := truncate(c.in, c.len); have != c.out {
			t.Errorf("Expected result %q for %q to length %d, got %q", c.out, c.in, c.len, have)
		}
	}
}
