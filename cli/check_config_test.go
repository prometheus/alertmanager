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

package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
)

func TestCheckConfig(t *testing.T) {
	err := CheckConfig([]string{"testdata/conf.good.yml"})
	if err != nil {
		t.Fatalf("checking valid config file failed with: %v", err)
	}

	err = CheckConfig([]string{"testdata/conf.bad.yml"})
	if err == nil {
		t.Fatalf("failed to detect invalid file.")
	}

	// Routes with the same matchers and receiver but different options load
	// with a warning.
	err = CheckConfig([]string{"testdata/conf.routes-same-key.yml"})
	require.NoError(t, err)
	cfg, err := config.LoadFile("testdata/conf.routes-same-key.yml")
	require.NoError(t, err)
	warnings := dispatch.NewRoute(cfg.Route, nil).Warnings()
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "have the same matchers and receiver")
}
