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

package gotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/notify/test"
	"github.com/prometheus/alertmanager/types"
)

func TestGotify_Notify(t *testing.T) {
	var gotHeader string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Gotify-Key")
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		dec := json.NewDecoder(r.Body)
		require.NoError(t, dec.Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	cfg := &GotifyConfig{
		URL:        &amcommoncfg.URL{URL: u},
		Token:      commoncfg.Secret("token"),
		HTTPConfig: &commoncfg.HTTPClientConfig{},
		Title:      "t",
		Message:    "m",
		Priority:   "2",
	}

	n, err := New(cfg, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)

	ctx := notify.WithGroupKey(context.Background(), "1")
	_, err = n.Notify(ctx, &types.Alert{Alert: model.Alert{StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)}})
	require.NoError(t, err)

	require.Equal(t, "token", gotHeader)
	require.Equal(t, "t", gotBody["title"])
	require.Equal(t, "m", gotBody["message"])
	require.EqualValues(t, float64(2), gotBody["priority"])
	_, ok := gotBody["extras"]
	require.False(t, ok)
}

func TestGotify_Notify_MarkdownExtras(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		require.NoError(t, dec.Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	cfg := &GotifyConfig{
		URL:         &amcommoncfg.URL{URL: u},
		Token:       commoncfg.Secret("token"),
		HTTPConfig:  &commoncfg.HTTPClientConfig{},
		Title:       "t",
		Message:     "m",
		Priority:    "2",
		ContentType: "text/markdown",
	}

	n, err := New(cfg, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)

	ctx := notify.WithGroupKey(context.Background(), "1")
	_, err = n.Notify(ctx, &types.Alert{Alert: model.Alert{StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)}})
	require.NoError(t, err)

	extras, ok := gotBody["extras"].(map[string]any)
	require.True(t, ok)
	clientDisplay, ok := extras["client::display"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text/markdown", clientDisplay["contentType"])
}

func TestGotifyReadingURLAndTokenFromFiles(t *testing.T) {
	ctx, u, fn := test.GetContextWithCancelingURL()
	defer fn()

	urlFile, err := os.CreateTemp(t.TempDir(), "gotify_url")
	require.NoError(t, err)
	_, err = urlFile.WriteString(u.String() + "\n")
	require.NoError(t, err)

	tokenFile, err := os.CreateTemp(t.TempDir(), "gotify_token")
	require.NoError(t, err)
	_, err = tokenFile.WriteString("secret\n")
	require.NoError(t, err)

	n, err := New(&GotifyConfig{
		URLFile:     urlFile.Name(),
		TokenFile:   tokenFile.Name(),
		HTTPConfig:  &commoncfg.HTTPClientConfig{},
		Title:       "t",
		Message:     "m",
		Priority:    "2",
		ContentType: "text/plain",
	}, test.CreateTmpl(t), promslog.NewNopLogger())
	require.NoError(t, err)

	test.AssertNotifyLeaksNoSecret(ctx, t, n, "secret")
}
