// Copyright 2019 Prometheus Team
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

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTruncate(t *testing.T) {
	testCases := []struct {
		in string
		n  int

		out   string
		trunc bool
	}{
		{
			in:    "",
			n:     5,
			out:   "",
			trunc: false,
		},
		{
			in:    "abcde",
			n:     2,
			out:   "ab",
			trunc: true,
		},
		{
			in:    "abcde",
			n:     4,
			out:   "a...",
			trunc: true,
		},
		{
			in:    "abcde",
			n:     5,
			out:   "abcde",
			trunc: false,
		},
		{
			in:    "abcdefgh",
			n:     5,
			out:   "ab...",
			trunc: true,
		},
		{
			in:    "a⌘cde",
			n:     5,
			out:   "a⌘cde",
			trunc: false,
		},
		{
			in:    "a⌘cdef",
			n:     5,
			out:   "a⌘...",
			trunc: true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("truncate(%s,%d)", tc.in, tc.n), func(t *testing.T) {
			s, trunc := Truncate(tc.in, tc.n)
			require.Equal(t, tc.trunc, trunc)
			require.Equal(t, tc.out, s)
		})
	}
}
