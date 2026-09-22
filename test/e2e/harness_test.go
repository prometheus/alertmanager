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

// Package e2e contains end-to-end tests that boot Alertmanager in-process via
// the app package and exercise the experimental Connect API services through
// their generated ConnectRPC clients.
//
// The tests in this package intentionally do not call t.Parallel: building the
// API v2 router mutates a process-global OpenAPI spec inside go-openapi, and
// starting several instances concurrently trips the race detector.
package e2e

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/app"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
)

const minimalConfig = `route:
  receiver: default
receivers:
  - name: default
`

// requestTimeout bounds every request an e2e test makes against an instance.
const requestTimeout = 5 * time.Second

// featureFlags is the feature set shared by every instance in this package.
var featureFlags featurecontrol.Flagger

// TestMain initializes the process-global matcher compatibility mode once.
// The call to compat.InitFromFlags mutates package-level state, so it must
// not run concurrently from parallel tests.
func TestMain(m *testing.M) {
	logger := promslog.NewNopLogger()
	ff, err := featurecontrol.NewFlags(logger, "")
	if err != nil {
		panic(err)
	}
	featureFlags = ff
	compat.InitFromFlags(logger, ff)
	os.Exit(m.Run())
}

// instance is a running in-process Alertmanager bound to an ephemeral port
// with clustering disabled. Both API v2 and the Connect API are served.
type instance struct {
	app         *app.App
	baseURL     string
	routePrefix string
	httpClient  *http.Client
	h2cClient   *http.Client
}

// startInstance boots an Alertmanager and stops it when the test ends.
func startInstance(t testing.TB, routePrefix string) *instance {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(minimalConfig), 0o600))

	addrs := []string{"127.0.0.1:0"}
	systemd := false
	webCfg := ""

	opts := app.DefaultOptions()
	opts.ConfigFile = configPath
	opts.DataDir = dir
	opts.RoutePrefix = routePrefix
	opts.WebConfig = &web.FlagConfig{
		WebListenAddresses: &addrs,
		WebSystemdSocket:   &systemd,
		WebConfigFile:      &webCfg,
	}
	opts.Logger = promslog.NewNopLogger()
	opts.Registerer = prometheus.NewRegistry()
	opts.Flagger = featureFlags

	a, err := app.New(opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		require.NoError(t, a.Stop(ctx))
	})
	require.NoError(t, a.Start())

	client := &http.Client{Timeout: requestTimeout}
	t.Cleanup(client.CloseIdleConnections)
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	h2cTransport := &http.Transport{Protocols: protocols}
	h2cClient := &http.Client{Transport: h2cTransport, Timeout: requestTimeout}
	t.Cleanup(h2cTransport.CloseIdleConnections)

	inst := &instance{
		app:         a,
		baseURL:     "http://" + a.Addr(),
		routePrefix: routePrefix,
		httpClient:  client,
		h2cClient:   h2cClient,
	}
	inst.waitHealthy(t)
	return inst
}

// waitHealthy blocks until the instance serves /-/healthy with a 200.
func (i *instance) waitHealthy(t testing.TB) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp, err := i.httpClient.Get(i.webURL("/-/healthy"))
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, requestTimeout, 50*time.Millisecond)
}

func (i *instance) webURL(path string) string {
	return i.baseURL + i.routePrefix + path
}

func (i *instance) apiPath() string {
	return i.routePrefix + "/api"
}

func (i *instance) statusClient(httpClient connect.HTTPClient, basePath string, opts ...connect.ClientOption) statusv3alphaconnect.StatusServiceClient {
	return statusv3alphaconnect.NewStatusServiceClient(httpClient, i.baseURL+basePath, opts...)
}
