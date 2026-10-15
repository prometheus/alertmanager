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
	"crypto/tls"
	"crypto/x509"
	"fmt"
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
	rpcClient   *http.Client
	tlsConfig   *tls.Config
}

type instanceOptions struct {
	routePrefix  string
	tlsEnabled   bool
	disableHTTP2 bool
}

// startInstance boots an Alertmanager and stops it when the test ends.
func startInstance(t testing.TB, options instanceOptions) *instance {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "alertmanager.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(minimalConfig), 0o600))

	addrs := []string{"127.0.0.1:0"}
	systemd := false
	webCfg := ""
	var clientTLS *tls.Config
	if options.tlsEnabled {
		certDir, err := filepath.Abs(filepath.Join("..", "..", "cluster", "testdata", "certs"))
		require.NoError(t, err)
		ca, err := os.ReadFile(filepath.Join(certDir, "ca.pem"))
		require.NoError(t, err)
		webCfg = filepath.Join(dir, "web.yml")
		require.NoError(t, os.WriteFile(webCfg, []byte(fmt.Sprintf(`tls_server_config:
  cert_file: %q
  key_file: %q
http_server_config:
  http2: %t
`, filepath.Join(certDir, "node1.pem"), filepath.Join(certDir, "node1-key.pem"), !options.disableHTTP2)), 0o600))
		roots := x509.NewCertPool()
		require.True(t, roots.AppendCertsFromPEM(ca))
		clientTLS = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}

	opts := app.DefaultOptions()
	opts.ConfigFile = configPath
	opts.DataDir = dir
	opts.RoutePrefix = options.routePrefix
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
	var rpcClient *http.Client
	scheme := "http://"
	if options.tlsEnabled {
		protocols := new(http.Protocols)
		protocols.SetHTTP1(true)
		protocols.SetHTTP2(true)
		transport := &http.Transport{TLSClientConfig: clientTLS, Protocols: protocols}
		client = &http.Client{Transport: transport, Timeout: requestTimeout}
		rpcClient = client
		scheme = "https://"
		t.Cleanup(transport.CloseIdleConnections)
	} else {
		t.Cleanup(client.CloseIdleConnections)
		protocols := new(http.Protocols)
		protocols.SetUnencryptedHTTP2(true)
		transport := &http.Transport{Protocols: protocols}
		rpcClient = &http.Client{Transport: transport, Timeout: requestTimeout}
		t.Cleanup(transport.CloseIdleConnections)
	}

	inst := &instance{
		app:         a,
		baseURL:     scheme + a.Addr(),
		routePrefix: options.routePrefix,
		httpClient:  client,
		rpcClient:   rpcClient,
		tlsConfig:   clientTLS,
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
