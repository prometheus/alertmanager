// Copyright 2024 Prometheus Team
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

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/exporter-toolkit/web"

	"github.com/prometheus/alertmanager/featurecontrol"
)

func TestStartupWithHTTPConfig(t *testing.T) {
	// Start a HTTP server that returns a minimal valid config.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("route:\n  receiver: test\nreceivers:\n- name: test"))
	}))
	defer srv.Close()

	// Create a temporary data directory.
	dir := t.TempDir()

	// Build minimal options for HTTP source.
	webCfg := web.FlagConfig{}
	webCfg.WebListenAddresses = &[]string{"127.0.0.1:0"}
	webCfgFile := ""
	webCfg.WebConfigFile = &webCfgFile

	ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), "")
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		ConfigHTTPURL:               srv.URL,
		DataDir:                     dir,
		Retention:                   DefaultRetention,
		MaintenanceInterval:         DefaultMaintenanceInterval,
		AlertGCInterval:             DefaultAlertGCInterval,
		DispatchMaintenanceInterval: DefaultDispatchMaintenanceInterval,
		WebConfig:                   &webCfg,
		Logger:                      promslog.NewNopLogger(),
		Registerer:                  prometheus.NewRegistry(),
		Flagger:                     ff,
	}

	// Try to create the app (setup).
	app, err := New(opts)
	if err != nil {
		t.Fatalf("failed to create app with HTTP config: %v", err)
	}
	defer func() { _ = app.Stop(context.Background()) }()

	// Start the app.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = app.Start() }()

	// Verify it started without error.
	select {
	case <-ctx.Done():
		t.Fatal("app stopped unexpectedly")
	default:
	}
}

func TestStartupWithFileConfig(t *testing.T) {
	// Create a temporary config file.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "alertmanager.yml")
	data := []byte("route:\n  receiver: test\nreceivers:\n- name: test")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	webCfg := web.FlagConfig{}
	webCfg.WebListenAddresses = &[]string{"127.0.0.1:0"}
	webCfgFile := ""
	webCfg.WebConfigFile = &webCfgFile

	ff, err := featurecontrol.NewFlags(promslog.NewNopLogger(), "")
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		ConfigFile:                  configPath,
		DataDir:                     dir,
		Retention:                   DefaultRetention,
		MaintenanceInterval:         DefaultMaintenanceInterval,
		AlertGCInterval:             DefaultAlertGCInterval,
		DispatchMaintenanceInterval: DefaultDispatchMaintenanceInterval,
		WebConfig:                   &webCfg,
		Logger:                      promslog.NewNopLogger(),
		Registerer:                  prometheus.NewRegistry(),
		Flagger:                     ff,
	}

	app, err := New(opts)
	if err != nil {
		t.Fatalf("failed to create app with file config: %v", err)
	}
	defer func() { _ = app.Stop(context.Background()) }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = app.Start() }()

	select {
	case <-ctx.Done():
		t.Fatal("app stopped unexpectedly")
	default:
	}
}

func TestStartupWithBothSources(t *testing.T) {
	// Create a temporary config file.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "alertmanager.yml")
	data := []byte("route:\n  receiver: test\nreceivers:\n- name: test")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	webCfg := web.FlagConfig{}
	webCfg.WebListenAddresses = &[]string{"127.0.0.1:0"}
	webCfgFile := ""
	webCfg.WebConfigFile = &webCfgFile
	ff, _ := featurecontrol.NewFlags(promslog.NewNopLogger(), "")
	opts := Options{
		ConfigFile:                  configPath,
		ConfigHTTPURL:               "http://example.com/config",
		DataDir:                     dir,
		Retention:                   DefaultRetention,
		MaintenanceInterval:         DefaultMaintenanceInterval,
		AlertGCInterval:             DefaultAlertGCInterval,
		DispatchMaintenanceInterval: DefaultDispatchMaintenanceInterval,
		WebConfig:                   &webCfg,
		Logger:                      promslog.NewNopLogger(),
		Registerer:                  prometheus.NewRegistry(),
		Flagger:                     ff,
	}

	_, err := New(opts)
	if err == nil {
		t.Fatal("expected error when both config sources are set")
	}
	if err.Error() != "alertmanager/app: Options.ConfigFile and Options.ConfigHTTPURL are mutually exclusive" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestStartupWithNeitherSource(t *testing.T) {
	webCfg := web.FlagConfig{}
	webCfg.WebListenAddresses = &[]string{"127.0.0.1:0"}
	webCfgFile := ""
	webCfg.WebConfigFile = &webCfgFile
	ff, _ := featurecontrol.NewFlags(promslog.NewNopLogger(), "")
	opts := Options{
		DataDir:                     t.TempDir(),
		Retention:                   DefaultRetention,
		MaintenanceInterval:         DefaultMaintenanceInterval,
		AlertGCInterval:             DefaultAlertGCInterval,
		DispatchMaintenanceInterval: DefaultDispatchMaintenanceInterval,
		WebConfig:                   &webCfg,
		Logger:                      promslog.NewNopLogger(),
		Registerer:                  prometheus.NewRegistry(),
		Flagger:                     ff,
	}

	_, err := New(opts)
	if err == nil {
		t.Fatal("expected error when no config source is set")
	}
	if err.Error() != "alertmanager/app: exactly one of Options.ConfigFile or Options.ConfigHTTPURL must be set" {
		t.Fatalf("unexpected error message: %v", err)
	}
}
