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

package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	commoncfg "github.com/prometheus/common/config"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
)

// Email implements a Notifier for email notifications.
type Email struct {
	conf   *config.EmailConfig
	tmpl   *template.Template
	logger log.Logger
}

// New returns a new Email notifier.
func New(c *config.EmailConfig, t *template.Template, l log.Logger) *Email {
	if _, ok := c.Headers["Subject"]; !ok {
		c.Headers["Subject"] = config.DefaultEmailSubject
	}
	if _, ok := c.Headers["To"]; !ok {
		c.Headers["To"] = c.To
	}
	if _, ok := c.Headers["From"]; !ok {
		c.Headers["From"] = c.From
	}
	return &Email{conf: c, tmpl: t, logger: l}
}

// auth resolves a string of authentication mechanisms.
func (n *Email) auth(mechs string) (smtp.Auth, error) {
	username := n.conf.AuthUsername

	// If no username is set, keep going without authentication.
	if n.conf.AuthUsername == "" {
		level.Debug(n.logger).Log("msg", "smtp_auth_username is not configured. Attempting to send email without authenticating")
		return nil, nil
	}

	err := &types.MultiError{}
	for _, mech := range strings.Split(mechs, " ") {
		switch mech {
		case "CRAM-MD5":
			secret := string(n.conf.AuthSecret)
			if secret == "" {
				err.Add(errors.New("missing secret for CRAM-MD5 auth mechanism"))
				continue
			}
			return smtp.CRAMMD5Auth(username, secret), nil

		case "PLAIN":
			password := string(n.conf.AuthPassword)
			if password == "" {
				err.Add(errors.New("missing password for PLAIN auth mechanism"))
				continue
			}
			identity := n.conf.AuthIdentity

			// We need to know the hostname for both auth and TLS.
			host, _, err := net.SplitHostPort(n.conf.Smarthost)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %s", err)
			}
			return smtp.PlainAuth(identity, username, password, host), nil
		case "LOGIN":
			password := string(n.conf.AuthPassword)
			if password == "" {
				err.Add(errors.New("missing password for LOGIN auth mechanism"))
				continue
			}
			return LoginAuth(username, password), nil
		}
	}
	if err.Len() == 0 {
		err.Add(errors.New("unknown auth mechanism: " + mechs))
	}
	return nil, err
}

