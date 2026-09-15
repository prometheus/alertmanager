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

package notify

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/types"
)

type closableNotifier struct {
	closed bool
}

func (n *closableNotifier) Notify(context.Context, ...*types.Alert) (bool, error) {
	return false, nil
}

func (n *closableNotifier) Close() error {
	n.closed = true
	return nil
}

func TestIntegrationClose(t *testing.T) {
	n := &closableNotifier{}
	i := NewIntegration(n, sendResolved(false), "test", 0, "receiver")
	require.NoError(t, i.Close())
	require.True(t, n.closed)
}
