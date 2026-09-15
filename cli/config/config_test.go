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
