// Copyright 2026 Prometheus Team
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSilenceUpdateQuiet(t *testing.T) {
	// The server answers every update with a new ID, as it does when an update replaces the silence.
	newIDs := map[string]string{
		"00000000-0000-4000-8000-000000000001": "00000000-0000-4000-8000-0000000000a1",
		"00000000-0000-4000-8000-000000000002": "00000000-0000-4000-8000-0000000000a2",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v2/silence/"):
			fmt.Fprintf(w, `{"id": %q, "startsAt": "2026-01-01T00:00:00.000Z", "endsAt": "2026-01-01T01:00:00.000Z"}`,
				strings.TrimPrefix(r.URL.Path, "/api/v2/silence/"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/silences":
			var posted struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			fmt.Fprintf(w, `{"silenceID": %q}`, newIDs[posted.ID])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	// The command prints to os.Stdout and reads the package globals, so this test cannot run in parallel.
	oldURL, oldStdout := alertmanagerURL, os.Stdout
	t.Cleanup(func() { alertmanagerURL, os.Stdout = oldURL, oldStdout })
	alertmanagerURL = u

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	c := &silenceUpdateCmd{
		quiet:    true,
		duration: "2h",
		ids:      []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"},
	}
	require.NoError(t, c.update(context.Background(), nil))

	require.NoError(t, w.Close())
	var got bytes.Buffer
	_, err = io.Copy(&got, r)
	require.NoError(t, err)
	require.Equal(t, "00000000-0000-4000-8000-0000000000a1\n00000000-0000-4000-8000-0000000000a2\n", got.String())
}
