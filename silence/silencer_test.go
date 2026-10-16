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
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/labelset"
	"github.com/prometheus/alertmanager/marker"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

// checkMutes checks that the marker recorded the expected silenced state
// for the given target after a Mutes() call.
func checkMutes(t *testing.T, m marker.AlertMarker, target model.LabelSet, wantSilenced bool, msgAndArgs ...any) {
	t.Helper()
	fp := target.Fingerprint()
	status := m.Status(fp)
	if wantSilenced {
		require.Equal(t, alert.AlertStateSuppressed, status.State, msgAndArgs...)
		require.NotEmpty(t, status.SilencedBy, msgAndArgs...)
	} else {
		require.Empty(t, status.SilencedBy, msgAndArgs...)
	}
}

func TestSilencer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ss, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
		require.NoError(t, err)

		now := ss.nowUTC()

		s := NewSilencer(ss, promslog.NewNopLogger(), eventrecorder.NopRecorder())

		m := marker.NewAlertMarker()
		ctx := marker.WithContext(t.Context(), m)

		require.False(t, s.Mutes(ctx, labelset.FromModel(model.LabelSet{"foo": "bar"})), "expected alert not silenced without any silences")
		checkMutes(t, m, model.LabelSet{"foo": "bar"}, false, "expected marker not silenced without any silences")

		sil1 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "foo", Pattern: "baz"}},
			}},
			StartsAt: timestamppb.New(now.Add(-time.Hour)),
			EndsAt:   timestamppb.New(now.Add(5 * time.Minute)),
		}
		require.NoError(t, ss.Set(t.Context(), sil1))

		m = marker.NewAlertMarker()
		ctx = marker.WithContext(t.Context(), m)
		require.False(t, s.Mutes(ctx, labelset.FromModel(model.LabelSet{"foo": "bar"})), "expected alert not silenced by non-matching silence")
		checkMutes(t, m, model.LabelSet{"foo": "bar"}, false, "expected marker not silenced by non-matching silence")

		sil2 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "foo", Pattern: "bar"}},
			}},
			StartsAt: timestamppb.New(now.Add(-time.Hour)),
			EndsAt:   timestamppb.New(now.Add(5 * time.Minute)),
		}
		require.NoError(t, ss.Set(t.Context(), sil2))
		require.NotEmpty(t, sil2.Id)

		m = marker.NewAlertMarker()
		ctx = marker.WithContext(t.Context(), m)
		require.True(t, s.Mutes(ctx, labelset.FromModel(model.LabelSet{"foo": "bar"})), "expected alert silenced by matching silence")
		checkMutes(t, m, model.LabelSet{"foo": "bar"}, true, "expected marker silenced by matching silence")

		// One hour passes, silence expires.
		time.Sleep(time.Hour)
		now = ss.nowUTC()

		m = marker.NewAlertMarker()
		ctx = marker.WithContext(t.Context(), m)
		require.False(t, s.Mutes(ctx, labelset.FromModel(model.LabelSet{"foo": "bar"})), "expected alert not silenced by expired silence")
		checkMutes(t, m, model.LabelSet{"foo": "bar"}, false, "expected marker not silenced by expired silence")

		// Update silence to start in the future.
		err = ss.Set(t.Context(), &pb.Silence{
			Id: sil2.Id,
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "foo", Pattern: "bar"}},
			}},
			StartsAt: timestamppb.New(now.Add(time.Hour)),
			EndsAt:   timestamppb.New(now.Add(3 * time.Hour)),
		})
		require.NoError(t, err)

		m = marker.NewAlertMarker()
		ctx = marker.WithContext(t.Context(), m)
		require.False(t, s.Mutes(ctx, labelset.FromModel(model.LabelSet{"foo": "bar"})), "expected alert not silenced by future silence")
		checkMutes(t, m, model.LabelSet{"foo": "bar"}, false, "expected marker not silenced by future silence")

		// Two hours pass, silence becomes active.
		time.Sleep(2 * time.Hour)
		now = ss.nowUTC()

		// Exposes issue #2426.
		m = marker.NewAlertMarker()
		ctx = marker.WithContext(t.Context(), m)
		require.True(t, s.Mutes(ctx, labelset.FromModel(model.LabelSet{"foo": "bar"})), "expected alert silenced by activated silence")
		checkMutes(t, m, model.LabelSet{"foo": "bar"}, true, "expected marker silenced by activated silence")

		err = ss.Set(t.Context(), &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "foo", Pattern: "b..", Type: pb.Matcher_REGEXP}},
			}},
			StartsAt: timestamppb.New(now.Add(time.Hour)),
			EndsAt:   timestamppb.New(now.Add(3 * time.Hour)),
		})
		require.NoError(t, err)

		// Note that issue #2426 doesn't apply anymore because we added a new silence.
		m = marker.NewAlertMarker()
		ctx = marker.WithContext(t.Context(), m)
		require.True(t, s.Mutes(ctx, labelset.FromModel(model.LabelSet{"foo": "bar"})), "expected alert still silenced by activated silence")
		checkMutes(t, m, model.LabelSet{"foo": "bar"}, true, "expected marker still silenced by activated silence")

		// Two hours pass, first silence expires, overlapping second silence becomes active.
		time.Sleep(2 * time.Hour)

		// Another variant of issue #2426 (overlapping silences).
		m = marker.NewAlertMarker()
		ctx = marker.WithContext(t.Context(), m)
		require.True(t, s.Mutes(ctx, labelset.FromModel(model.LabelSet{"foo": "bar"})), "expected alert silenced by activated second silence")
		checkMutes(t, m, model.LabelSet{"foo": "bar"}, true, "expected marker silenced by activated second silence")
	})
}

func TestSilencerPostDeleteEvictsCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ss, err := New(Options{Metrics: prometheus.NewRegistry(), Retention: time.Hour})
		require.NoError(t, err)

		now := ss.nowUTC()

		s := NewSilencer(ss, promslog.NewNopLogger(), eventrecorder.NopRecorder())

		lset := model.LabelSet{"foo": "bar"}
		fp := lset.Fingerprint()

		// Create a matching silence.
		sil := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{Name: "foo", Pattern: "bar"}},
			}},
			StartsAt: timestamppb.New(now.Add(-time.Hour)),
			EndsAt:   timestamppb.New(now.Add(5 * time.Minute)),
		}
		require.NoError(t, ss.Set(t.Context(), sil))

		// Mutes populates the cache.
		m := marker.NewAlertMarker()
		ctx := marker.WithContext(t.Context(), m)
		require.True(t, s.Mutes(ctx, labelset.FromModel(lset)))
		checkMutes(t, m, lset, true, "expected marker silenced after initial Mutes")
		entry := s.cache.get(fp)
		require.Positive(t, entry.count(), "cache should have entries after Mutes()")

		// PostGC evicts the cache entry for this fingerprint.
		s.PostGC(model.Fingerprints{fp})
		entry = s.cache.get(fp)
		require.Equal(t, 0, entry.count(), "cache should be empty after PostGC()")
		require.Equal(t, 0, entry.version, "version should be zero for evicted entry")

		// Mutes re-evaluates from scratch (cache miss) and still finds the silence.
		m = marker.NewAlertMarker()
		ctx = marker.WithContext(t.Context(), m)
		require.True(t, s.Mutes(ctx, labelset.FromModel(lset)), "expected alert still silenced after cache eviction")
		checkMutes(t, m, lset, true, "expected marker silenced after cache eviction")
		entry = s.cache.get(fp)
		require.Positive(t, entry.count(), "cache should be repopulated after Mutes()")

		// Expire the silence, advance time so it's truly expired.
		time.Sleep(time.Hour)

		// PostGC for a different fingerprint should not affect this entry.
		otherLset := model.LabelSet{"other": "alert"}
		s.PostGC(model.Fingerprints{otherLset.Fingerprint()})
		entry = s.cache.get(fp)
		require.Positive(t, entry.count(), "unrelated PostGC should not evict other entries")
	})
}
