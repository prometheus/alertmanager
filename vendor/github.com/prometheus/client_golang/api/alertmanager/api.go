// Copyright 2015 The Prometheus Authors
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

package alertmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	statusAPIError = 422
	apiPrefix      = "/api/v1"

	epSilence  = "/silence/:id"
	epSilences = "/silences"
	epAlerts   = "/alerts"
)

type ErrorType string

const (
	// The different API error types.
	ErrBadData     ErrorType = "bad_data"
	ErrTimeout               = "timeout"
	ErrCanceled              = "canceled"
	ErrExec                  = "execution"
	ErrBadResponse           = "bad_response"
)

// Error is an error returned by the API.
type Error struct {
	Type ErrorType
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Msg)
}

// CancelableTransport is like net.Transport but provides
// per-request cancelation functionality.
type CancelableTransport interface {
	http.RoundTripper
	CancelRequest(req *http.Request)
}

var DefaultTransport CancelableTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	Dial: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).Dial,
	TLSHandshakeTimeout: 10 * time.Second,
}

// Config defines configuration parameters for a new client.
type Config struct {
	// The address of the Prometheus to connect to.
	Address string

	// Transport is used by the Client to drive HTTP requests. If not
	// provided, DefaultTransport will be used.
	Transport CancelableTransport
}

func (cfg *Config) transport() CancelableTransport {
	if cfg.Transport == nil {
		return DefaultTransport
	}
	return cfg.Transport
}

type Client interface {
	url(ep string, args map[string]string) *url.URL
	do(context.Context, *http.Request) (*http.Response, []byte, error)
}

// New returns a new Client.
func New(cfg Config) (Client, error) {
	u, err := url.Parse(cfg.Address)
	if err != nil {
		return nil, err
	}
	u.Path = apiPrefix

	return &httpClient{
		endpoint:  u,
		transport: cfg.transport(),
	}, nil
}

type httpClient struct {
	endpoint  *url.URL
	transport CancelableTransport
}

func (c *httpClient) url(ep string, args map[string]string) *url.URL {
	p := path.Join(c.endpoint.Path, ep)

	for arg, val := range args {
		arg = ":" + arg
		p = strings.Replace(p, arg, val, -1)
	}

	u := *c.endpoint
	u.Path = p

	return &u
}

func (c *httpClient) do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	resp, err := ctxhttp.Do(ctx, &http.Client{Transport: c.transport}, req)

	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	if err != nil {
		return resp, nil, err
	}

	var body []byte
	done := make(chan struct{})
	go func() {
		body, err = ioutil.ReadAll(resp.Body)
		close(done)
	}()

	select {
	case <-ctx.Done():
		err = resp.Body.Close()
		<-done
		if err == nil {
			err = ctx.Err()
		}
	case <-done:
	}

	return resp, body, err
}

// apiClient wraps a regular client and processes successful API responses.
// Successful also includes responses that errored at the API level.
type apiClient struct {
	Client
}

type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType ErrorType       `json:"errorType"`
	Error     string          `json:"error"`
}

func (c apiClient) do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	resp, body, err := c.Client.do(ctx, req)
	if err != nil {
		return resp, body, err
	}

	code := resp.StatusCode

	if code/100 != 2 && code != statusAPIError {
		return resp, body, &Error{
			Type: ErrBadResponse,
			Msg:  fmt.Sprintf("bad response code %d", resp.StatusCode),
		}
	}

	var result apiResponse

	if err = json.Unmarshal(body, &result); err != nil {
		return resp, body, &Error{
			Type: ErrBadResponse,
			Msg:  err.Error(),
		}
	}

	if (code == statusAPIError) != (result.Status == "error") {
		err = &Error{
			Type: ErrBadResponse,
			Msg:  "inconsistent body for response code",
		}
	}

	if code == statusAPIError && result.Status == "error" {
		err = &Error{
			Type: result.ErrorType,
			Msg:  result.Error,
		}
	}

	return resp, []byte(result.Data), err
}

type AlertAPI interface {
	// Push a list of alerts into the Alertmanager.
	Push(ctx context.Context, alerts ...*model.Alert) error
}

// NewAlertAPI returns a new AlertAPI for the client.
func NewAlertAPI(c Client) AlertAPI {
	return &httpAlertAPI{client: apiClient{c}}
}

type httpAlertAPI struct {
	client Client
}

func (h *httpAlertAPI) Push(ctx context.Context, alerts ...*model.Alert) error {
	u := h.client.url(epAlerts, nil)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(alerts); err != nil {
		return err
	}

	req, _ := http.NewRequest("POST", u.String(), &buf)

	_, _, err := h.client.do(ctx, req)
	return err
}

// SilenceAPI provides bindings the Alertmanager's silence API.
type SilenceAPI interface {
	// Get returns the silence associated with the given ID.
	Get(ctx context.Context, id uint64) (*model.Silence, error)
	// Set updates or creates the given silence and returns its ID.
	Set(ctx context.Context, sil *model.Silence) (uint64, error)
	// Del deletes the silence with the given ID.
	Del(ctx context.Context, id uint64) error
	// List all silences of the server.
	List(ctx context.Context) ([]*model.Silence, error)
}

// NewSilenceAPI returns a new SilenceAPI for the client.
func NewSilenceAPI(c Client) SilenceAPI {
	return &httpSilenceAPI{client: apiClient{c}}
}

type httpSilenceAPI struct {
	client Client
}

func (h *httpSilenceAPI) Get(ctx context.Context, id uint64) (*model.Silence, error) {
	u := h.client.url(epSilence, map[string]string{
		"id": strconv.FormatUint(id, 10),
	})

	req, _ := http.NewRequest("GET", u.String(), nil)

	_, body, err := h.client.do(ctx, req)
	if err != nil {
		return nil, err
	}

	var sil model.Silence
	err = json.Unmarshal(body, &sil)

	return &sil, err
}

func (h *httpSilenceAPI) Del(ctx context.Context, id uint64) error {
	u := h.client.url(epSilence, map[string]string{
		"id": strconv.FormatUint(id, 10),
	})

	req, _ := http.NewRequest("DELETE", u.String(), nil)

	_, _, err := h.client.do(ctx, req)
	return err
}

func (h *httpSilenceAPI) Set(ctx context.Context, sil *model.Silence) (uint64, error) {
	var (
		u      *url.URL
		method string
	)

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(sil); err != nil {
		return 0, err
	}

	// Talk to different endpoints depending on whether its a new silence or not.
	if sil.ID != 0 {
		u = h.client.url(epSilence, map[string]string{
			"id": strconv.FormatUint(sil.ID, 10),
		})
		method = "PUT"
	} else {
		u = h.client.url(epSilences, nil)
		method = "POST"
	}

	req, _ := http.NewRequest(method, u.String(), &buf)

	_, body, err := h.client.do(ctx, req)
	if err != nil {
		return 0, err
	}

	var res struct {
		SilenceID uint64 `json:"silenceId"`
	}
	err = json.Unmarshal(body, &res)

	return res.SilenceID, err
}

func (h *httpSilenceAPI) List(ctx context.Context) ([]*model.Silence, error) {
	u := h.client.url(epSilences, nil)

	req, _ := http.NewRequest("GET", u.String(), nil)

	_, body, err := h.client.do(ctx, req)
	if err != nil {
		return nil, err
	}

	var sils []*model.Silence
	err = json.Unmarshal(body, &sils)

	return sils, err
}
