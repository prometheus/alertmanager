// Copyright 2018 Prometheus Team
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

package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/client_golang/prometheus"
)

func TestJoin(t *testing.T) {
	logger := log.NewNopLogger()
	p, err := Join(logger,
		prometheus.DefaultRegisterer,
		"0.0.0.0:0",
		"",
		[]string{},
		true,
		0*time.Second,
		0*time.Second,
		0*time.Second,
		0*time.Second,
		0*time.Second,
	)
	require.NoError(t, err)
	require.False(t, p == nil)
	require.False(t, p.Ready())
	require.Equal(t, p.Status(), "settling")
	go p.Settle(context.Background(), 0*time.Second)
	p.WaitReady()
	require.Equal(t, p.Status(), "ready")
}
