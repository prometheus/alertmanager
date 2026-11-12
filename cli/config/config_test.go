// Copyright 2015 Prometheus Team
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

package config

import (
	"io"
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/stretchr/testify/require"
)

var (
	url *string
	id  *string
)

func newApp() *kingpin.Application {
	url = new(string)
	id = new(string)

	app := kingpin.New("app", "")
	app.UsageWriter(io.Discard)
	app.ErrorWriter(io.Discard)
	app.Terminate(nil)

	app.Flag("url", "").StringVar(url)

	silence := app.Command("silence", "")
	silenceDel := silence.Command("del", "")
	silenceDel.Flag("id", "").StringVar(id)

	return app
}

func TestNewConfigResolver(t *testing.T) {
	for i, tc := range []struct {
		files []string
		err   bool
	}{
		{[]string{}, false},
		{[]string{"testdata/amtool.good1.yml", "testdata/amtool.good2.yml"}, false},
		{[]string{"testdata/amtool.good1.yml", "testdata/not_existing.yml"}, false},
		{[]string{"testdata/amtool.good1.yml", "testdata/amtool.bad.yml"}, true},
	} {
		_, err := NewResolver(tc.files, nil)
		if tc.err != (err != nil) {
			if tc.err {
				t.Fatalf("%d: expected error but got none", i)
			} else {
				t.Fatalf("%d: expected no error but got %v", i, err)
			}
		}
	}
}

type expectFn func()

func TestConfigResolverBind(t *testing.T) {
	expectURL := func(expected string) expectFn {
		return func() {
			if *url != expected {
				t.Fatalf("expected url flag %q but got %q", expected, *url)
			}
		}
	}
	expectID := func(expected string) expectFn {
		return func() {
			if *id != expected {
				t.Fatalf("expected ID flag %q but got %q", expected, *id)
			}
		}
	}

	for i, tc := range []struct {
		files       []string
		legacyFlags map[string]string
		args        []string

		err    bool
		expCmd string
		expFns []expectFn
	}{
		{
			[]string{"testdata/amtool.good1.yml", "testdata/amtool.good2.yml"},
			nil,
			[]string{},

			true,
			"",
			[]expectFn{expectURL("url1")}, // from amtool.good1.yml
		},
		{
			[]string{"testdata/amtool.good2.yml"},
			nil,
			[]string{},

			true,
			"",
			[]expectFn{expectURL("url2")}, // from amtool.good2.yml
		},
		{
			[]string{"testdata/amtool.good1.yml", "testdata/amtool.good2.yml"},
			nil,
			[]string{"--url", "url3"},

			true,
			"",
			[]expectFn{expectURL("url3")}, // from command line
		},
		{
			[]string{"testdata/amtool.good1.yml", "testdata/amtool.good2.yml"},
			map[string]string{"old-id": "id"},
			[]string{"silence", "del"},

			false,
			"silence del",
			[]expectFn{
				expectURL("url1"), // from amtool.good1.yml
				expectID("id1"),   // from amtool.good1.yml
			},
		},
		{
			[]string{"testdata/amtool.good2.yml"},
			map[string]string{"old-id": "id"},
			[]string{"silence", "del"},

			false,
			"silence del",
			[]expectFn{
				expectURL("url2"), // from amtool.good2.yml
				expectID("id2"),   // from amtool.good2.yml
			},
		},
		{
			[]string{"testdata/amtool.good2.yml"},
			map[string]string{"old-id": "id"},
			[]string{"silence", "del", "--id", "id3"},

			false,
			"silence del",
			[]expectFn{
				expectURL("url2"), // from amtool.good2.yml
				expectID("id3"),   // from command line
			},
		},
	} {
		r, err := NewResolver(tc.files, tc.legacyFlags)
		if err != nil {
			t.Fatalf("%d: expected no error but got: %v", i, err)
		}

		app := newApp()
		err = r.Bind(app, tc.args)
		if err != nil {
			t.Fatalf("%d: expected Bind() to return no error but got: %v", i, err)
		}

		cmd, err := app.Parse(tc.args)
		if tc.err != (err != nil) {
			if tc.err {
				t.Fatalf("%d: expected Parse() to return an error but got none", i)
			} else {
				t.Fatalf("%d: expected Parse() to return no error but got: %v", i, err)
			}
		}
		if cmd != tc.expCmd {
			t.Fatalf("%d: expected command %q but got %q", i, tc.expCmd, cmd)
		}
		for _, fn := range tc.expFns {
			fn()
		}
	}
}

func TestConfigResolverBindIncompleteArguments(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"empty command", []string{""}},
		{"partial command", []string{"si"}},
		{"empty subcommand", []string{"silence", ""}},
		{"partial subcommand", []string{"silence", "de"}},
		{"partial flag", []string{"silence", "del", "--i"}},
		{"missing flag value", []string{"silence", "del", "--id"}},
	} {
		for _, completion := range []bool{false, true} {
			name := tc.name
			if completion {
				name += " completion"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				terminated := false
				app := kingpin.New("app", "").UsageWriter(io.Discard).ErrorWriter(io.Discard)
				app.Terminate(func(code int) {
					terminated = true
					require.Zero(t, code)
				})
				app.Action(func(*kingpin.ParseContext) error {
					t.Error("incomplete arguments must not execute the command")
					return nil
				})
				url := app.Flag("url", "").String()
				id := app.Command("silence", "").Command("del", "").Flag("id", "").String()
				resolver := &Resolver{flags: map[string]string{"url": "config-url", "id": "config-id"}}

				args := tc.args
				if completion {
					args = append([]string{"--completion-bash"}, args...)
				}
				require.NoError(t, resolver.Bind(app, args))
				_, err := app.Parse(args)
				if completion {
					require.NoError(t, err)
					require.True(t, terminated, "completion should terminate successfully")
				} else {
					require.Error(t, err, "normal invocation must still reject incomplete arguments")
					require.False(t, terminated)
				}
				require.Equal(t, "config-url", *url)
				if len(tc.args) > 1 && tc.args[1] == "del" {
					require.Equal(t, "config-id", *id)
				}
			})
		}
	}
}

func TestConfigResolverBindInvalidApplication(t *testing.T) {
	t.Parallel()

	app := kingpin.New("app", "")
	app.Flag("duplicate", "").String()
	app.Flag("duplicate", "").String()
	resolver := &Resolver{}

	require.ErrorContains(t, resolver.Bind(app, nil), "duplicate")
}

func TestConfigResolverBindDoesNotEnableCompletion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"disabled completion", []string{"--no-completion-bash", ""}},
		{"flag value", []string{"--url=--completion-bash", "si"}},
		{"after terminator", []string{"--", "--completion-bash"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := kingpin.New("app", "")
			app.Flag("url", "").String()
			app.Command("silence", "")
			app.Terminate(func(int) { t.Error("completion must not be enabled") })
			resolver := &Resolver{}

			require.NoError(t, resolver.Bind(app, tc.args))
			_, err := app.Parse(tc.args)
			require.Error(t, err)
		})
	}
}
