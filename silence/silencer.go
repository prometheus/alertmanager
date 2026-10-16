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

package silence

import (
	"context"
	"log/slog"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/marker"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

// Silencer holds Silences and implements the Muter interface.
type Silencer struct {
	silences *Silences
	cache    *cache
	logger   *slog.Logger
	recorder eventrecorder.Recorder
}

// NewSilencer returns a new Silencer.
func NewSilencer(silences *Silences, logger *slog.Logger, recorder eventrecorder.Recorder) *Silencer {
	return &Silencer{
		silences: silences,
		cache:    &cache{entries: map[model.Fingerprint]*cacheEntry{}},
		logger:   logger,
		recorder: recorder,
	}
}

// Mutes implements the Muter interface.
func (s *Silencer) Mutes(ctx context.Context, lset labelset.LabelSet) bool {
	fp := lset.Fingerprint()
	ctx, span := tracer.Start(ctx, "silence.Silencer.Mutes")
	defer span.End()
	recording := span.IsRecording()
	if recording {
		span.SetAttributes(attribute.String("alerting.alert.fingerprint", fp.String()))
	}

	// Track the silences to set on marker
	var markedSilences []string
	defer func() {
		// Get the marker from context and set the silences on it if any.
		m, ok := marker.FromContext(ctx)
		if ok {
			m.SetSilenced(fp, markedSilences)
		}
	}()

	// Get the cached entry for this fingerprint.
	cachedEntry := s.cache.get(fp)

	var (
		oldSils    []*pb.Silence
		newSils    []*pb.Silence
		newVersion = cachedEntry.version
	)
	cacheIsUpToDate := cachedEntry.version == s.silences.Version()

	if cacheIsUpToDate && cachedEntry.count() == 0 {
		// Very fast path: no new silences have been added and this lset was not
		// silenced last time we checked.
		if recording {
			span.AddEvent("No new silences to match since last check",
				trace.WithAttributes(
					attribute.Int("alerting.silences.cache.count", cachedEntry.count()),
				),
			)
		}
		return false
	}
	// Either there are new silences and we need to check if those match lset or there were
	// silences last time we queried so we need to see if those are still active/have become
	// active. It's possible for there to be both old and new silences.

	now := s.silences.nowUTC()
	if cachedEntry.count() > 0 {
		// there were old silences for this lset, we need to find them to check if they
		// are still active/pending, or have ended.
		var err error
		oldSils, _, err = s.silences.QueryAt(
			ctx, now,
			QIDs(cachedEntry.silenceIDs...),
			QState(SilenceStateActive, SilenceStatePending),
		)
		if err != nil {
			if recording {
				span.SetStatus(codes.Error, err.Error())
				span.RecordError(err)
			}
			s.logger.Error(
				"Querying old silences failed, alerts might not get silenced correctly",
				"err", err,
			)
		}
	}

	if !cacheIsUpToDate {
		// New silences have been added since the last time the marker was updated. Do a full
		// query for any silences newer than the markerVersion that match the lset.
		// On this branch we WILL update newVersion since we can be sure we've seen any silences
		// newer than markerVersion.
		var err error
		newSils, newVersion, err = s.silences.QueryAt(
			ctx, now,
			QSince(cachedEntry.version),
			QState(SilenceStateActive, SilenceStatePending),
			QMatches(lset.LabelSet),
		)
		if err != nil {
			if recording {
				span.SetStatus(codes.Error, err.Error())
				span.RecordError(err)
			}
			s.logger.Error(
				"Querying silences failed, alerts might not get silenced correctly",
				"err", err,
			)
		}
	}
	// Note: if cacheIsUpToDate, newVersion is left at cachedEntry.version because the Query call
	// might already return a newer version, which is not the version our old list of
	// applicable silences is based on.

	totalSilences := len(oldSils) + len(newSils)
	if totalSilences == 0 {
		// Easy case, neither active nor pending silences anymore.
		s.cache.set(fp, newCacheEntry(newVersion))
		if recording {
			span.AddEvent("No silences to match", trace.WithAttributes(
				attribute.Int("alerting.silences.count", totalSilences),
			))
		}
		return false
	}

	// It is still possible that nothing has changed, but finding out is not
	// much less effort than just recreating the IDs from the query
	// result. So let's do it in any case. Note that we cannot reuse the
	// current ID slices for concurrency reasons.
	activeIDs := make([]string, 0, totalSilences)
	allIDs := make([]string, 0, totalSilences)
	seen := make(map[string]struct{}, totalSilences)

	// Categorize old and new silences by their current state.
	// oldSils and newSils may overlap if a cached silence was updated
	// (receiving a new version), so we deduplicate by ID.
	for _, sils := range [...][]*pb.Silence{oldSils, newSils} {
		for _, sil := range sils {
			if _, ok := seen[sil.Id]; ok {
				continue
			}
			seen[sil.Id] = struct{}{}
			switch StateFromProtoAt(sil, now) {
			case SilenceStatePending:
				allIDs = append(allIDs, sil.Id)
			case SilenceStateActive:
				activeIDs = append(activeIDs, sil.Id)
				allIDs = append(allIDs, sil.Id)

				s.recorder.RecordEvent(ctx, func() eventrecorder.EventData {
					return eventrecorder.NewSilenceMutedAlertEvent(
						sil, fp, lset.LabelSet,
					)
				})
			default:
				// Do nothing, silence has expired in the meantime.
			}
		}
	}
	s.logger.Debug(
		"determined current silences state",
		"now", now,
		"total", len(allIDs),
		"active", len(activeIDs),
		"pending", len(allIDs)-len(activeIDs),
	)

	s.cache.set(fp, newCacheEntry(newVersion, allIDs...))

	mutes := len(activeIDs) > 0
	if mutes {
		markedSilences = activeIDs
	}
	if recording {
		t := trace.WithAttributes(
			attribute.Int("alerting.silences.active.count", len(activeIDs)),
			attribute.Int("alerting.silences.pending.count", len(allIDs)-len(activeIDs)),
			attribute.Int("alerting.silences.total.count", len(allIDs)),
		)
		if mutes {
			span.AddEvent("Silencer mutes alert", t)
		} else {
			span.AddEvent("Silencer does not mute alert", t)
		}
	}
	return mutes
}

// The following methods implement mem.AlertStoreCallback.
func (s *Silencer) PreStore(_ *alert.Alert, _ bool) error { return nil }
func (s *Silencer) PostStore(_ *alert.Alert, _ bool)      {}
func (s *Silencer) PostDelete(_ *alert.Alert)             {}
func (s *Silencer) PostGC(ff model.Fingerprints) {
	for _, fp := range ff {
		s.cache.delete(fp)
	}
}
