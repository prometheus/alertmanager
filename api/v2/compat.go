// Copyright 2021 Prometheus Team
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

package v2

import (
	"context"
	"fmt"
	"time"

	"github.com/go-openapi/strfmt"
	prometheus_model "github.com/prometheus/common/model"
	"google.golang.org/protobuf/types/known/timestamppb"

	open_api_models "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/silence"
	"github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

// GettableSilenceFromProto converts *silencepb.Silence to open_api_models.GettableSilence.
func GettableSilenceFromProto(s *silencepb.Silence) (open_api_models.GettableSilence, error) {
	start := strfmt.DateTime(s.StartsAt.AsTime())
	end := strfmt.DateTime(s.EndsAt.AsTime())
	updated := strfmt.DateTime(s.UpdatedAt.AsTime())
	state := string(silence.CurrentState(s.StartsAt.AsTime(), s.EndsAt.AsTime()))
	sil := open_api_models.GettableSilence{
		Silence: open_api_models.Silence{
			StartsAt:    &start,
			EndsAt:      &end,
			Comment:     &s.Comment,
			CreatedBy:   &s.CreatedBy,
			Annotations: s.Annotations,
		},
		ID:        &s.Id,
		UpdatedAt: &updated,
		Status: &open_api_models.SilenceStatus{
			State: &state,
		},
	}

	// For backward compatibility, only return silences with a single matcher set
	if len(s.MatcherSets) > 1 {
		return sil, fmt.Errorf("silence '%v' has multiple matcher sets which is not supported by this API version", s.Id)
	}

	if len(s.MatcherSets) == 0 {
		return sil, nil
	}

	for _, m := range s.MatcherSets[0].Matchers {
		matcher := &open_api_models.Matcher{
			Name:  &m.Name,
			Value: &m.Pattern,
		}
		f := false
		t := true
		switch m.Type {
		case silencepb.Matcher_EQUAL:
			matcher.IsEqual = &t
			matcher.IsRegex = &f
		case silencepb.Matcher_NOT_EQUAL:
			matcher.IsEqual = &f
			matcher.IsRegex = &f
		case silencepb.Matcher_REGEXP:
			matcher.IsEqual = &t
			matcher.IsRegex = &t
		case silencepb.Matcher_NOT_REGEXP:
			matcher.IsEqual = &f
			matcher.IsRegex = &t
		default:
			return sil, fmt.Errorf(
				"unknown matcher type for matcher '%v' in silence '%v'",
				m.Name,
				s.Id,
			)
		}
		sil.Matchers = append(sil.Matchers, matcher)
	}

	return sil, nil
}

// PostableSilenceToProto converts *open_api_models.PostableSilenc to *silencepb.Silence.
func PostableSilenceToProto(s *open_api_models.PostableSilence) (*silencepb.Silence, error) {
	sil := &silencepb.Silence{
		Id:          s.ID,
		StartsAt:    timestamppb.New(time.Time(*s.StartsAt)),
		EndsAt:      timestamppb.New(time.Time(*s.EndsAt)),
		Comment:     *s.Comment,
		CreatedBy:   *s.CreatedBy,
		Annotations: map[string]string{},
	}

	matcherSet := &silencepb.MatcherSet{}
	for _, m := range s.Matchers {
		matcher := &silencepb.Matcher{
			Name:    *m.Name,
			Pattern: *m.Value,
		}
		isEqual := true
		if m.IsEqual != nil {
			isEqual = *m.IsEqual
		}
		isRegex := false
		if m.IsRegex != nil {
			isRegex = *m.IsRegex
		}

		switch {
		case isEqual && !isRegex:
			matcher.Type = silencepb.Matcher_EQUAL
		case !isEqual && !isRegex:
			matcher.Type = silencepb.Matcher_NOT_EQUAL
		case isEqual && isRegex:
			matcher.Type = silencepb.Matcher_REGEXP
		case !isEqual && isRegex:
			matcher.Type = silencepb.Matcher_NOT_REGEXP
		}
		matcherSet.Matchers = append(matcherSet.Matchers, matcher)
	}
	sil.MatcherSets = append(sil.MatcherSets, matcherSet)

	if s.Annotations != nil {
		sil.Annotations = s.Annotations
	}

	return sil, nil
}

// AlertToOpenAPIAlert converts internal alerts, alert types, and receivers to *open_api_models.GettableAlert.
func AlertToOpenAPIAlert(alert *types.Alert, status types.AlertStatus, receivers, mutedBy []string) *open_api_models.GettableAlert {
	startsAt := strfmt.DateTime(alert.StartsAt)
	updatedAt := strfmt.DateTime(alert.UpdatedAt)
	endsAt := strfmt.DateTime(alert.EndsAt)

	apiReceivers := make([]*open_api_models.Receiver, 0, len(receivers))
	for i := range receivers {
		apiReceivers = append(apiReceivers, &open_api_models.Receiver{Name: &receivers[i]})
	}

	fp := alert.Fingerprint().String()

	state := string(status.State)
	if len(mutedBy) > 0 {
		// If the alert is muted, change the state to suppressed.
		state = open_api_models.AlertStatusStateSuppressed
	}

	aa := &open_api_models.GettableAlert{
		Alert: open_api_models.Alert{
			GeneratorURL: strfmt.URI(alert.GeneratorURL),
			Labels:       ModelLabelSetToAPILabelSet(alert.Labels),
		},
		Annotations: ModelLabelSetToAPILabelSet(alert.Annotations),
		StartsAt:    &startsAt,
		UpdatedAt:   &updatedAt,
		EndsAt:      &endsAt,
		Fingerprint: &fp,
		Receivers:   apiReceivers,
		Status: &open_api_models.AlertStatus{
			State:       &state,
			SilencedBy:  status.SilencedBy,
			InhibitedBy: status.InhibitedBy,
			MutedBy:     mutedBy,
		},
	}

	if aa.Status.SilencedBy == nil {
		aa.Status.SilencedBy = []string{}
	}

	if aa.Status.InhibitedBy == nil {
		aa.Status.InhibitedBy = []string{}
	}

	if aa.Status.MutedBy == nil {
		aa.Status.MutedBy = []string{}
	}

	return aa
}

// OpenAPIAlertsToAlerts converts open_api_models.PostableAlerts to []*types.Alert.
func OpenAPIAlertsToAlerts(ctx context.Context, apiAlerts open_api_models.PostableAlerts) []*types.Alert {
	_, span := tracer.Start(ctx, "OpenAPIAlertsToAlerts")
	defer span.End()

	alerts := []*types.Alert{}
	for _, apiAlert := range apiAlerts {
		alert := types.Alert{
			Alert: prometheus_model.Alert{
				Labels:       APILabelSetToModelLabelSet(apiAlert.Labels),
				Annotations:  APILabelSetToModelLabelSet(apiAlert.Annotations),
				StartsAt:     time.Time(apiAlert.StartsAt),
				EndsAt:       time.Time(apiAlert.EndsAt),
				GeneratorURL: string(apiAlert.GeneratorURL),
			},
		}
		alerts = append(alerts, &alert)
	}

	return alerts
}

// ModelLabelSetToAPILabelSet converts prometheus_model.LabelSet to open_api_models.LabelSet.
func ModelLabelSetToAPILabelSet(modelLabelSet prometheus_model.LabelSet) open_api_models.LabelSet {
	apiLabelSet := open_api_models.LabelSet{}
	for key, value := range modelLabelSet {
		apiLabelSet[string(key)] = string(value)
	}

	return apiLabelSet
}

// APILabelSetToModelLabelSet converts open_api_models.LabelSet to prometheus_model.LabelSet.
func APILabelSetToModelLabelSet(apiLabelSet open_api_models.LabelSet) prometheus_model.LabelSet {
	modelLabelSet := prometheus_model.LabelSet{}
	for key, value := range apiLabelSet {
		modelLabelSet[prometheus_model.LabelName(key)] = prometheus_model.LabelValue(value)
	}

	return modelLabelSet
}
