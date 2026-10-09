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

package apiconnect

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	statusv3alpha "github.com/prometheus/alertmanager/api/status/v3alpha"
	"github.com/prometheus/alertmanager/api/status/v3alpha/statusv3alphaconnect"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/pkg/labels"
)

func TestSharedMatchers(t *testing.T) {
	t.Parallel()

	t.Run("converts every matcher type in both directions", func(t *testing.T) {
		t.Parallel()

		matcherTypes := []commonv3alpha.MatcherType{
			commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL,
			commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL,
			commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP,
			commonv3alpha.MatcherType_MATCHER_TYPE_NOT_REGEXP,
		}
		for _, matcherType := range matcherTypes {
			t.Run(matcherType.String(), func(t *testing.T) {
				t.Parallel()

				input := &commonv3alpha.Matcher{Name: "service", Value: "api.*", Type: matcherType}
				internal, err := matcherFromProto(input)
				require.NoError(t, err)
				output, err := matcherToProto(internal)
				require.NoError(t, err)
				require.True(t, proto.Equal(input, output))
			})
		}
	})

	t.Run("applies OR across sets and AND within a set", func(t *testing.T) {
		t.Parallel()

		sets, err := matcherSetsFromProto([]*commonv3alpha.MatcherSet{
			{Matchers: []*commonv3alpha.Matcher{
				{Name: "service", Value: "api", Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL},
				{Name: "region", Value: "us-.+", Type: commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP},
			}},
			{Matchers: []*commonv3alpha.Matcher{
				{Name: "severity", Value: "critical", Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL},
			}},
		})
		require.NoError(t, err)
		require.True(t, matchesMatcherSets(sets, model.LabelSet{"service": "api", "region": "us-east"}))
		require.True(t, matchesMatcherSets(sets, model.LabelSet{"severity": "critical"}))
		require.False(t, matchesMatcherSets(sets, model.LabelSet{"service": "api", "region": "eu-west"}))
		require.True(t, matchesMatcherSets(nil, nil))

		output, err := matcherSetsToProto(sets)
		require.NoError(t, err)
		require.Len(t, output, 2)
		require.Len(t, output[0].GetMatchers(), 2)
	})

	t.Run("rejects invalid matchers", func(t *testing.T) {
		t.Parallel()

		_, err := matcherFromProto(nil)
		require.EqualError(t, err, "matcher is required")
		_, err = matcherFromProto(&commonv3alpha.Matcher{Name: "", Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL})
		require.ErrorContains(t, err, "invalid label name")
		_, err = matcherFromProto(&commonv3alpha.Matcher{Name: "service"})
		require.ErrorContains(t, err, "unsupported matcher type")
		_, err = matcherFromProto(&commonv3alpha.Matcher{Name: "service", Value: "[", Type: commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP})
		require.ErrorContains(t, err, "invalid matcher")
		_, err = matcherSetsFromProto([]*commonv3alpha.MatcherSet{nil})
		require.EqualError(t, err, "matcher set 0 is required")
		_, err = matcherToProto(&labels.Matcher{Type: labels.MatchType(99)})
		require.ErrorContains(t, err, "unsupported internal matcher type")
	})
}

func TestSharedFilters(t *testing.T) {
	t.Parallel()

	valid := func(state commonv3alpha.ResourceState) bool {
		return state == commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE || state == commonv3alpha.ResourceState_RESOURCE_STATE_EXPIRED
	}
	empty, err := newValueFilter([]commonv3alpha.ResourceState(nil), valid)
	require.NoError(t, err)
	require.True(t, empty.matches(commonv3alpha.ResourceState_RESOURCE_STATE_PENDING))

	selected, err := newValueFilter([]commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE}, valid)
	require.NoError(t, err)
	require.True(t, selected.matches(commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE))
	require.False(t, selected.matches(commonv3alpha.ResourceState_RESOURCE_STATE_EXPIRED))

	_, err = newValueFilter([]commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_PENDING}, valid)
	require.ErrorContains(t, err, "invalid filter value at index 0")
}

