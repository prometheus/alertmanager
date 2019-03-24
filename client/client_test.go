// Copyright 2018 The Prometheus Authors
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

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

type apiTest struct {
	// Wrapper around the tested function.
	do func() (interface{}, error)

	apiRes fakeAPIResponse

	// Expected values returned by the tested function.
	res interface{}
	err error
}

// Fake HTTP client for TestAPI.
type fakeAPIClient struct {
	*testing.T
	ch chan fakeAPIResponse
}

type fakeAPIResponse struct {
	// Expected input values.
	path   string
	method string

	// Values to be returned by fakeAPIClient.Do().
	err error
	res interface{}
}

func (c *fakeAPIClient) URL(ep string, args map[string]string) *url.URL {
	path := ep
	for k, v := range args {
		path = strings.Replace(path, ":"+k, v, -1)
	}

	return &url.URL{
		Host: "test:9093",
		Path: path,
	}
}

func (c *fakeAPIClient) Do(ctx context.Context, req *http.Request) (*http.Response, []byte, error) {
	test := <-c.ch

	if req.URL.Path != test.path {
		c.Errorf("unexpected request path: want %s, got %s", test.path, req.URL.Path)
	}
	if req.Method != test.method {
		c.Errorf("unexpected request method: want %s, got %s", test.method, req.Method)
	}

	b, err := json.Marshal(test.res)
	if err != nil {
		c.Fatal(err)
	}

	return &http.Response{}, b, test.err
}

