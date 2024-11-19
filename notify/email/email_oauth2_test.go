// Copyright 2019 Prometheus Team
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

// Some tests require a working OAUTH2 smtp server.
// At the time of writing, the only available server are Google's and Microsoft's.
// Follow the instructions on the respective pages to set up the client configuration:
// * https://learn.microsoft.com/de-de/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth
// * https://developers.google.com/gmail/imap/xoauth2-protocol
//
// To run the tests locally, run:
// $ EMAIL_AUTH_XOAUTH2_CONFIG=testdata/auth_xoauth2.yml make
package email

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"

	// nolint:depguard // require cannot be called outside the main goroutine: https://pkg.go.dev/testing#T.FailNow
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/alertmanager/config"
)

const (
	emailAuthXOAuth2ConfigVar = "EMAIL_AUTH_XOAUTH2_CONFIG"

	TestBearerUsername = "fxcp"
	TestBearerToken    = "VkIvciKi9ijpiKNWrQmYCJrzgd9QYCMB"
)

func TestEmail_OAuth2(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	// Setup mock SMTP server which will reject at the DATA stage.
	srv, l, err := mockSMTPServer(t, &xOAuth2Backend{})
	require.NoError(t, err)
	t.Cleanup(func() {
		// We expect that the server has already been closed in the test.
		require.ErrorIs(t, srv.Shutdown(ctx), smtp.ErrServerClosed)
	})

	done := make(chan any, 1)
	go func() {
		// nolint:testifylint // require cannot be called outside the main goroutine: https://pkg.go.dev/testing#T.FailNow
		assert.NoError(t, srv.Serve(l))
		close(done)
	}()

	oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"%s","token_type":"Bearer","expires_in":3600}`, TestBearerToken)
	}))

	// Wait for mock SMTP server to become ready.
	require.Eventuallyf(t, func() bool {
		c, err := smtp.Dial(srv.Addr)
		if err != nil {
			t.Logf("dial failed to %q: %s", srv.Addr, err)
			return false
		}

		// Ping.
		if err = c.Noop(); err != nil {
			t.Logf("ping failed to %q: %s", srv.Addr, err)
			return false
		}

		// Ensure we close the connection to not prevent server from shutting down cleanly.
		if err = c.Close(); err != nil {
			t.Logf("close failed to %q: %s", srv.Addr, err)
			return false
		}

		return true
	}, time.Second*10, time.Millisecond*100, "mock SMTP server failed to start")

	// Use mock SMTP server and prepare alert to be sent.
	require.IsType(t, &net.TCPAddr{}, l.Addr())
	addr := l.Addr().(*net.TCPAddr)
	cfg := &config.EmailConfig{
		Smarthost:    config.HostPort{Host: addr.IP.String(), Port: strconv.Itoa(addr.Port)},
		Hello:        "localhost",
		Headers:      make(map[string]string),
		From:         "alertmanager@system",
		To:           "sre@company",
		AuthUsername: TestBearerUsername,
		AuthXOAuth2: &commoncfg.OAuth2{
			ClientID:     "client_id",
			ClientSecret: "client_secret",
			TokenURL:     oidcServer.URL,
			Scopes:       []string{"email"},
		},
	}

	tmpl, firingAlert, err := prepare(cfg)
	require.NoError(t, err)

	e := New(cfg, tmpl, promslog.NewNopLogger())

	// Send the alert to mock SMTP server.
	retry, err := e.Notify(context.Background(), firingAlert)
	require.ErrorContains(t, err, "501 5.5.4 Rejected!")
	require.True(t, retry)
	require.NoError(t, srv.Shutdown(ctx))

	require.Eventuallyf(t, func() bool {
		<-done
		return true
	}, time.Second*10, time.Millisecond*100, "mock SMTP server goroutine failed to close in time")
}

// xOAuth2Backend will reject submission at the DATA stage.
type xOAuth2Backend struct{}

func (b *xOAuth2Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &mockSMTPxOAuth2Session{
		conn:    c,
		backend: b,
	}, nil
}

type mockSMTPxOAuth2Session struct {
	conn    *smtp.Conn
	backend smtp.Backend
}

func (s *mockSMTPxOAuth2Session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.Login, "XOAUTH2"}
}

func (s *mockSMTPxOAuth2Session) Auth(string) (sasl.Server, error) {
	return &xOAuth2BackendAuth{}, nil
}

func (s *mockSMTPxOAuth2Session) Mail(string, *smtp.MailOptions) error {
	return nil
}

func (s *mockSMTPxOAuth2Session) Rcpt(string, *smtp.RcptOptions) error {
	return nil
}

func (s *mockSMTPxOAuth2Session) Data(io.Reader) error {
	return &smtp.SMTPError{Code: 501, EnhancedCode: smtp.EnhancedCode{5, 5, 4}, Message: "Rejected!"}
}

func (*mockSMTPxOAuth2Session) Reset() {}

func (*mockSMTPxOAuth2Session) Logout() error { return nil }

type xOAuth2BackendAuth struct{}

func (*xOAuth2BackendAuth) Next(response []byte) ([]byte, bool, error) {
	// Generate empty challenge.
	if response == nil {
		return []byte{}, false, nil
	}

	token := make([]byte, base64.RawStdEncoding.DecodedLen(len(response)))

	_, err := base64.RawStdEncoding.Decode(token, response)
	if err != nil {
		return nil, true, err
	}

	expectedToken := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", TestBearerUsername, TestBearerToken)
	if expectedToken == string(token) {
		return nil, true, nil
	}

	return nil, true, fmt.Errorf("unexpected token: %s, expected: %s", token, expectedToken)
}

// TestEmailNotifyWithXOAuth2Authentication sends emails to an instance of MailDev
// configured with authentication.
func TestEmailNotifyWithXOAuth2Authentication(t *testing.T) {
	cfgFile := os.Getenv(emailAuthXOAuth2ConfigVar)
	if len(cfgFile) == 0 {
		t.Skipf("%s not set", emailAuthXOAuth2ConfigVar)
	}

	c, err := loadEmailTestConfiguration(cfgFile)
	if err != nil {
		t.Fatal(err)
	}

	fileWithCorrectClientSecret, err := os.CreateTemp("", "client-secret-correct")
	require.NoError(t, err, "creating temp file failed")
	_, err = fileWithCorrectClientSecret.WriteString(string(c.XOAuth2.ClientSecret))
	require.NoError(t, err, "writing to temp file failed")

	fileWithIncorrectClientSecret, err := os.CreateTemp("", "client-secret-incorrect")
	require.NoError(t, err, "creating temp file failed")
	_, err = fileWithIncorrectClientSecret.WriteString(string(c.XOAuth2.ClientSecret) + "wrong")
	require.NoError(t, err, "writing to temp file failed")

	for _, tc := range []struct {
		title     string
		updateCfg func(*config.EmailConfig)

		errMsg string
		retry  bool
	}{
		{
			title: "email with authentication",
			updateCfg: func(cfg *config.EmailConfig) {
				cfg.AuthUsername = c.Username
				cfg.AuthXOAuth2 = c.XOAuth2
			},
		},
		{
			title: "email with authentication (password from file)",
			updateCfg: func(cfg *config.EmailConfig) {
				cfg.AuthUsername = c.Username
				cfg.AuthXOAuth2 = c.XOAuth2
				cfg.AuthXOAuth2.ClientSecret = ""
				cfg.AuthXOAuth2.ClientSecretFile = fileWithCorrectClientSecret.Name()
			},
		},
		{
			title: "wrong credentials",
			updateCfg: func(cfg *config.EmailConfig) {
				cfg.AuthUsername = c.Username
				cfg.AuthXOAuth2 = c.XOAuth2
				cfg.AuthXOAuth2.ClientSecret = cfg.AuthXOAuth2.ClientSecret + "wrong"
			},

			errMsg: "Invalid username or password",
			retry:  true,
		},
		{
			title: "wrong credentials (password from file)",
			updateCfg: func(cfg *config.EmailConfig) {
				cfg.AuthUsername = c.Username
				cfg.AuthXOAuth2 = c.XOAuth2
				cfg.AuthXOAuth2.ClientSecret = ""
				cfg.AuthXOAuth2.ClientSecretFile = fileWithIncorrectClientSecret.Name()
			},

			errMsg: "Invalid username or password",
			retry:  true,
		},
		{
			title: "wrong credentials (missing password file)",
			updateCfg: func(cfg *config.EmailConfig) {
				cfg.AuthUsername = c.Username
				cfg.AuthXOAuth2 = c.XOAuth2
				cfg.AuthXOAuth2.ClientSecret = ""
				cfg.AuthXOAuth2.ClientSecretFile = "/does/not/exist"
			},

			errMsg: "could not read",
			retry:  true,
		},
		{
			title:  "no credentials",
			errMsg: "authentication Required",
			retry:  true,
		},
	} {
		tc := tc
		t.Run(tc.title, func(t *testing.T) {
			emailCfg := &config.EmailConfig{
				TLSConfig: &commoncfg.TLSConfig{},
				Smarthost: c.Smarthost,
				To:        emailTo,
				From:      emailFrom,
				HTML:      "HTML body",
				Text:      "Text body",
				Headers: map[string]string{
					"Subject": "{{ len .Alerts }} {{ .Status }} alert(s)",
				},
			}

			if c.Smarthost.Port == "587" {
				requireTLS := true
				emailCfg.RequireTLS = &requireTLS
			}

			if tc.updateCfg != nil {
				tc.updateCfg(emailCfg)
			}

			tmpl, firingAlert, err := prepare(emailCfg)
			require.NoError(t, err)

			email := New(emailCfg, tmpl, promslog.NewNopLogger())

			retry, err := email.Notify(context.Background(), firingAlert)
			require.NoError(t, err)
			require.Equal(t, tc.retry, retry)
		})
	}
}
