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

//go:build !linux

package app

import (
	"bytes"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemdNotifierUnsupportedPlatform(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", filepath.Join(t.TempDir(), "notify"))
	t.Setenv("WATCHDOG_USEC", "invalid")
	var logs bytes.Buffer
	var n LifecycleNotifier = NewSystemdNotifier(slog.New(slog.NewTextHandler(&logs, nil)))
	n.Ready()
	n.Reloading()
	n.Reloaded(errors.New("reload failed"))
	n.Stopping()
	require.Empty(t, logs.String())
}