func TestAPI(t *testing.T) {
	client := &fakeAPIClient{T: t, ch: make(chan fakeAPIResponse, 1)}
	now := time.Now()

	statusData := &ServerStatus{
		ConfigYAML:    "{}",
		ConfigJSON:    &config.Config{},
		VersionInfo:   map[string]string{"version": "v1"},
		Uptime:        now,
		ClusterStatus: &ClusterStatus{Peers: []PeerStatus{}},
	}
	doStatus := func() (interface{}, error) {
		api := httpStatusAPI{client: client}
		return api.Get(context.Background())
	}

	alertOne := Alert{
		StartsAt:    now,
		EndsAt:      now.Add(time.Duration(5 * time.Minute)),
		Labels:      LabelSet{"label1": "test1"},
		Annotations: LabelSet{"annotation1": "some text"},
	}
	alerts := []*ExtendedAlert{
		{
			Alert:       alertOne,
			Fingerprint: "1c93eec3511dc156",
			Status: types.AlertStatus{
				State: types.AlertStateActive,
			},
		},
	}
	doAlertList := func() (interface{}, error) {
		api := httpAlertAPI{client: client}
		return api.List(context.Background(), "", "", false, false, false, false)
	}
	doAlertPush := func() (interface{}, error) {
		api := httpAlertAPI{client: client}
		return nil, api.Push(context.Background(), []Alert{alertOne}...)
	}

	silOne := &types.Silence{
		ID: "abc",
		Matchers: []*types.Matcher{
			{
				Name:    "label1",
				Value:   "test1",
				IsRegex: false,
			},
		},
		StartsAt:  now,
		EndsAt:    now.Add(time.Duration(2 * time.Hour)),
		UpdatedAt: now,
		CreatedBy: "alice",
		Comment:   "some comment",
		Status: types.SilenceStatus{
			State: "active",
		},
	}
	doSilenceGet := func(id string) func() (interface{}, error) {
		return func() (interface{}, error) {
			api := httpSilenceAPI{client: client}
			return api.Get(context.Background(), id)
		}
	}
	doSilenceSet := func(sil types.Silence) func() (interface{}, error) {
		return func() (interface{}, error) {
			api := httpSilenceAPI{client: client}
			return api.Set(context.Background(), sil)
		}
	}
	doSilenceExpire := func(id string) func() (interface{}, error) {
		return func() (interface{}, error) {
			api := httpSilenceAPI{client: client}
			return nil, api.Expire(context.Background(), id)
		}
	}
	doSilenceList := func() (interface{}, error) {
		api := httpSilenceAPI{client: client}
		return api.List(context.Background(), "")
	}

	tests := []apiTest{
		{
			do: doStatus,
			apiRes: fakeAPIResponse{
				res:    statusData,
				path:   "/api/v1/status",
				method: http.MethodGet,
			},
			res: statusData,
		},
		{
			do: doStatus,
			apiRes: fakeAPIResponse{
				err:    fmt.Errorf("some error"),
				path:   "/api/v1/status",
				method: http.MethodGet,
			},
			err: fmt.Errorf("some error"),
		},
		{
			do: doAlertList,
			apiRes: fakeAPIResponse{
				res:    alerts,
				path:   "/api/v1/alerts",
				method: http.MethodGet,
			},
			res: alerts,
		},
		{
			do: doAlertList,
			apiRes: fakeAPIResponse{
				err:    fmt.Errorf("some error"),
				path:   "/api/v1/alerts",
				method: http.MethodGet,
			},
			err: fmt.Errorf("some error"),
		},
		{
			do: doAlertPush,
			apiRes: fakeAPIResponse{
				res:    nil,
				path:   "/api/v1/alerts",
				method: http.MethodPost,
			},
			res: nil,
		},
		{
			do: doAlertPush,
			apiRes: fakeAPIResponse{
				err:    fmt.Errorf("some error"),
				path:   "/api/v1/alerts",
				method: http.MethodPost,
			},
			err: fmt.Errorf("some error"),
		},
		{
			do: doSilenceGet("abc"),
			apiRes: fakeAPIResponse{
				res:    silOne,
				path:   "/api/v1/silence/abc",
				method: http.MethodGet,
			},
			res: silOne,
		},
		{
			do: doSilenceGet("abc"),
			apiRes: fakeAPIResponse{
				err:    fmt.Errorf("some error"),
				path:   "/api/v1/silence/abc",
				method: http.MethodGet,
			},
			err: fmt.Errorf("some error"),
		},
		{
			do: doSilenceSet(*silOne),
			apiRes: fakeAPIResponse{
				res:    map[string]string{"SilenceId": "abc"},
				path:   "/api/v1/silences",
				method: http.MethodPost,
			},
			res: "abc",
		},
		{
			do: doSilenceSet(*silOne),
			apiRes: fakeAPIResponse{
				err:    fmt.Errorf("some error"),
				path:   "/api/v1/silences",
				method: http.MethodPost,
			},
			err: fmt.Errorf("some error"),
		},
		{
			do: doSilenceExpire("abc"),
			apiRes: fakeAPIResponse{
				path:   "/api/v1/silence/abc",
				method: http.MethodDelete,
			},
		},
		{
			do: doSilenceExpire("abc"),
			apiRes: fakeAPIResponse{
				err:    fmt.Errorf("some error"),
				path:   "/api/v1/silence/abc",
				method: http.MethodDelete,
			},
			err: fmt.Errorf("some error"),
		},
		{
			do: doSilenceList,
			apiRes: fakeAPIResponse{
				res:    []*types.Silence{silOne},
				path:   "/api/v1/silences",
				method: http.MethodGet,
			},
			res: []*types.Silence{silOne},
		},
		{
			do: doSilenceList,
			apiRes: fakeAPIResponse{
				err:    fmt.Errorf("some error"),
				path:   "/api/v1/silences",
				method: http.MethodGet,
			},
			err: fmt.Errorf("some error"),
		},
	}
	for _, test := range tests {
		test := test
		client.ch <- test.apiRes
		t.Run(fmt.Sprintf("%s %s", test.apiRes.method, test.apiRes.path), func(t *testing.T) {
			res, err := test.do()
			if test.err != nil {
				if err == nil {
					t.Errorf("unexpected error: want: %s but got none", test.err)
					return
				}
				if err.Error() != test.err.Error() {
					t.Errorf("unexpected error: want: %s, got: %s", test.err, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %s", err)
				return
			}
			want, err := json.Marshal(test.res)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(res)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(want, got) {
				t.Errorf("unexpected result: want: %s, got: %s", string(want), string(got))
			}
		})
	}
}

// Fake HTTP client for TestAPIClientDo.
type fakeClient struct {
	*testing.T
	ch chan fakeResponse
}

type fakeResponse struct {
	code int
	res  interface{}
	err  error
}

func (c fakeClient) URL(string, map[string]string) *url.URL {
	return nil
}

func (c fakeClient) Do(context.Context, *http.Request) (*http.Response, []byte, error) {
	fakeRes := <-c.ch

	if fakeRes.err != nil {
		return nil, nil, fakeRes.err
	}

	var b []byte
	var err error
	switch v := fakeRes.res.(type) {
	case string:
		b = []byte(v)
	default:
		b, err = json.Marshal(v)
		if err != nil {
			c.Fatal(err)
		}
	}

	return &http.Response{StatusCode: fakeRes.code}, b, nil
}

type apiClientTest struct {
	response fakeResponse

	expected string
	err      error
}

func TestAPIClientDo(t *testing.T) {
	tests := []apiClientTest{
		{
			response: fakeResponse{
				code: http.StatusOK,
				res: &apiResponse{
					Status: statusSuccess,
					Data:   json.RawMessage(`"test"`),
				},
				err: nil,
			},
			expected: `"test"`,
			err:      nil,
		},
		{
			response: fakeResponse{
				code: http.StatusBadRequest,
				res: &apiResponse{
					Status: statusError,
					Error:  "some error",
				},
				err: nil,
			},
			err: fmt.Errorf("some error (code: 400)"),
		},
		{
			response: fakeResponse{
				code: http.StatusOK,
				res: &apiResponse{
					Status: statusError,
					Error:  "some error",
				},
				err: nil,
			},
			err: fmt.Errorf("inconsistent body for response code (code: 200)"),
		},
		{
			response: fakeResponse{
				code: http.StatusNotFound,
				res:  "not found",
				err:  nil,
			},
			err: fmt.Errorf("not found (code: 404)"),
		},
		{
			response: fakeResponse{
				err: fmt.Errorf("some error"),
			},
			err: fmt.Errorf("some error"),
		},
	}

	fake := fakeClient{T: t, ch: make(chan fakeResponse, 1)}
	client := apiClient{fake}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			fake.ch <- test.response

			_, body, err := client.Do(context.Background(), &http.Request{})
			if test.err != nil {
				if err == nil {
					t.Errorf("expected error %q but got none", test.err)
					return
				}
				if test.err.Error() != err.Error() {
					t.Errorf("unexpected error: want %q, got %q", test.err, err)
					return
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error %q", err)
				return
			}

			want, got := test.expected, string(body)
			if want != got {
				t.Errorf("unexpected body: want %q, got %q", want, got)
			}
		})
	}
}
