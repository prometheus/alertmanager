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

package kafka

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrokerList(t *testing.T) {
	// Order independence: the same brokers in any order produce the
	// same string.
	a := BrokerList([]string{"b:9092", "a:9092", "c:9092"})
	b := BrokerList([]string{"c:9092", "a:9092", "b:9092"})
	require.Equal(t, a, b)
	require.Equal(t, "a:9092,b:9092,c:9092", a)

	// Empty input.
	require.Empty(t, BrokerList(nil))
	require.Empty(t, BrokerList([]string{}))
}

func TestBrokerListsEqual(t *testing.T) {
	require.True(t, BrokerListsEqual([]string{"a", "b"}, []string{"b", "a"}))
	require.True(t, BrokerListsEqual(nil, nil))
	require.True(t, BrokerListsEqual([]string{}, nil))
	require.False(t, BrokerListsEqual([]string{"a"}, []string{"b"}))
	require.False(t, BrokerListsEqual([]string{"a", "b"}, []string{"a"}))
}