func TestSharedPagination(t *testing.T) {
	t.Parallel()

	newPagination := func(t *testing.T) (*pageTokenCodec, paginationSpec, *time.Time) {
		t.Helper()
		now := time.Unix(1_800_000_000, 0)
		codec := newPageTokenCodec([]byte("test-key"), time.Minute)
		codec.now = func() time.Time { return now }
		digest, err := protoFilterDigest(&commonv3alpha.StateFilter{States: []commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE}})
		require.NoError(t, err)
		return codec, paginationSpec{resource: "alerts", defaultSize: 2, maximumSize: 3, filterDigest: digest}, &now
	}

	t.Run("creates opaque tokens and continues after the last cursor", func(t *testing.T) {
		t.Parallel()

		codec, spec, _ := newPagination(t)
		first, page, err := paginate([]string{"a", "b", "c"}, nil, spec, codec, func(value string) string { return value })
		require.NoError(t, err)
		require.Equal(t, []string{"a", "b"}, first)
		require.NotEmpty(t, page.GetNextPageToken())

		second, page, err := paginate(
			[]string{"a", "b", "bb", "c"},
			&commonv3alpha.PageRequest{PageToken: page.GetNextPageToken()},
			spec,
			codec,
			func(value string) string { return value },
		)
		require.NoError(t, err)
		require.Equal(t, []string{"bb", "c"}, second)
		require.Empty(t, page.GetNextPageToken())
	})

	t.Run("clamps page size and rejects invalid input", func(t *testing.T) {
		t.Parallel()

		codec, spec, _ := newPagination(t)
		page, _, err := paginate(
			[]string{"a", "b", "c", "d"},
			&commonv3alpha.PageRequest{PageSize: 100},
			spec,
			codec,
			func(value string) string { return value },
		)
		require.NoError(t, err)
		require.Len(t, page, 3)

		_, _, err = paginate(
			[]string{"a"},
			&commonv3alpha.PageRequest{PageSize: -1},
			spec,
			codec,
			func(value string) string { return value },
		)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(translateRPCError(t.Context(), err)))

		_, _, err = paginate([]string{"a", "a"}, nil, spec, codec, func(value string) string { return value })
		require.Equal(t, connect.CodeInternal, connect.CodeOf(translateRPCError(t.Context(), err)))
	})

	t.Run("binds tokens to the resource, filters, signature, and expiry", func(t *testing.T) {
		t.Parallel()

		codec, spec, now := newPagination(t)
		_, firstPage, err := paginate([]string{"a", "b", "c"}, nil, spec, codec, func(value string) string { return value })
		require.NoError(t, err)
		token := firstPage.GetNextPageToken()

		mismatched := spec
		mismatched.resource = "silences"
		_, _, err = paginate([]string{"a", "b", "c"}, &commonv3alpha.PageRequest{PageToken: token}, mismatched, codec, func(value string) string { return value })
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(translateRPCError(t.Context(), err)))

		otherDigest, digestErr := protoFilterDigest(&commonv3alpha.StateFilter{})
		require.NoError(t, digestErr)
		mismatched = spec
		mismatched.filterDigest = otherDigest
		_, _, err = paginate([]string{"a", "b", "c"}, &commonv3alpha.PageRequest{PageToken: token}, mismatched, codec, func(value string) string { return value })
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(translateRPCError(t.Context(), err)))

		tampered := []byte(token)
		tampered[0] ^= 1
		_, _, err = paginate([]string{"a", "b", "c"}, &commonv3alpha.PageRequest{PageToken: string(tampered)}, spec, codec, func(value string) string { return value })
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(translateRPCError(t.Context(), err)))

		*now = now.Add(time.Minute)
		_, _, err = paginate([]string{"a", "b", "c"}, &commonv3alpha.PageRequest{PageToken: token}, spec, codec, func(value string) string { return value })
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(translateRPCError(t.Context(), err)))
	})

	t.Run("creates deterministic filter digests", func(t *testing.T) {
		t.Parallel()

		left, err := protoFilterDigest(&commonv3alpha.StateFilter{States: []commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE}})
		require.NoError(t, err)
		right, err := protoFilterDigest(proto.Clone(&commonv3alpha.StateFilter{States: []commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE}}))
		require.NoError(t, err)
		require.Equal(t, left, right)
	})
}

