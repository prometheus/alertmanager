// Copyright 2013 Prometheus Team
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

package manager

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestWriteEmailBody(t *testing.T) {
	event := &Alert{
		Summary:     "Testsummary",
		Description: "Test alert description, something went wrong here.",
		Labels: AlertLabelSet{
			"alertname":       "TestAlert",
			"grouping_label1": "grouping_value1",
			"grouping_label2": "grouping_value2",
		},
		Payload: AlertPayload{
			"payload_label1": "payload_value1",
			"payload_label2": "payload_value2",
		},
	}
	buf := &bytes.Buffer{}
	location, _ := time.LoadLocation("Europe/Amsterdam")
	moment := time.Date(1918, 11, 11, 11, 0, 3, 0, location)
	writeEmailBodyWithTime(buf, "from@prometheus.io", "to@prometheus.io", "ALERT", event, moment)

	expected := `From: Prometheus Alertmanager <from@prometheus.io>
To: to@prometheus.io
Date: Mon, 11 Nov 1918 11:00:03 +0019
Subject: [ALERT] TestAlert: Testsummary

Test alert description, something went wrong here.

Grouping labels:

  alertname = "TestAlert"
  grouping_label1 = "grouping_value1"
  grouping_label2 = "grouping_value2"

Payload labels:

  payload_label1 = "payload_value1"
  payload_label2 = "payload_value2"`

	if buf.String() != expected {
		t.Fatalf("Expected:\n%s\n\nGot:\n%s", expected, buf.String())
	}
}

type authTestCase struct {
	hasAuth     bool
	mechs       string
	expAuthType string
}

func (tc *authTestCase) test(t *testing.T) {
	auth, cfg, err := getSMTPAuth(tc.hasAuth, tc.mechs)
	if err != nil {
		tc.fail(t, "unexpected error: %s", err)
		return
	}
	if tc.expAuthType == "" {
		if auth != nil {
			tc.fail(t, "expected auth to be nil, got %T", auth)
		}
		if cfg != nil {
			tc.fail(t, "expected tls config to be nil, got %v", cfg)
		}
	} else {
		if fmt.Sprintf("%T", auth) != tc.expAuthType {
			tc.fail(t, "expected auth to be %s, got %T", tc.expAuthType, auth)
		}
		if tc.expAuthType == "*smtp.plainAuth" {
			if cfg == nil {
				tc.fail(t, "expected tls config")
			} else if cfg.ServerName != "testSMTPHost" {
				tc.fail(t, "expected tls config to be %v, got %v",
					&tls.Config{ServerName: "testSMTPHost"}, cfg)
			}
		}
	}
}

func (tc *authTestCase) fail(t *testing.T, format string, args ...interface{}) {
	t.Errorf("{auth test: %#v}: %s", tc, fmt.Sprintf(format, args...))
}

func runAuthTests(t *testing.T, tcs []authTestCase) {
	for _, tc := range tcs {
		tc.test(t)
	}
}

func TestGetSMTPAuth(t *testing.T) {
	// Save and clear environment.
	vars := []string{"SMTP_AUTH_USERNAME", "SMTP_AUTH_PASSWORD", "SMTP_AUTH_SECRET", "SMTP_AUTH_IDENTITY"}
	saved := map[string]string{}
	for _, k := range vars {
		saved[k] = os.Getenv(k)
		os.Setenv(k, "")
	}

	// Set up deferred restoration of environment.
	defer func() {
		for k, v := range saved {
			os.Setenv(k, v)
		}
	}()

	// Auth never occurs without environment variables set.
	runAuthTests(t, []authTestCase{
		{false, "", ""},
		{false, "PLAIN", ""},
		{false, "CRAM-MD5", ""},
	})

	// Simple single-mechanism cases.
	*smtpSmartHost = "testSMTPHost:port"
	os.Setenv("SMTP_AUTH_USERNAME", "u")
	os.Setenv("SMTP_AUTH_PASSWORD", "p") // for PLAIN
	os.Setenv("SMTP_AUTH_SECRET", "s")   // for CRAM-MD5
	runAuthTests(t, []authTestCase{
		{true, "", ""},
		{true, "CRAM-MD5", "*smtp.cramMD5Auth"},
		{true, "PLAIN", "*smtp.plainAuth"},

		// If all mechanisms are valid, return the first one that qualifies.
		{true, "CRAM-MD5 PLAIN", "*smtp.cramMD5Auth"},
		{true, "PLAIN CRAM-MD5", "*smtp.plainAuth"},
	})

	// Skip CRAM-MD5.
	os.Setenv("SMTP_AUTH_SECRET", "")
	runAuthTests(t, []authTestCase{
		{true, "CRAM-MD5 PLAIN", "*smtp.plainAuth"},
	})

	// Skip PLAIN.
	os.Setenv("SMTP_AUTH_PASSWORD", "")
	os.Setenv("SMTP_AUTH_SECRET", "s")
	runAuthTests(t, []authTestCase{
		{true, "PLAIN CRAM-MD5", "*smtp.cramMD5Auth"},
	})

	// Error should be returned if we try to use PLAIN auth with an invalid -smtpSmartHost.
	*smtpSmartHost = "testSMTPHost"
	runAuthTests(t, []authTestCase{
		{true, "PLAIN", ""},
	})
	os.Setenv("SMTP_AUTH_PASSWORD", "p")
	if auth, cfg, err := getSMTPAuth(true, "PLAIN"); err == nil {
		t.Errorf("PLAIN auth with bad host-port: expected error but got %T, %v", auth, cfg)
	}
}
