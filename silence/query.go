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
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

// QueryParam expresses parameters along which silences are queried.
type QueryParam func(*query) error

type query struct {
	ids     []string
	since   *int
	filters []silenceFilter
}

// silenceFilter is a function that returns true if a silence
// should be dropped from a result set for a given time.
type silenceFilter func(*pb.Silence, *Silences, time.Time) (bool, error)

// QIDs configures a query to select the given silence IDs.
func QIDs(ids ...string) QueryParam {
	return func(q *query) error {
		if len(ids) == 0 {
			return errors.New("QIDs filter must have at least one id")
		}
		if q.since != nil {
			return fmt.Errorf("QSince cannot be used with QIDs")
		}
		q.ids = append(q.ids, ids...)
		return nil
	}
}

// QSince filters silences to those created after the provided version. This can be used to
// scan all silences which have been added after the provided version to incrementally update
// a cache.
func QSince(version int) QueryParam {
	return func(q *query) error {
		if len(q.ids) != 0 {
			return fmt.Errorf("QSince cannot be used with QIDs")
		}
		q.since = &version
		return nil
	}
}

// QMatches returns silences that match the given label set.
func QMatches(set model.LabelSet) QueryParam {
	return func(q *query) error {
		f := func(sil *pb.Silence, s *Silences, _ time.Time) (bool, error) {
			m, err := s.mi.get(sil)
			if err != nil {
				return true, err
			}
			return m.Matches(set), nil
		}
		q.filters = append(q.filters, f)
		return nil
	}
}

// QState filters queried silences by the given states.
func QState(states ...SilenceState) QueryParam {
	return func(q *query) error {
		f := func(sil *pb.Silence, _ *Silences, at time.Time) (bool, error) {
			s := StateFromProtoAt(sil, at)

			if slices.Contains(states, s) {
				return true, nil
			}
			return false, nil
		}
		q.filters = append(q.filters, f)
		return nil
	}
}

// QueryOne queries with the given parameters and returns the first result.
// Returns ErrNotFound if the query result is empty.
func (s *Silences) QueryOne(ctx context.Context, params ...QueryParam) (*pb.Silence, error) {
	_, span := tracer.Start(ctx, "silence.Silences.QueryOne",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	res, _, err := s.Query(ctx, params...)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	return res[0], nil
}

// Query for silences based on the given query parameters. It returns the
// resulting silences and the state version the result is based on.
func (s *Silences) Query(ctx context.Context, params ...QueryParam) ([]*pb.Silence, int, error) {
	return s.QueryAt(ctx, s.nowUTC(), params...)
}

// QueryAt evaluates query filters at the given time.
// It reads the current stored records, not a historical snapshot.
func (s *Silences) QueryAt(ctx context.Context, at time.Time, params ...QueryParam) ([]*pb.Silence, int, error) {
	_, span := tracer.Start(ctx, "silence.Silences.Query",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()
	s.metrics.queriesTotal.Inc()
	defer prometheus.NewTimer(s.metrics.queryDuration).ObserveDuration()

	q := &query{}
	for _, p := range params {
		if err := p(q); err != nil {
			s.metrics.queryErrorsTotal.Inc()
			return nil, s.Version(), err
		}
	}
	sils, version, err := s.query(q, at)
	if err != nil {
		s.metrics.queryErrorsTotal.Inc()
	}
	return sils, version, err
}

// Version of the silence state.
func (s *Silences) Version() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.version
}

// CountState counts silences by state.
func (s *Silences) CountState(ctx context.Context, states ...SilenceState) (int, error) {
	_, span := tracer.Start(ctx, "silence.Silences.CountState",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	now := s.nowUTC()
	count := 0

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	for _, sv := range s.vi {
		if slices.Contains(states, StateFromProtoAt(s.st[sv.id].Silence, now)) {
			count++
		}
	}
	return count, nil
}

// query executes the given query and returns the resulting silences.
func (s *Silences) query(q *query, now time.Time) ([]*pb.Silence, int, error) {
	var res []*pb.Silence
	var err error

	scannedCount := 0
	defer func() {
		s.metrics.queryScannedTotal.Add(float64(scannedCount))
	}()

	// appendIfFiltersMatch appends the given silence to the result set
	// if it matches all filters in the query. In case of a filter error, the error is returned.
	appendIfFiltersMatch := func(res []*pb.Silence, sil *pb.Silence) ([]*pb.Silence, error) {
		for _, f := range q.filters {
			matches, err := f(sil, s, now)
			// In case of error return it immediately and don't process further filters.
			if err != nil {
				return res, err
			}
			// If one filter doesn't match, return the result unchanged, immediately.
			if !matches {
				return res, nil
			}
		}
		// All filters matched, append the silence to the result.
		return append(res, cloneSilence(sil)), nil
	}

	// Preallocate result slice if we have IDs (if not this will be a no-op)
	res = make([]*pb.Silence, 0, len(q.ids))

	// Take a read lock on Silences: we can read but not modify the Silences struct.
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	// If we have IDs, only consider the silences with the given IDs, if they exist.
	if q.ids != nil {
		for _, id := range q.ids {
			if sil, ok := s.st[id]; ok {
				scannedCount++
				// append the silence to the results if it satisfies the query.
				res, err = appendIfFiltersMatch(res, sil.Silence)
				if err != nil {
					return nil, s.version, err
				}
			}
		}
	} else {
		start := 0
		if q.since != nil {
			var found bool
			start, found = s.vi.findVersionGreaterThan(*q.since)
			// no new silences, nothing to do
			if !found {
				return res, s.version, nil
			}
			// Track how many silences we skipped using the version index.
			s.metrics.querySkippedTotal.Add(float64(start))
		}
		// Preallocate result slice with a reasonable capacity. If we are
		// scanning less than 64 silences, we can allocate that many,
		// otherwise we just allocate 64 and let it grow as needed.
		res = make([]*pb.Silence, 0, min(64, len(s.vi)-start))
		for _, sv := range s.vi[start:] {
			scannedCount++
			sil := s.st[sv.id]
			// append the silence to the results if it satisfies the query.
			res, err = appendIfFiltersMatch(res, sil.Silence)
			if err != nil {
				return nil, s.version, err
			}
		}
	}

	return res, s.version, nil
}