func TestSharedConnectErrors(t *testing.T) {
	t.Parallel()

	t.Run("translates stable service codes", func(t *testing.T) {
		t.Parallel()

		codes := []connect.Code{
			connect.CodeInvalidArgument,
			connect.CodeNotFound,
			connect.CodeResourceExhausted,
			connect.CodeUnavailable,
			connect.CodeFailedPrecondition,
			connect.CodeAborted,
			connect.CodeInternal,
		}
		for _, code := range codes {
			t.Run(code.String(), func(t *testing.T) {
				t.Parallel()
				require.Equal(t, code, connect.CodeOf(translateRPCError(t.Context(), newRPCError(code, "failure", nil))))
			})
		}
	})

	t.Run("preserves cancellation, deadlines, and existing Connect errors", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, connect.CodeCanceled, connect.CodeOf(translateRPCError(t.Context(), context.Canceled)))
		require.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(translateRPCError(t.Context(), context.DeadlineExceeded)))
		existing := connect.NewError(connect.CodePermissionDenied, errors.New("denied"))
		require.Same(t, existing, translateRPCError(t.Context(), existing))
		require.Equal(t, connect.CodeInternal, connect.CodeOf(translateRPCError(t.Context(), errors.New("unexpected"))))
	})

	t.Run("returns structured validation and partial-result details", func(t *testing.T) {
		t.Parallel()

		translated := translateRPCError(t.Context(), invalidArgumentError(
			"invalid request",
			&commonv3alpha.FieldViolation{Field: "filter", Description: "invalid"},
		))
		var connectErr *connect.Error
		require.ErrorAs(t, translated, &connectErr)
		require.Len(t, connectErr.Details(), 1)
		value, err := connectErr.Details()[0].Value()
		require.NoError(t, err)
		require.True(t, proto.Equal(value, &commonv3alpha.ValidationErrorDetail{Violations: []*commonv3alpha.FieldViolation{{Field: "filter", Description: "invalid"}}}))

		translated = translateRPCError(t.Context(), partialResultError(
			connect.CodeResourceExhausted,
			"partly accepted",
			&commonv3alpha.PartialResultDetail{Accepted: 1, Failed: 1, CountsExact: true},
		))
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(translated))
	})

	t.Run("gates named capabilities with structured details", func(t *testing.T) {
		t.Parallel()

		description := "event recording is required"
		err := requireFeature(featurecontrol.NoopFlags{}, featurecontrol.FeatureEventRecorder, description)
		translated := translateRPCError(t.Context(), err)
		require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(translated))

		var connectErr *connect.Error
		require.ErrorAs(t, translated, &connectErr)
		require.Len(t, connectErr.Details(), 1)
		value, err := connectErr.Details()[0].Value()
		require.NoError(t, err)
		require.True(t, proto.Equal(value, &commonv3alpha.RequiredFeatureDetail{Feature: featurecontrol.FeatureEventRecorder, Description: description}))

		flagger, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureEventRecorder)
		require.NoError(t, err)
		require.NoError(t, requireFeature(flagger, featurecontrol.FeatureEventRecorder, description))
	})
}

func TestSharedConnectErrorsOverHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		features string
		err      func(*API) error
		code     connect.Code
		detail   proto.Message
	}{
		{
			name: "validation",
			err: func(*API) error {
				return invalidArgumentError("invalid request", &commonv3alpha.FieldViolation{Field: "filter", Description: "invalid"})
			},
			code:   connect.CodeInvalidArgument,
			detail: &commonv3alpha.ValidationErrorDetail{Violations: []*commonv3alpha.FieldViolation{{Field: "filter", Description: "invalid"}}},
		},
		{
			name: "partial result",
			err: func(*API) error {
				return partialResultError(connect.CodeResourceExhausted, "partly accepted", &commonv3alpha.PartialResultDetail{Accepted: 1, Failed: 1, CountsExact: true})
			},
			code:   connect.CodeResourceExhausted,
			detail: &commonv3alpha.PartialResultDetail{Accepted: 1, Failed: 1, CountsExact: true},
		},
		{
			name: "disabled capability",
			err: func(api *API) error {
				return requireFeature(api.flagger, featurecontrol.FeatureEventRecorder, "event recording is required")
			},
			code:   connect.CodeFailedPrecondition,
			detail: &commonv3alpha.RequiredFeatureDetail{Feature: featurecontrol.FeatureEventRecorder, Description: "event recording is required"},
		},
		{
			name:     "enabled capability",
			features: featurecontrol.FeatureEventRecorder,
			err: func(api *API) error {
				return requireFeature(api.flagger, featurecontrol.FeatureEventRecorder, "event recording is required")
			},
		},
	}
	transports := []struct {
		name string
		opts []connect.ClientOption
	}{
		{name: "Connect"},
		{name: "gRPC-Web", opts: []connect.ClientOption{connect.WithGRPCWeb()}},
		{name: "gRPC", opts: []connect.ClientOption{connect.WithGRPC()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for _, transport := range transports {
				t.Run(transport.name, func(t *testing.T) {
					t.Parallel()

					flagger, err := featurecontrol.NewFlags(promslog.NewNopLogger(), test.features)
					require.NoError(t, err)
					api := NewAPI(Options{Flagger: flagger})
					api.Update(&config.Config{}, nil)
					interceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
						return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
							if err := test.err(api); err != nil {
								return nil, err
							}
							return next(ctx, request)
						}
					})
					srv := newTestServer(t, api.Handler(connect.WithInterceptors(interceptor)), true)
					client := statusv3alphaconnect.NewStatusServiceClient(newH2CClient(t, 5*time.Second), srv.URL, transport.opts...)
					response, err := client.GetStatus(t.Context(), connect.NewRequest(&statusv3alpha.GetStatusRequest{}))
					if test.code == 0 {
						require.NoError(t, err)
						require.NotNil(t, response.Msg.GetStatus())
						return
					}
					require.Equal(t, test.code, connect.CodeOf(err))
					var connectErr *connect.Error
					require.ErrorAs(t, err, &connectErr)
					require.Len(t, connectErr.Details(), 1)
					value, err := connectErr.Details()[0].Value()
					require.NoError(t, err)
					require.True(t, proto.Equal(test.detail, value))
				})
			}
		})
	}
}
