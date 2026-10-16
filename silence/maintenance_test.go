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
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/test"
)

func TestSilenceGCOverTime(t *testing.T) {
	t.Run("GC does not remove active silences", func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		now := s.nowUTC()
		initialState := state{
			"1": &pb.MeshSilence{Silence: &pb.Silence{Id: "1"}, ExpiresAt: timestamppb.New(now)},
			"2": &pb.MeshSilence{Silence: &pb.Silence{Id: "2"}, ExpiresAt: timestamppb.New(now.Add(-time.Second))},
			"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}, ExpiresAt: timestamppb.New(now.Add(time.Second))},
		}
		for _, sil := range initialState {
			s.st[sil.Silence.Id] = sil
			s.indexSilence(sil.Silence)
		}
		want := state{
			"3": &pb.MeshSilence{Silence: &pb.Silence{Id: "3"}, ExpiresAt: timestamppb.New(now.Add(time.Second))},
		}
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 2, n)
		requireStatesEqual(t, want, s.st)
	})

	test.SyncTestRun("GC removes expired silences", t, func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		now := time.Now().UTC()
		sil1 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{
					Type:    pb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
			}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(time.Minute)),
		}
		require.NoError(t, s.Set(t.Context(), sil1))
		require.Len(t, s.st, 1)
		require.Len(t, s.mi, 1)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		time.Sleep(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mi)
	})

	test.SyncTestRun("replacing a silence does not leak cache entries", t, func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		now := time.Now().UTC()
		sil1 := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{
					Type:    pb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
			}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(time.Minute)),
		}
		require.NoError(t, s.Set(t.Context(), sil1))
		require.Len(t, s.st, 1)
		require.Len(t, s.mi, 1)
		// must clone sil1 before replacing it.
		sil2 := cloneSilence(sil1)
		sil2.MatcherSets = []*pb.MatcherSet{{
			Matchers: []*pb.Matcher{{
				Type:    pb.Matcher_EQUAL,
				Name:    "bar",
				Pattern: "baz",
			}},
		}}
		require.NoError(t, s.Set(t.Context(), sil2))
		require.Len(t, s.st, 2)
		require.Len(t, s.mi, 2)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		time.Sleep(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mi)
	})

	// This test checks for a memory leak that occurred in the matcher cache when
	// updating an existing silence.
	test.SyncTestRun("updating a silence does not leak cache entries", t, func(t *testing.T) {
		s, err := New(Options{Metrics: prometheus.NewRegistry()})
		require.NoError(t, err)
		now := s.nowUTC()
		sil1 := &pb.Silence{
			Id: "1",
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{
					Type:    pb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
			}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(time.Minute)),
		}
		s.st["1"] = &pb.MeshSilence{Silence: sil1, ExpiresAt: timestamppb.New(now.Add(time.Minute))}
		s.indexSilence(sil1)
		require.Len(t, s.mi, 1)
		// must clone sil1 before updating it.
		sil2 := cloneSilence(sil1)
		require.NoError(t, s.Set(t.Context(), sil2))
		// The memory leak occurred because updating a silence would add a new
		// entry in the matcher cache even though no new silence was created.
		// This check asserts that this no longer happens.
		s.Query(t.Context(), QMatches(model.LabelSet{"foo": "bar"}))
		require.Len(t, s.st, 1)
		require.Len(t, s.mi, 1)
		// Move time forward and both silence and cache entry should be garbage
		// collected.
		time.Sleep(time.Minute)
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Empty(t, s.st)
		require.Empty(t, s.mi)
	})

	test.SyncTestRun("GC collects silences in multiple rounds", t, func(t *testing.T) {
		s, err := New(Options{
			Metrics:   prometheus.NewRegistry(),
			Retention: time.Hour,
		})
		require.NoError(t, err)
		now := s.nowUTC()

		matcher := &pb.Matcher{
			Type:    pb.Matcher_EQUAL,
			Name:    "job",
			Pattern: "test",
		}

		// Create silences that expire at different times.
		// Directly set them in state to create pre-expired silences.
		// Group 1: expires at now+30min (with retention: now+90min)
		// Group 2: expires at now+45min (with retention: now+105min)
		// Group 3: expires at now+60min (with retention: now+120min)
		// Group 4: active, expires at now+3hours (with retention: now+4hours)

		sils := make([]*pb.Silence, 0, 60)
		for i := range 10 {
			sil := &pb.Silence{
				Id: fmt.Sprintf("group1-%d", i),
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{matcher},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(30 * time.Minute)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}
			sils = append(sils, sil)
		}

		for i := range 10 {
			sil := &pb.Silence{
				Id: fmt.Sprintf("group2-%d", i),
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{matcher},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(45 * time.Minute)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}
			sils = append(sils, sil)
		}

		for i := range 10 {
			sil := &pb.Silence{
				Id: fmt.Sprintf("group3-%d", i),
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{matcher},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(60 * time.Minute)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}
			sils = append(sils, sil)
		}

		for i := range 30 {
			sil := &pb.Silence{
				Id: fmt.Sprintf("active-%d", i),
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{matcher},
				}},
				StartsAt:  timestamppb.New(now.Add(-time.Hour)),
				EndsAt:    timestamppb.New(now.Add(3 * time.Hour)),
				UpdatedAt: timestamppb.New(now.Add(-time.Hour)),
			}
			sils = append(sils, sil)
		}

		// Shuffle silences to ensure GC order is not dependent on insertion order.
		rand.Shuffle(len(sils), func(i, j int) {
			sils[i], sils[j] = sils[j], sils[i]
		})
		for _, sil := range sils {
			ms := s.toMeshSilence(sil)
			s.st[ms.Silence.Id] = ms
			s.indexSilence(ms.Silence)
		}

		require.Len(t, s.st, 60)
		require.Len(t, s.mi, 60)

		// First GC: nothing should be collected yet
		n, err := s.GC()
		require.NoError(t, err)
		require.Equal(t, 0, n)
		require.Len(t, s.st, 60)
		require.Len(t, s.mi, 60)

		// Advance time to 91 minutes - Group 1 should be GC'd
		time.Sleep(91 * time.Minute)
		n, err = s.GC()
		require.NoError(t, err)
		require.Equal(t, 10, n)
		require.Len(t, s.st, 50)
		require.Len(t, s.mi, 50)

		// Advance time to 106 minutes - Group 2 should be GC'd
		time.Sleep(15 * time.Minute)
		n, err = s.GC()
		require.NoError(t, err)
		require.Equal(t, 10, n)
		require.Len(t, s.st, 40)
		require.Len(t, s.mi, 40)

		// Advance time to 121 minutes - Group 3 should be GC'd
		time.Sleep(15 * time.Minute)
		n, err = s.GC()
		require.NoError(t, err)
		require.Equal(t, 10, n)
		require.Len(t, s.st, 30)
		require.Len(t, s.mi, 30)

		// Verify all remaining silences are active
		for id := range s.st {
			require.Contains(t, id, "active-")
		}
	})

	test.SyncTestRun("GC continues and removes erroneous silences", t, func(t *testing.T) {
		reg := prometheus.NewRegistry()
		s, err := New(Options{Metrics: reg})
		require.NoError(t, err)
		now := time.Now().UTC()

		// Create a valid silence
		validSil := &pb.Silence{
			MatcherSets: []*pb.MatcherSet{{
				Matchers: []*pb.Matcher{{
					Type:    pb.Matcher_EQUAL,
					Name:    "foo",
					Pattern: "bar",
				}},
			}},
			StartsAt: timestamppb.New(now),
			EndsAt:   timestamppb.New(now.Add(time.Minute)),
		}
		require.NoError(t, s.Set(t.Context(), validSil))
		validID := validSil.Id

		// Manually add an erroneous silence with zero expiration
		erroneousSil := &pb.MeshSilence{
			Silence: &pb.Silence{
				Id: "erroneous",
				MatcherSets: []*pb.MatcherSet{{
					Matchers: []*pb.Matcher{{
						Type:    pb.Matcher_EQUAL,
						Name:    "bar",
						Pattern: "baz",
					}},
				}},
				StartsAt: timestamppb.New(now),
				EndsAt:   timestamppb.New(now.Add(time.Minute)),
			},
			ExpiresAt: nil, // Zero expiration - invalid
		}
		s.st["erroneous"] = erroneousSil
		s.vi.add(s.version+1, "erroneous")
		s.version++

		// Manually add an entry to version index that doesn't exist in state
		s.vi.add(s.version+1, "missing")
		s.version++

		require.Len(t, s.st, 2)
		require.Len(t, s.vi, 3)

		// Run GC - should continue despite errors
		n, err := s.GC()
		require.Error(t, err)
		require.Contains(t, err.Error(), "zero expiration timestamp")
		require.Contains(t, err.Error(), "missing from state")

		// GC should have removed erroneous silences
		require.Equal(t, 1, n) // Only the erroneous silence with zero expiration
		require.Len(t, s.st, 1)
		require.Len(t, s.vi, 1)
		require.Contains(t, s.st, validID)
		require.NotContains(t, s.st, "erroneous")

		// Check that the error metric was incremented
		metricValue := testutil.ToFloat64(s.metrics.gcErrorsTotal)
		require.Equal(t, float64(2), metricValue)
	})
}