// Notify implements the Notifier interface.
func (n *Email) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	// We need to know the hostname for both auth and TLS.
	var c *smtp.Client
	host, port, err := net.SplitHostPort(n.conf.Smarthost)
	if err != nil {
		return false, fmt.Errorf("invalid address: %s", err)
	}

	if port == "465" {
		tlsConfig, err := commoncfg.NewTLSConfig(&n.conf.TLSConfig)
		if err != nil {
			return false, err
		}
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = host
		}

		conn, err := tls.Dial("tcp", n.conf.Smarthost, tlsConfig)
		if err != nil {
			return true, err
		}
		c, err = smtp.NewClient(conn, host)
		if err != nil {
			return true, err
		}
	} else {
		// Connect to the SMTP smarthost.
		c, err = smtp.Dial(n.conf.Smarthost)
		if err != nil {
			return true, err
		}
	}
	defer func() {
		if err := c.Quit(); err != nil {
			level.Error(n.logger).Log("msg", "failed to close SMTP connection", "err", err)
		}
	}()

	if n.conf.Hello != "" {
		err := c.Hello(n.conf.Hello)
		if err != nil {
			return true, err
		}
	}

	// Global Config guarantees RequireTLS is not nil.
	if *n.conf.RequireTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return true, fmt.Errorf("require_tls: true (default), but %q does not advertise the STARTTLS extension", n.conf.Smarthost)
		}

		tlsConf, err := commoncfg.NewTLSConfig(&n.conf.TLSConfig)
		if err != nil {
			return false, err
		}
		if tlsConf.ServerName == "" {
			tlsConf.ServerName = host
		}

		if err := c.StartTLS(tlsConf); err != nil {
			return true, fmt.Errorf("starttls failed: %s", err)
		}
	}

	if ok, mech := c.Extension("AUTH"); ok {
		auth, err := n.auth(mech)
		if err != nil {
			return true, err
		}
		if auth != nil {
			if err := c.Auth(auth); err != nil {
				return true, fmt.Errorf("%T failed: %s", auth, err)
			}
		}
	}

	var (
		tmplErr error
		data    = notify.GetTemplateData(ctx, n.tmpl, as, n.logger)
		tmpl    = notify.TmplText(n.tmpl, data, &tmplErr)
		from    = tmpl(n.conf.From)
		to      = tmpl(n.conf.To)
	)
	if tmplErr != nil {
		return false, fmt.Errorf("failed to template 'from' or 'to': %v", tmplErr)
	}

	addrs, err := mail.ParseAddressList(from)
	if err != nil {
		return false, fmt.Errorf("parsing from addresses: %s", err)
	}
	if len(addrs) != 1 {
		return false, fmt.Errorf("must be exactly one from address")
	}
	if err := c.Mail(addrs[0].Address); err != nil {
		return true, fmt.Errorf("sending mail from: %s", err)
	}
	addrs, err = mail.ParseAddressList(to)
	if err != nil {
		return false, fmt.Errorf("parsing to addresses: %s", err)
	}
	for _, addr := range addrs {
		if err := c.Rcpt(addr.Address); err != nil {
			return true, fmt.Errorf("sending rcpt to: %s", err)
		}
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return true, err
	}
	defer wc.Close()

	buffer := &bytes.Buffer{}
	for header, t := range n.conf.Headers {
		value, err := n.tmpl.ExecuteTextString(t, data)
		if err != nil {
			return false, fmt.Errorf("executing %q header template: %s", header, err)
		}
		fmt.Fprintf(buffer, "%s: %s\r\n", header, mime.QEncoding.Encode("utf-8", value))
	}

	multipartBuffer := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(multipartBuffer)

	fmt.Fprintf(buffer, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(buffer, "Content-Type: multipart/alternative;  boundary=%s\r\n", multipartWriter.Boundary())
	fmt.Fprintf(buffer, "MIME-Version: 1.0\r\n\r\n")

	// TODO: Add some useful headers here, such as URL of the alertmanager
	// and active/resolved.
	_, err = wc.Write(buffer.Bytes())
	if err != nil {
		return false, fmt.Errorf("failed to write header buffer: %v", err)
	}

	if len(n.conf.Text) > 0 {
		// Text template
		w, err := multipartWriter.CreatePart(textproto.MIMEHeader{
			"Content-Transfer-Encoding": {"quoted-printable"},
			"Content-Type":              {"text/plain; charset=UTF-8"},
		})
		if err != nil {
			return false, fmt.Errorf("creating part for text template: %s", err)
		}
		body, err := n.tmpl.ExecuteTextString(n.conf.Text, data)
		if err != nil {
			return false, fmt.Errorf("executing email text template: %s", err)
		}
		qw := quotedprintable.NewWriter(w)
		_, err = qw.Write([]byte(body))
		if err != nil {
			return true, err
		}
		err = qw.Close()
		if err != nil {
			return true, err
		}
	}

	if len(n.conf.HTML) > 0 {
		// Html template
		// Preferred alternative placed last per section 5.1.4 of RFC 2046
		// https://www.ietf.org/rfc/rfc2046.txt
		w, err := multipartWriter.CreatePart(textproto.MIMEHeader{
			"Content-Transfer-Encoding": {"quoted-printable"},
			"Content-Type":              {"text/html; charset=UTF-8"},
		})
		if err != nil {
			return false, fmt.Errorf("creating part for html template: %s", err)
		}
		body, err := n.tmpl.ExecuteHTMLString(n.conf.HTML, data)
		if err != nil {
			return false, fmt.Errorf("executing email html template: %s", err)
		}
		qw := quotedprintable.NewWriter(w)
		_, err = qw.Write([]byte(body))
		if err != nil {
			return true, err
		}
		err = qw.Close()
		if err != nil {
			return true, err
		}
	}

	err = multipartWriter.Close()
	if err != nil {
		return false, fmt.Errorf("failed to close multipartWriter: %v", err)
	}

	_, err = wc.Write(multipartBuffer.Bytes())
	if err != nil {
		return false, fmt.Errorf("failed to write body buffer: %v", err)
	}

	return false, nil
}

type loginAuth struct {
	username, password string
}

func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

// Used for AUTH LOGIN. (Maybe password should be encrypted)
func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch strings.ToLower(string(fromServer)) {
		case "username:":
			return []byte(a.username), nil
		case "password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unexpected server challenge")
		}
	}
	return nil, nil
}
