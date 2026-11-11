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

package cli

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/stretchr/testify/require"
)

func TestRoutingConfigFileFlagAcceptsStdin(t *testing.T) {
	t.Parallel()

	// kingpin runs the application validator after the flag values are
	// set and before any command action, so returning an error from it
	// stops Parse without reading stdin.
	errStop := errors.New("stop before running the command")
	app := kingpin.New("amtool", "")
	app.Validate(func(*kingpin.Application) error { return errStop })
	configureRoutingCmd(app.Command("config", ""))

	for _, args := range [][]string{
		{"config", "routes", "show", "--config.file=-"},
		{"config", "routes", "test", "--config.file=-", "foo=bar"},
	} {
		_, err := app.Parse(args)
		require.ErrorIs(t, err, errStop, "args: %v", args)
	}
}

func TestLoadConfigFile(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("testdata/conf.routing.yml")
	require.NoError(t, err)

	t.Run("file", func(t *testing.T) {
		t.Parallel()
		cfg, err := loadConfigFile("testdata/conf.routing.yml", strings.NewReader(""))
		require.NoError(t, err)
		require.Equal(t, "default", cfg.Route.Receiver)
	})

	t.Run("stdin", func(t *testing.T) {
		t.Parallel()
		cfg, err := loadConfigFile("-", strings.NewReader(string(content)))
		require.NoError(t, err)
		require.Equal(t, "default", cfg.Route.Receiver)
		require.Len(t, cfg.Route.Routes, 2)
	})

	t.Run("invalid stdin", func(t *testing.T) {
		t.Parallel()
		_, err := loadConfigFile("-", strings.NewReader("route: ["))
		require.Error(t, err)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()
		_, err := loadConfigFile("testdata/does-not-exist.yml", strings.NewReader(""))
		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}
