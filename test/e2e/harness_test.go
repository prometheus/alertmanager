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

package e2e

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/exporter-toolkit/web"

	"github.com/prometheus/alertmanager/api/status/v3/statusv3connect"
	"github.com/prometheus/alertmanager/app"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/matcher/compat"
)

const minimalConfig = `route:
  receiver: default
receivers:
  - name: default
`

// instance is a running in-process Alertmanager bound to an ephemeral port
// with clustering disabled. Both API v2 and the Connect API are served.
type instance struct {
	app     *app.App
	baseURL string
}

// startInstance boots an Alertmanager and registers its teardown (and
// temp-dir removal) via Ginkgo's DeferCleanup.
func startInstance() *instance {
	GinkgoHelper()

	dir, err := os.MkdirTemp("", "am-e2e-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(dir) })

	configPath := filepath.Join(dir, "alertmanager.yml")
	Expect(os.WriteFile(configPath, []byte(minimalConfig), 0o600)).To(Succeed())

	logger := promslog.NewNopLogger()
	ff, err := featurecontrol.NewFlags(logger, "")
	Expect(err).NotTo(HaveOccurred())
	// compat.InitFromFlags mutates package-global matcher state; the e2e
	// suite always uses the same feature set, so this is safe.
	compat.InitFromFlags(logger, ff)

	addrs := []string{"127.0.0.1:0"}
	systemd := false
	webCfg := ""

	opts := app.DefaultOptions()
	opts.ConfigFile = configPath
	opts.DataDir = dir
	opts.WebConfig = &web.FlagConfig{
		WebListenAddresses: &addrs,
		WebSystemdSocket:   &systemd,
		WebConfigFile:      &webCfg,
	}
	opts.Logger = logger
	opts.Registerer = prometheus.NewRegistry()
	opts.Flagger = ff

	a, err := app.New(opts)
	Expect(err).NotTo(HaveOccurred())
	Expect(a.Start()).To(Succeed())
	DeferCleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Expect(a.Stop(ctx)).To(Succeed())
	})

	inst := &instance{app: a, baseURL: "http://" + a.Addr()}
	inst.waitHealthy()
	return inst
}

// waitHealthy blocks until the instance serves /-/healthy with a 200.
func (i *instance) waitHealthy() {
	GinkgoHelper()
	client := &http.Client{Timeout: time.Second}
	Eventually(func() int {
		resp, err := client.Get(i.baseURL + "/-/healthy")
		if err != nil {
			return 0
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}, 5*time.Second, 50*time.Millisecond).Should(Equal(http.StatusOK))
}

func newInsecureHTTP2Client() *http.Client {
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Client{Transport: &http.Transport{Protocols: protocols}}
}

func (i *instance) statusClient(httpClient connect.HTTPClient, basePath string, opts ...connect.ClientOption) statusv3connect.StatusServiceClient {
	return statusv3connect.NewStatusServiceClient(httpClient, i.baseURL+basePath, opts...)
}
