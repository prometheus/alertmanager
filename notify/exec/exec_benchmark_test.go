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

package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
)

func BenchmarkExecEmpty(b *testing.B) {
	conf := &config.ExecConfig{
		ExecFile: "/bin/true",
	}
	notifier := newSubject(b, conf)

	ctx := context.Background()
	ctx = notify.WithGroupKey(ctx, "1")

	for b.Loop() {
		retry, err := notifier.Notify(ctx)
		require.NoError(b, err)
		require.Equal(b, false, retry)
	}
}