// This tests a regression introduced by https://github.com/prometheus/alertmanager/pull/2689.
func TestSilences_Maintenance_DefaultMaintenanceFuncDoesntCrash(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "snapshot")
		require.NoError(t, err, "creating temp file failed")
		s := &Silences{st: state{}, logger: promslog.NewNopLogger(), metrics: newMetrics(nil, nil)}
		stopc := make(chan struct{})

		done := make(chan struct{})
		go func() {
			s.Maintenance(100*time.Millisecond, f.Name(), stopc, nil)
			close(done)
		}()
		gosched()

		time.Sleep(100 * time.Millisecond)
		close(stopc)

		<-done
	})
}

func TestSilences_Maintenance_SupportsCustomCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "snapshot")
		require.NoError(t, err, "creating temp file failed")
		reg := prometheus.NewRegistry()
		s := &Silences{st: state{}, logger: promslog.NewNopLogger()}
		s.metrics = newMetrics(reg, s)
		stopc := make(chan struct{})

		var calls atomic.Int32
		var wg sync.WaitGroup

		wg.Go(func() {
			s.Maintenance(10*time.Second, f.Name(), stopc, func() (int64, error) {
				calls.Add(1)
				return 0, nil
			})
		})
		gosched()

		// Before the first tick, no maintenance executed.
		time.Sleep(9 * time.Second)
		require.EqualValues(t, 0, calls.Load())

		// Tick once.
		time.Sleep(1 * time.Second)
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load())

		// Stop the maintenance loop. We should get exactly one more execution of the maintenance func.
		close(stopc)
		wg.Wait()

		require.EqualValues(t, 2, calls.Load())

		// Check the maintenance metrics.
		require.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
# HELP alertmanager_silences_maintenance_errors_total How many maintenances were executed for silences that failed.
# TYPE alertmanager_silences_maintenance_errors_total counter
alertmanager_silences_maintenance_errors_total 0
# HELP alertmanager_silences_maintenance_total How many maintenances were executed for silences.
# TYPE alertmanager_silences_maintenance_total counter
alertmanager_silences_maintenance_total 2
`), "alertmanager_silences_maintenance_total", "alertmanager_silences_maintenance_errors_total"))
	})
}

// runtime.Gosched() does not "suspend" the current goroutine so there's no guarantee that the main goroutine won't
// be able to continue. For more see https://pkg.go.dev/runtime#Gosched.
func gosched() {
	time.Sleep(1 * time.Millisecond)
}
