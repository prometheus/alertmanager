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
	"time"

	"connectrpc.com/connect"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"google.golang.org/protobuf/proto"

	commonv3alpha "github.com/prometheus/alertmanager/api/common/v3alpha"
	"github.com/prometheus/alertmanager/featurecontrol"
	"github.com/prometheus/alertmanager/pkg/labels"
)

var _ = Describe("shared matchers", func() {
	It("converts every matcher type in both directions", func() {
		matcherTypes := []commonv3alpha.MatcherType{
			commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL,
			commonv3alpha.MatcherType_MATCHER_TYPE_NOT_EQUAL,
			commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP,
			commonv3alpha.MatcherType_MATCHER_TYPE_NOT_REGEXP,
		}
		for _, matcherType := range matcherTypes {
			input := &commonv3alpha.Matcher{Name: "service", Value: "api.*", Type: matcherType}
			internal, err := matcherFromProto(input)
			Expect(err).NotTo(HaveOccurred())
			output, err := matcherToProto(internal)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(input))
		}
	})

	It("applies OR across sets and AND within a set", func() {
		sets, err := matcherSetsFromProto([]*commonv3alpha.MatcherSet{
			{Matchers: []*commonv3alpha.Matcher{
				{Name: "service", Value: "api", Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL},
				{Name: "region", Value: "us-.+", Type: commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP},
			}},
			{Matchers: []*commonv3alpha.Matcher{
				{Name: "severity", Value: "critical", Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL},
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(matchesMatcherSets(sets, model.LabelSet{"service": "api", "region": "us-east"})).To(BeTrue())
		Expect(matchesMatcherSets(sets, model.LabelSet{"severity": "critical"})).To(BeTrue())
		Expect(matchesMatcherSets(sets, model.LabelSet{"service": "api", "region": "eu-west"})).To(BeFalse())
		Expect(matchesMatcherSets(nil, nil)).To(BeTrue())

		output, err := matcherSetsToProto(sets)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(HaveLen(2))
		Expect(output[0].GetMatchers()).To(HaveLen(2))
	})

	It("rejects invalid matchers", func() {
		_, err := matcherFromProto(nil)
		Expect(err).To(MatchError("matcher is required"))
		_, err = matcherFromProto(&commonv3alpha.Matcher{Name: "", Type: commonv3alpha.MatcherType_MATCHER_TYPE_EQUAL})
		Expect(err).To(MatchError(ContainSubstring("invalid label name")))
		_, err = matcherFromProto(&commonv3alpha.Matcher{Name: "service"})
		Expect(err).To(MatchError(ContainSubstring("unsupported matcher type")))
		_, err = matcherFromProto(&commonv3alpha.Matcher{Name: "service", Value: "[", Type: commonv3alpha.MatcherType_MATCHER_TYPE_REGEXP})
		Expect(err).To(MatchError(ContainSubstring("invalid matcher")))
		_, err = matcherSetsFromProto([]*commonv3alpha.MatcherSet{nil})
		Expect(err).To(MatchError("matcher set 0 is required"))
		_, err = matcherToProto(&labels.Matcher{Type: labels.MatchType(99)})
		Expect(err).To(MatchError(ContainSubstring("unsupported internal matcher type")))
	})
})

var _ = Describe("shared filters", func() {
	It("matches every value when empty and selected values when populated", func() {
		valid := func(state commonv3alpha.ResourceState) bool {
			return state == commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE || state == commonv3alpha.ResourceState_RESOURCE_STATE_EXPIRED
		}
		empty, err := newValueFilter([]commonv3alpha.ResourceState(nil), valid)
		Expect(err).NotTo(HaveOccurred())
		Expect(empty.matches(commonv3alpha.ResourceState_RESOURCE_STATE_PENDING)).To(BeTrue())

		selected, err := newValueFilter([]commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE}, valid)
		Expect(err).NotTo(HaveOccurred())
		Expect(selected.matches(commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE)).To(BeTrue())
		Expect(selected.matches(commonv3alpha.ResourceState_RESOURCE_STATE_EXPIRED)).To(BeFalse())

		_, err = newValueFilter([]commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_PENDING}, valid)
		Expect(err).To(MatchError(ContainSubstring("invalid filter value at index 0")))
	})
})

var _ = Describe("shared pagination", func() {
	var (
		codec  *pageTokenCodec
		digest string
		now    time.Time
		spec   paginationSpec
	)

	BeforeEach(func() {
		now = time.Unix(1_800_000_000, 0)
		codec = newPageTokenCodec([]byte("test-key"), time.Minute)
		codec.now = func() time.Time { return now }
		var err error
		digest, err = protoFilterDigest(&commonv3alpha.StateFilter{States: []commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE}})
		Expect(err).NotTo(HaveOccurred())
		spec = paginationSpec{resource: "alerts", defaultSize: 2, maximumSize: 3, filterDigest: digest}
	})

	It("creates opaque tokens and continues after the last cursor", func() {
		first, page, err := paginate([]string{"a", "b", "c"}, nil, spec, codec, func(value string) string { return value })
		Expect(err).NotTo(HaveOccurred())
		Expect(first).To(Equal([]string{"a", "b"}))
		Expect(page.GetNextPageToken()).NotTo(BeEmpty())

		second, page, err := paginate(
			[]string{"a", "b", "bb", "c"},
			&commonv3alpha.PageRequest{PageToken: page.GetNextPageToken()},
			spec,
			codec,
			func(value string) string { return value },
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal([]string{"bb", "c"}))
		Expect(page.GetNextPageToken()).To(BeEmpty())
	})

	It("clamps page size and rejects invalid input", func() {
		page, _, err := paginate(
			[]string{"a", "b", "c", "d"},
			&commonv3alpha.PageRequest{PageSize: 100},
			spec,
			codec,
			func(value string) string { return value },
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(page).To(HaveLen(3))

		_, _, err = paginate(
			[]string{"a"},
			&commonv3alpha.PageRequest{PageSize: -1},
			spec,
			codec,
			func(value string) string { return value },
		)
		Expect(connect.CodeOf(translateRPCError(context.Background(), err))).To(Equal(connect.CodeInvalidArgument))

		_, _, err = paginate([]string{"a", "a"}, nil, spec, codec, func(value string) string { return value })
		Expect(connect.CodeOf(translateRPCError(context.Background(), err))).To(Equal(connect.CodeInternal))
	})

	It("binds tokens to the resource, filters, signature, and expiry", func() {
		_, firstPage, err := paginate([]string{"a", "b", "c"}, nil, spec, codec, func(value string) string { return value })
		Expect(err).NotTo(HaveOccurred())
		token := firstPage.GetNextPageToken()

		mismatched := spec
		mismatched.resource = "silences"
		_, _, err = paginate([]string{"a", "b", "c"}, &commonv3alpha.PageRequest{PageToken: token}, mismatched, codec, func(value string) string { return value })
		Expect(connect.CodeOf(translateRPCError(context.Background(), err))).To(Equal(connect.CodeInvalidArgument))

		otherDigest, digestErr := protoFilterDigest(&commonv3alpha.StateFilter{})
		Expect(digestErr).NotTo(HaveOccurred())
		mismatched = spec
		mismatched.filterDigest = otherDigest
		_, _, err = paginate([]string{"a", "b", "c"}, &commonv3alpha.PageRequest{PageToken: token}, mismatched, codec, func(value string) string { return value })
		Expect(connect.CodeOf(translateRPCError(context.Background(), err))).To(Equal(connect.CodeInvalidArgument))

		tampered := token[:len(token)-1] + "A"
		_, _, err = paginate([]string{"a", "b", "c"}, &commonv3alpha.PageRequest{PageToken: tampered}, spec, codec, func(value string) string { return value })
		Expect(connect.CodeOf(translateRPCError(context.Background(), err))).To(Equal(connect.CodeInvalidArgument))

		now = now.Add(time.Minute)
		_, _, err = paginate([]string{"a", "b", "c"}, &commonv3alpha.PageRequest{PageToken: token}, spec, codec, func(value string) string { return value })
		Expect(connect.CodeOf(translateRPCError(context.Background(), err))).To(Equal(connect.CodeInvalidArgument))
	})

	It("creates deterministic filter digests", func() {
		left, err := protoFilterDigest(&commonv3alpha.StateFilter{States: []commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE}})
		Expect(err).NotTo(HaveOccurred())
		right, err := protoFilterDigest(proto.Clone(&commonv3alpha.StateFilter{States: []commonv3alpha.ResourceState{commonv3alpha.ResourceState_RESOURCE_STATE_ACTIVE}}))
		Expect(err).NotTo(HaveOccurred())
		Expect(left).To(Equal(right))
	})
})

var _ = Describe("shared Connect errors", func() {
	It("translates stable service codes", func() {
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
			Expect(connect.CodeOf(translateRPCError(context.Background(), newRPCError(code, "failure", nil)))).To(Equal(code))
		}
	})

	It("preserves cancellation, deadlines, and existing Connect errors", func() {
		Expect(connect.CodeOf(translateRPCError(context.Background(), context.Canceled))).To(Equal(connect.CodeCanceled))
		Expect(connect.CodeOf(translateRPCError(context.Background(), context.DeadlineExceeded))).To(Equal(connect.CodeDeadlineExceeded))
		existing := connect.NewError(connect.CodePermissionDenied, errors.New("denied"))
		Expect(translateRPCError(context.Background(), existing)).To(BeIdenticalTo(existing))
		Expect(connect.CodeOf(translateRPCError(context.Background(), errors.New("unexpected")))).To(Equal(connect.CodeInternal))
	})

	It("returns structured validation and partial-result details", func() {
		translated := translateRPCError(context.Background(), invalidArgumentError(
			"invalid request",
			&commonv3alpha.FieldViolation{Field: "filter", Description: "invalid"},
		))
		var connectErr *connect.Error
		Expect(errors.As(translated, &connectErr)).To(BeTrue())
		Expect(connectErr.Details()).To(HaveLen(1))
		value, err := connectErr.Details()[0].Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(proto.Equal(value, &commonv3alpha.ValidationErrorDetail{Violations: []*commonv3alpha.FieldViolation{{Field: "filter", Description: "invalid"}}})).To(BeTrue())

		translated = translateRPCError(context.Background(), partialResultError(
			connect.CodeResourceExhausted,
			"partly accepted",
			&commonv3alpha.PartialResultDetail{Accepted: 1, Failed: 1, CountsExact: true},
		))
		Expect(connect.CodeOf(translated)).To(Equal(connect.CodeResourceExhausted))
	})

	It("gates named capabilities with structured details", func() {
		description := "event recording is required"
		err := requireFeature(featurecontrol.NoopFlags{}, featurecontrol.FeatureEventRecorder, description)
		translated := translateRPCError(context.Background(), err)
		Expect(connect.CodeOf(translated)).To(Equal(connect.CodeFailedPrecondition))

		var connectErr *connect.Error
		Expect(errors.As(translated, &connectErr)).To(BeTrue())
		value, err := connectErr.Details()[0].Value()
		Expect(err).NotTo(HaveOccurred())
		Expect(proto.Equal(value, &commonv3alpha.RequiredFeatureDetail{Feature: featurecontrol.FeatureEventRecorder, Description: description})).To(BeTrue())

		flagger, err := featurecontrol.NewFlags(promslog.NewNopLogger(), featurecontrol.FeatureEventRecorder)
		Expect(err).NotTo(HaveOccurred())
		Expect(requireFeature(flagger, featurecontrol.FeatureEventRecorder, description)).To(Succeed())
	})
})
