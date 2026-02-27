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

// Package silence provides a storage for silences, which can share its
// state over a mesh network and snapshot it.
package silence

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"regexp"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/coder/quartz"
	uuid "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/cluster"
	"github.com/prometheus/alertmanager/matcher/compat"
	"github.com/prometheus/alertmanager/pkg/labels"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/types"
)

var tracer = otel.Tracer("github.com/prometheus/alertmanager/silence")

// ErrNotFound is returned if a silence was not found.
var ErrNotFound = errors.New("silence not found")

// ErrInvalidState is returned if the state isn't valid.
var ErrInvalidState = errors.New("invalid state")

type matcherIndex map[string]labels.MatcherSet

// get retrieves the matcher set for a given silence.
func (c matcherIndex) get(s *pb.Silence) (labels.MatcherSet, error) {
	if m, ok := c[s.Id]; ok {
		return m, nil
	}
	return nil, ErrNotFound
}

// add compiles a silences' matchers and adds them to the cache.
// It returns the compiled matcher set.
func (c matcherIndex) add(s *pb.Silence) (labels.MatcherSet, error) {
	matcherSet := make(labels.MatcherSet, 0, len(s.MatcherSets))

	for _, ms := range s.MatcherSets {
		matchers := make(labels.Matchers, len(ms.Matchers))

		for i, m := range ms.Matchers {
			var mt labels.MatchType
			switch m.Type {
			case pb.Matcher_EQUAL:
				mt = labels.MatchEqual
			case pb.Matcher_NOT_EQUAL:
				mt = labels.MatchNotEqual
			case pb.Matcher_REGEXP:
				mt = labels.MatchRegexp
			case pb.Matcher_NOT_REGEXP:
				mt = labels.MatchNotRegexp
			default:
				return nil, fmt.Errorf("unknown matcher type %q", m.Type)
			}
			matcher, err := labels.NewMatcher(mt, m.Name, m.Pattern)
			if err != nil {
				return nil, err
			}

			matchers[i] = matcher
		}

		matcherSet = append(matcherSet, &matchers)
	}

	c[s.Id] = matcherSet
	return matcherSet, nil
}

// silenceVersion associates a silence with the Silences version when it was created.
type silenceVersion struct {
	id      string
	version int
}

// versionIndex is a index into Silences ordered by the version of Silences when the
// silence was created. The index is always sorted from lowest to highest version.
//
// The versionIndex allows clients of Silences.Query to incrementally update local caches
// of query results. Instead of a new version requiring the client to scan  everything
// again to get an up-to-date picture of Silences, they can use the versionIndex to figure
// out which silences have been added since the last version they saw. This means they can
// just scan the NEW silences, rather than all of them.
type versionIndex []silenceVersion

// add pushes a new silenceVersionMapping to the back of the silenceVersionIndex. It does not
// validate the input.
func (s *versionIndex) add(version int, sil string) {
	*s = append(*s, silenceVersion{version: version, id: sil})
}

// findVersionGreaterThan uses a log(n) search to find the first index of the versionIndex
// which has a version higher than version. If any entries with a higher version exist,
// it returns true and the starting index (which is guaranteed to be a valid index into
// the slice). Otherwise it returns false.
func (s versionIndex) findVersionGreaterThan(version int) (index int, found bool) {
	startIdx := sort.Search(len(s), func(i int) bool {
		return s[i].version > version
	})
	return startIdx, startIdx < len(s)
}

// Silencer binds together a AlertMarker and a Silences to implement the Muter
// interface.
type Silencer struct {
	silences *Silences
	cache    *cache
	marker   types.AlertMarker
	logger   *slog.Logger
}

// NewSilencer returns a new Silencer.
func NewSilencer(silences *Silences, marker types.AlertMarker, logger *slog.Logger) *Silencer {
	return &Silencer{
		silences: silences,
		cache:    &cache{entries: map[model.Fingerprint]*cacheEntry{}},
		marker:   marker,
		logger:   logger,
	}
}

// Mutes implements the Muter interface.
func (s *Silencer) Mutes(ctx context.Context, lset model.LabelSet) bool {
	fp := lset.Fingerprint()
	ctx, span := tracer.Start(ctx, "silence.Silencer.Mutes",
		trace.WithAttributes(
			attribute.String("alerting.alert.fingerprint", fp.String()),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

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
		span.AddEvent("No new silences to match since last check",
			trace.WithAttributes(
				attribute.Int("alerting.silences.cache.count", cachedEntry.count()),
			),
		)
		return false
	}
	// Either there are new silences and we need to check if those match lset or there were
	// silences last time we queried so we need to see if those are still active/have become
	// active. It's possible for there to be both old and new silences.

	if cachedEntry.count() > 0 {
		// there were old silences for this lset, we need to find them to check if they
		// are still active/pending, or have ended.
		var err error
		oldSils, _, err = s.silences.Query(
			ctx,
			QIDs(cachedEntry.silenceIDs...),
			QState(SilenceStateActive, SilenceStatePending),
		)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
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
		newSils, newVersion, err = s.silences.Query(
			ctx,
			QSince(cachedEntry.version),
			QState(SilenceStateActive, SilenceStatePending),
			QMatches(lset),
		)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
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
		s.marker.SetActiveOrSilenced(fp, nil)
		span.AddEvent("No silences to match", trace.WithAttributes(
			attribute.Int("alerting.silences.count", totalSilences),
		))
		return false
	}

	// It is still possible that nothing has changed, but finding out is not
	// much less effort than just recreating the IDs from the query
	// result. So let's do it in any case. Note that we cannot reuse the
	// current ID slices for concurrency reasons.
	activeIDs := make([]string, 0, totalSilences)
	allIDs := make([]string, 0, totalSilences)
	seen := make(map[string]struct{}, totalSilences)
	now := s.silences.nowUTC()

	// Categorize old and new silences by their current state.
	// oldSils and newSils may overlap if a cached silence was updated
	// (receiving a new version), so we deduplicate by ID.
	for _, sils := range [...][]*pb.Silence{oldSils, newSils} {
		for _, sil := range sils {
			if _, ok := seen[sil.Id]; ok {
				continue
			}
			seen[sil.Id] = struct{}{}
			switch getState(sil, now) {
			case SilenceStatePending:
				allIDs = append(allIDs, sil.Id)
			case SilenceStateActive:
				activeIDs = append(activeIDs, sil.Id)
				allIDs = append(allIDs, sil.Id)
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
	// TODO: remove this sort once the marker is removed.
	sort.Strings(activeIDs)

	s.cache.set(fp, newCacheEntry(newVersion, allIDs...))
	s.marker.SetActiveOrSilenced(fp, activeIDs)

	t := trace.WithAttributes(
		attribute.Int("alerting.silences.active.count", len(activeIDs)),
		attribute.Int("alerting.silences.pending.count", len(allIDs)-len(activeIDs)),
		attribute.Int("alerting.silences.total.count", len(allIDs)),
	)

	mutes := len(activeIDs) > 0
	if mutes {
		span.AddEvent("Silencer mutes alert", t)
	} else {
		span.AddEvent("Silencer does not mute alert", t)
	}
	return mutes
}

// The following methods implement mem.AlertStoreCallback.
func (s *Silencer) PreStore(_ *types.Alert, _ bool) error { return nil }
func (s *Silencer) PostStore(_ *types.Alert, _ bool)      {}
func (s *Silencer) PostDelete(alert *types.Alert)         {}
func (s *Silencer) PostGC(ff model.Fingerprints) {
	for _, fp := range ff {
		s.cache.delete(fp)
	}
}

// Silences holds a silence state that can be modified, queried, and snapshot.
type Silences struct {
	clock quartz.Clock

	logger    *slog.Logger
	metrics   *metrics
	retention time.Duration
	limits    Limits

	mtx       sync.RWMutex
	st        state
	version   int // Increments whenever silences are added.
	broadcast func([]byte)
	mi        matcherIndex
	vi        versionIndex
}

// Limits contains the limits for silences.
type Limits struct {
	// MaxSilences limits the maximum number of silences, including expired
	// silences.
	MaxSilences func() int
	// MaxSilenceSizeBytes is the maximum size of an individual silence as
	// stored on disk.
	MaxSilenceSizeBytes func() int
}

// MaintenanceFunc represents the function to run as part of the periodic maintenance for silences.
// It returns the size of the snapshot taken or an error if it failed.
type MaintenanceFunc func() (int64, error)

type metrics struct {
	gcDuration                            prometheus.Summary
	gcErrorsTotal                         prometheus.Counter
	snapshotDuration                      prometheus.Summary
	snapshotSize                          prometheus.Gauge
	queriesTotal                          prometheus.Counter
	queryErrorsTotal                      prometheus.Counter
	queryDuration                         prometheus.Histogram
	queryScannedTotal                     prometheus.Counter
	querySkippedTotal                     prometheus.Counter
	silencesActive                        prometheus.GaugeFunc
	silencesPending                       prometheus.GaugeFunc
	silencesExpired                       prometheus.GaugeFunc
	stateSize                             prometheus.Gauge
	matcherIndexSize                      prometheus.Gauge
	versionIndexSize                      prometheus.Gauge
	propagatedMessagesTotal               prometheus.Counter
	maintenanceTotal                      prometheus.Counter
	maintenanceErrorsTotal                prometheus.Counter
	matcherCompileIndexSilenceErrorsTotal prometheus.Counter
	matcherCompileLoadSnapshotErrorsTotal prometheus.Counter
}

func newSilenceMetricByState(r prometheus.Registerer, s *Silences, st SilenceState) prometheus.GaugeFunc {
	return promauto.With(r).NewGaugeFunc(
		prometheus.GaugeOpts{
			Name:        "alertmanager_silences",
			Help:        "How many silences by state.",
			ConstLabels: prometheus.Labels{"state": string(st)},
		},
		func() float64 {
			count, err := s.CountState(context.Background(), st)
			if err != nil {
				s.logger.Error("Counting silences failed", "err", err)
			}
			return float64(count)
		},
	)
}

func newMetrics(r prometheus.Registerer, s *Silences) *metrics {
	m := &metrics{}

	m.gcDuration = promauto.With(r).NewSummary(prometheus.SummaryOpts{
		Name:       "alertmanager_silences_gc_duration_seconds",
		Help:       "Duration of the last silence garbage collection cycle.",
		Objectives: map[float64]float64{},
	})
	m.gcErrorsTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_gc_errors_total",
		Help: "How many silence GC errors were encountered.",
	})
	m.snapshotDuration = promauto.With(r).NewSummary(prometheus.SummaryOpts{
		Name:       "alertmanager_silences_snapshot_duration_seconds",
		Help:       "Duration of the last silence snapshot.",
		Objectives: map[float64]float64{},
	})
	m.snapshotSize = promauto.With(r).NewGauge(prometheus.GaugeOpts{
		Name: "alertmanager_silences_snapshot_size_bytes",
		Help: "Size of the last silence snapshot in bytes.",
	})
	m.maintenanceTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_maintenance_total",
		Help: "How many maintenances were executed for silences.",
	})
	m.maintenanceErrorsTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_maintenance_errors_total",
		Help: "How many maintenances were executed for silences that failed.",
	})
	matcherCompileErrorsTotal := promauto.With(r).NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmanager_silences_matcher_compile_errors_total",
			Help: "How many silence matcher compilations failed.",
		},
		[]string{"stage"},
	)
	m.matcherCompileIndexSilenceErrorsTotal = matcherCompileErrorsTotal.WithLabelValues("index")
	m.matcherCompileLoadSnapshotErrorsTotal = matcherCompileErrorsTotal.WithLabelValues("load_snapshot")
	m.queriesTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_queries_total",
		Help: "How many silence queries were received.",
	})
	m.queryErrorsTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_query_errors_total",
		Help: "How many silence received queries did not succeed.",
	})
	m.queryDuration = promauto.With(r).NewHistogram(prometheus.HistogramOpts{
		Name:                            "alertmanager_silences_query_duration_seconds",
		Help:                            "Duration of silence query evaluation.",
		Buckets:                         prometheus.DefBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	})
	m.queryScannedTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_query_silences_scanned_total",
		Help: "How many silences were scanned during query evaluation.",
	})
	m.querySkippedTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_query_silences_skipped_total",
		Help: "How many silences were skipped during query evaluation using the version index.",
	})
	m.propagatedMessagesTotal = promauto.With(r).NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_silences_gossip_messages_propagated_total",
		Help: "Number of received gossip messages that have been further gossiped.",
	})
	if s != nil {
		m.silencesActive = newSilenceMetricByState(r, s, SilenceStateActive)
		m.silencesPending = newSilenceMetricByState(r, s, SilenceStatePending)
		m.silencesExpired = newSilenceMetricByState(r, s, SilenceStateExpired)
		m.stateSize = promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "alertmanager_silences_state_size",
			Help: "The number of silences in the state map.",
		})
		m.matcherIndexSize = promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "alertmanager_silences_matcher_index_size",
			Help: "The number of entries in the matcher cache index.",
		})
		m.versionIndexSize = promauto.With(r).NewGauge(prometheus.GaugeOpts{
			Name: "alertmanager_silences_version_index_size",
			Help: "The number of entries in the version index.",
		})
	}

	return m
}

// Options exposes configuration options for creating a new Silences object.
// Its zero value is a safe default.
type Options struct {
	// A snapshot file or reader from which the initial state is loaded.
	// None or only one of them must be set.
	SnapshotFile   string
	SnapshotReader io.Reader

	// Retention time for newly created Silences. Silences may be
	// garbage collected after the given duration after they ended.
	Retention time.Duration
	Limits    Limits

	// A logger used by background processing.
	Logger  *slog.Logger
	Metrics prometheus.Registerer
}

func (o *Options) validate() error {
	if o.SnapshotFile != "" && o.SnapshotReader != nil {
		return errors.New("only one of SnapshotFile and SnapshotReader must be set")
	}
	return nil
}

// New returns a new Silences object with the given configuration.
func New(o Options) (*Silences, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}

	s := &Silences{
		clock:     quartz.NewReal(),
		mi:        make(matcherIndex, 512),
		vi:        make(versionIndex, 0, 512),
		logger:    promslog.NewNopLogger(),
		retention: o.Retention,
		limits:    o.Limits,
		broadcast: func([]byte) {},
		st:        state{},
	}
	if o.Metrics == nil {
		return nil, errors.New("Options.Metrics is nil")
	}
	s.metrics = newMetrics(o.Metrics, s)

	if o.Logger != nil {
		s.logger = o.Logger
	}

	if o.SnapshotFile != "" {
		if r, err := os.Open(o.SnapshotFile); err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
			s.logger.Debug("silences snapshot file doesn't exist", "err", err)
		} else {
			o.SnapshotReader = r
			defer r.Close()
		}
	}

	if o.SnapshotReader != nil {
		if err := s.loadSnapshot(o.SnapshotReader); err != nil {
			return s, err
		}
	}
	return s, nil
}

func (s *Silences) nowUTC() time.Time {
	return s.clock.Now().UTC()
}

// updateSizeMetrics updates the size metrics for state, matcher index, and version index.
// Must be called while holding s.mtx.
func (s *Silences) updateSizeMetrics() {
	if s.metrics != nil && s.metrics.stateSize != nil {
		s.metrics.stateSize.Set(float64(len(s.st)))
		s.metrics.matcherIndexSize.Set(float64(len(s.mi)))
		s.metrics.versionIndexSize.Set(float64(len(s.vi)))
	}
}

// Maintenance garbage collects the silence state at the given interval. If the snapshot
// file is set, a snapshot is written to it afterwards.
// Terminates on receiving from stopc.
// If not nil, the last argument is an override for what to do as part of the maintenance - for advanced usage.
func (s *Silences) Maintenance(interval time.Duration, snapf string, stopc <-chan struct{}, override MaintenanceFunc) {
	if interval == 0 || stopc == nil {
		s.logger.Error("interval or stop signal are missing - not running maintenance")
		return
	}
	t := s.clock.NewTicker(interval)
	defer t.Stop()

	var doMaintenance MaintenanceFunc
	doMaintenance = func() (int64, error) {
		var size int64

		if _, err := s.GC(); err != nil {
			return size, err
		}
		if snapf == "" {
			return size, nil
		}
		f, err := openReplace(snapf)
		if err != nil {
			return size, err
		}
		if size, err = s.Snapshot(f); err != nil {
			f.Close()
			return size, err
		}
		return size, f.Close()
	}

	if override != nil {
		doMaintenance = override
	}

	runMaintenance := func(do MaintenanceFunc) error {
		s.metrics.maintenanceTotal.Inc()
		s.logger.Debug("Running maintenance")
		start := s.nowUTC()
		size, err := do()
		s.metrics.snapshotSize.Set(float64(size))
		if err != nil {
			s.metrics.maintenanceErrorsTotal.Inc()
			return err
		}
		s.logger.Debug("Maintenance done", "duration", s.clock.Since(start), "size", size)
		return nil
	}

Loop:
	for {
		select {
		case <-stopc:
			break Loop
		case <-t.C:
			if err := runMaintenance(doMaintenance); err != nil {
				s.logger.Error("Running maintenance failed", "err", err)
			}
		}
	}

	// No need for final maintenance if we don't want to snapshot.
	if snapf == "" {
		return
	}
	if err := runMaintenance(doMaintenance); err != nil {
		s.logger.Error("Creating shutdown snapshot failed", "err", err)
	}
}

// GC runs a garbage collection that removes silences that have ended longer
// than the configured retention time ago.
func (s *Silences) GC() (int, error) {
	start := time.Now()
	defer func() { s.metrics.gcDuration.Observe(time.Since(start).Seconds()) }()

	now := s.nowUTC()
	var n int
	var errs error

	s.mtx.Lock()
	defer s.mtx.Unlock()

	// During GC we will delete expired silences from the state map and the indices.
	// If between the last GC's deletion, and including any silences that were added
	// until now, we have more than 50% spare capacity, we want to reallocate to a smaller
	// slice, while still leaving some growth buffer.
	needsRealloc := cap(s.vi) > 1024 && len(s.vi) < cap(s.vi)/2

	var targetVi versionIndex
	if needsRealloc {
		// Allocate new slice with growth buffer.
		newCap := max(len(s.vi)*5/4, 1024)
		targetVi = make(versionIndex, 0, newCap)
	} else {
		targetVi = s.vi[:0]
	}

	// Iterate state map directly (fast - no extra lookups).
	for _, sv := range s.vi {
		sil, ok := s.st[sv.id]
		expire := false
		if !ok {
			// Silence in version index but not in state - remove from version index and count error
			s.metrics.gcErrorsTotal.Inc()
			errs = errors.Join(errs, fmt.Errorf("silence %s in version index missing from state", sv.id))
			// not adding to targetVi effectively removes it
			continue
		}
		if sil.ExpiresAt == nil || sil.ExpiresAt.AsTime().IsZero() {
			// Invalid expiration timestamp - remove silence and count error
			s.metrics.gcErrorsTotal.Inc()
			errs = errors.Join(errs, fmt.Errorf("silence %s has zero expiration timestamp", sil.Silence.Id))
			expire = true
		}
		if expire || !sil.ExpiresAt.AsTime().After(now) {
			delete(s.st, sil.Silence.Id)
			delete(s.mi, sil.Silence.Id)
			n++
		} else {
			targetVi = append(targetVi, sv)
		}
	}

	if !needsRealloc {
		// If we didn't reallocate, clear tail to prevent string pointer leaks
		clear(s.vi[len(targetVi):])
	}
	s.vi = targetVi
	s.updateSizeMetrics()

	return n, errs
}

func validateMatcher(m *pb.Matcher) error {
	if !compat.IsValidLabelName(model.LabelName(m.Name)) {
		return fmt.Errorf("invalid label name %q", m.Name)
	}
	switch m.Type {
	case pb.Matcher_EQUAL, pb.Matcher_NOT_EQUAL:
		if !model.LabelValue(m.Pattern).IsValid() {
			return fmt.Errorf("invalid label value %q", m.Pattern)
		}
	case pb.Matcher_REGEXP, pb.Matcher_NOT_REGEXP:
		if _, err := regexp.Compile(m.Pattern); err != nil {
			return fmt.Errorf("invalid regular expression %q: %w", m.Pattern, err)
		}
	default:
		return fmt.Errorf("unknown matcher type %q", m.Type)
	}
	return nil
}

func matchesEmpty(m *pb.Matcher) bool {
	switch m.Type {
	case pb.Matcher_EQUAL:
		return m.Pattern == ""
	case pb.Matcher_REGEXP:
		matched, _ := regexp.MatchString(m.Pattern, "")
		return matched
	default:
		return false
	}
}

func validateSilence(s *pb.Silence) error {
	// Convert old-style Matchers to MatcherSets for backward compatibility
	postprocessUnmarshalledSilence(s)

	if len(s.MatcherSets) == 0 {
		return errors.New("at least one matcher set required")
	}

	for setIdx, ms := range s.MatcherSets {
		if len(ms.Matchers) == 0 {
			return fmt.Errorf("matcher set %d is empty", setIdx)
		}
		allMatchEmpty := true

		for i, m := range ms.Matchers {
			if err := validateMatcher(m); err != nil {
				return fmt.Errorf("invalid label matcher %d in set %d: %w", i, setIdx, err)
			}
			allMatchEmpty = allMatchEmpty && matchesEmpty(m)
		}
		if allMatchEmpty {
			return fmt.Errorf("matcher set %d: at least one matcher must not match the empty string", setIdx)
		}
	}

	if s.StartsAt == nil || s.StartsAt.AsTime().IsZero() {
		return errors.New("invalid zero start timestamp")
	}
	if s.EndsAt == nil || s.EndsAt.AsTime().IsZero() {
		return errors.New("invalid zero end timestamp")
	}
	if s.EndsAt.AsTime().Before(s.StartsAt.AsTime()) {
		return errors.New("end time must not be before start time")
	}
	return nil
}

// cloneSilence returns a copy of a silence.
func cloneSilence(sil *pb.Silence) *pb.Silence {
	return proto.Clone(sil).(*pb.Silence)
}

func (s *Silences) checkSizeLimits(msil *pb.MeshSilence) error {
	if s.limits.MaxSilenceSizeBytes != nil {
		n := proto.Size(msil)
		if m := s.limits.MaxSilenceSizeBytes(); m > 0 && n > m {
			return fmt.Errorf("silence exceeded maximum size: %d bytes (limit: %d bytes)", n, m)
		}
	}
	return nil
}

func (s *Silences) indexSilence(sil *pb.Silence) {
	s.version++
	s.vi.add(s.version, sil.Id)
	_, err := s.mi.add(sil)
	if err != nil {
		s.metrics.matcherCompileIndexSilenceErrorsTotal.Inc()
		s.logger.Error("Failed to compile silence matchers", "silence_id", sil.Id, "err", err)
	}
}

func (s *Silences) getSilence(id string) (*pb.Silence, bool) {
	msil, ok := s.st[id]
	if !ok {
		return nil, false
	}
	return msil.Silence, true
}

func (s *Silences) toMeshSilence(sil *pb.Silence) *pb.MeshSilence {
	return &pb.MeshSilence{
		Silence:   sil,
		ExpiresAt: timestamppb.New(sil.EndsAt.AsTime().Add(s.retention)),
	}
}

func (s *Silences) setSilence(msil *pb.MeshSilence, now time.Time) error {
	b, err := marshalMeshSilence(msil)
	if err != nil {
		return err
	}
	_, added := s.st.merge(msil, now)
	if added {
		s.indexSilence(msil.Silence)
		s.updateSizeMetrics()
	}
	s.broadcast(b)
	return nil
}

// Set the specified silence. If a silence with the ID already exists and the modification
// modifies history, the old silence gets expired and a new one is created.
func (s *Silences) Set(ctx context.Context, sil *pb.Silence) error {
	_, span := tracer.Start(ctx, "silences.Set")
	defer span.End()

	now := s.nowUTC()
	if sil.StartsAt == nil || sil.StartsAt.AsTime().IsZero() {
		sil.StartsAt = timestamppb.New(now)
	}

	if err := validateSilence(sil); err != nil {
		return fmt.Errorf("invalid silence: %w", err)
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()

	prev, ok := s.getSilence(sil.Id)
	if sil.Id != "" && !ok {
		return ErrNotFound
	}

	if ok && canUpdate(prev, sil, now) {
		sil.UpdatedAt = timestamppb.New(now)
		msil := s.toMeshSilence(sil)
		if err := s.checkSizeLimits(msil); err != nil {
			return err
		}
		return s.setSilence(msil, now)
	}

	// If we got here it's either a new silence or a replacing one (which would
	// also create a new silence) so we need to make sure we have capacity for
	// the new silence.
	if s.limits.MaxSilences != nil {
		if m := s.limits.MaxSilences(); m > 0 && len(s.st)+1 > m {
			return fmt.Errorf("exceeded maximum number of silences: %d (limit: %d)", len(s.st), m)
		}
	}

	uid, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("generate uuid: %w", err)
	}
	sil.Id = uid.String()

	if sil.StartsAt.AsTime().Before(now) {
		sil.StartsAt = timestamppb.New(now)
	}
	sil.UpdatedAt = timestamppb.New(now)

	msil := s.toMeshSilence(sil)
	if err := s.checkSizeLimits(msil); err != nil {
		return err
	}

	if ok && getState(prev, s.nowUTC()) != SilenceStateExpired {
		// We cannot update the silence, expire the old one to leave a history of
		// the silence before modification.
		if err := s.expire(prev.Id); err != nil {
			return fmt.Errorf("expire previous silence: %w", err)
		}
	}

	return s.setSilence(msil, now)
}

// canUpdate returns true if silence a can be updated to b without
// affecting the historic view of silencing.
func canUpdate(a, b *pb.Silence, now time.Time) bool {
	if !slices.EqualFunc(a.MatcherSets, b.MatcherSets, func(x, y *pb.MatcherSet) bool {
		return proto.Equal(x, y)
	}) {
		return false
	}
	// Allowed timestamp modifications depend on the current time.
	switch st := getState(a, now); st {
	case SilenceStateActive:
		if a.StartsAt.AsTime().Unix() != b.StartsAt.AsTime().Unix() {
			return false
		}
		if b.EndsAt.AsTime().Before(now) {
			return false
		}
	case SilenceStatePending:
		if b.StartsAt.AsTime().Before(now) {
			return false
		}
	case SilenceStateExpired:
		return false
	default:
		panic("unknown silence state")
	}
	return true
}

// Expire the silence with the given ID immediately.
func (s *Silences) Expire(ctx context.Context, id string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	_, span := tracer.Start(ctx, "silences.Expire", trace.WithAttributes(
		attribute.String("alerting.silence.id", id),
	))
	defer span.End()

	return s.expire(id)
}

// Expire the silence with the given ID immediately.
// It is idempotent, nil is returned if the silence already expired before it is GC'd.
// If the silence is not found an error is returned.
func (s *Silences) expire(id string) error {
	sil, ok := s.getSilence(id)
	if !ok {
		return ErrNotFound
	}
	sil = cloneSilence(sil)
	now := s.nowUTC()

	switch getState(sil, now) {
	case SilenceStateExpired:
		return nil
	case SilenceStateActive:
		sil.EndsAt = timestamppb.New(now)
	case SilenceStatePending:
		// Set both to now to make Silence move to "expired" state
		sil.StartsAt = timestamppb.New(now)
		sil.EndsAt = timestamppb.New(now)
	}
	sil.UpdatedAt = timestamppb.New(now)
	return s.setSilence(s.toMeshSilence(sil), now)
}

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

// getState returns a silence's SilenceState at the given timestamp.
func getState(sil *pb.Silence, ts time.Time) SilenceState {
	if ts.Before(sil.StartsAt.AsTime()) {
		return SilenceStatePending
	}
	if ts.After(sil.EndsAt.AsTime()) {
		return SilenceStateExpired
	}
	return SilenceStateActive
}

// QState filters queried silences by the given states.
func QState(states ...SilenceState) QueryParam {
	return func(q *query) error {
		f := func(sil *pb.Silence, _ *Silences, now time.Time) (bool, error) {
			s := getState(sil, now)

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
	sils, version, err := s.query(q, s.nowUTC())
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
	// This could probably be optimized.
	sils, _, err := s.Query(ctx, QState(states...))
	if err != nil {
		return -1, err
	}
	return len(sils), nil
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

// loadSnapshot loads a snapshot generated by Snapshot() into the state.
// Any previous state is wiped.
func (s *Silences) loadSnapshot(r io.Reader) error {
	st, err := decodeState(r)
	if err != nil {
		return err
	}

	mi := make(matcherIndex, len(st)) // for a map, len is ok as a size hint.
	// Choose new version index capacity with some growth buffer.
	vi := make(versionIndex, 0, max(len(st)*5/4, 1024))

	for _, e := range st {
		// Comments list was moved to a single comment. Upgrade on loading the snapshot.
		if len(e.Silence.Comments) > 0 {
			e.Silence.Comment = e.Silence.Comments[0].Comment
			e.Silence.CreatedBy = e.Silence.Comments[0].Author
			e.Silence.Comments = nil
		}
		// Add to matcher index, and only if successful, to the new state.
		if _, err := mi.add(e.Silence); err != nil {
			s.metrics.matcherCompileLoadSnapshotErrorsTotal.Inc()
			s.logger.Error("Failed to compile silence matchers during snapshot load", "silence_id", e.Silence.Id, "err", err)
		} else {
			st[e.Silence.Id] = e

			vi.add(s.version+1, e.Silence.Id)
		}
	}
	s.mtx.Lock()
	s.st = st
	s.mi = mi
	s.vi = vi
	s.version++
	s.updateSizeMetrics()
	s.mtx.Unlock()

	return nil
}

// Snapshot writes the full internal state into the writer and returns the number of bytes
// written.
func (s *Silences) Snapshot(w io.Writer) (int64, error) {
	start := time.Now()
	defer func() { s.metrics.snapshotDuration.Observe(time.Since(start).Seconds()) }()

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	b, err := s.st.MarshalBinary()
	if err != nil {
		return 0, err
	}

	return io.Copy(w, bytes.NewReader(b))
}

// MarshalBinary serializes all silences.
func (s *Silences) MarshalBinary() ([]byte, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.st.MarshalBinary()
}

// Merge merges silence state received from the cluster with the local state.
func (s *Silences) Merge(b []byte) error {
	st, err := decodeState(bytes.NewReader(b))
	if err != nil {
		return err
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()

	now := s.nowUTC()

	for _, e := range st {
		merged, added := s.st.merge(e, now)
		if merged {
			if added {
				s.indexSilence(e.Silence)
			}
			if !cluster.OversizedMessage(b) {
				// If this is the first we've seen the message and it's
				// not oversized, gossip it to other nodes. We don't
				// propagate oversized messages because they're sent to
				// all nodes already.
				s.broadcast(b)
				s.metrics.propagatedMessagesTotal.Inc()
				s.logger.Debug("Gossiping new silence", "silence", e)
			}
		}
	}
	s.updateSizeMetrics()
	return nil
}

// SetBroadcast sets the provided function as the one creating data to be
// broadcast.
func (s *Silences) SetBroadcast(f func([]byte)) {
	s.mtx.Lock()
	s.broadcast = f
	s.mtx.Unlock()
}

type state map[string]*pb.MeshSilence

// merge returns two bools: the first is true when merge caused a state change. The second
// is true if that state change added a new silence. In other words, the second return is
// true whenever a silence with a new ID has been added to the state as a result of merge.
func (s state) merge(e *pb.MeshSilence, now time.Time) (bool, bool) {
	id := e.Silence.Id
	if e.ExpiresAt.AsTime().Before(now) {
		return false, false
	}
	// Comments list was moved to a single comment. Apply upgrade
	// on silences received from peers.
	if len(e.Silence.Comments) > 0 {
		e.Silence.Comment = e.Silence.Comments[0].Comment
		e.Silence.CreatedBy = e.Silence.Comments[0].Author
		e.Silence.Comments = nil
	}

	prev, ok := s[id]
	if !ok || prev.Silence.UpdatedAt.AsTime().Before(e.Silence.UpdatedAt.AsTime()) {
		s[id] = e
		return true, !ok
	}
	return false, false
}

func (s state) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	for _, e := range s {
		if _, err := protodelim.MarshalTo(&buf, e); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func decodeState(r io.Reader) (state, error) {
	st := state{}
	br := bufio.NewReader(r)
	for {
		var s pb.MeshSilence
		err := protodelim.UnmarshalFrom(br, &s)
		if err == nil {
			if s.Silence == nil {
				return nil, ErrInvalidState
			}
			postprocessUnmarshalledSilence(s.Silence)
			st[s.Silence.Id] = &s
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return nil, err
	}
	return st, nil
}

// prepareSilenceForMarshalling prepares a silence for marshalling by copying
// the first matcher set to the matchers field for backward compatibility with
// older alertmanager versions.
func prepareSilenceForMarshalling(sil *pb.Silence) {
	if len(sil.MatcherSets) > 0 {
		sil.Matchers = sil.MatcherSets[0].Matchers
	}
}

// postprocessUnmarshalledSilence processes a silence after unmarshalling by
// moving matchers to MatcherSets if needed for backward compatibility.
func postprocessUnmarshalledSilence(sil *pb.Silence) {
	// maintain compatibility with older versions of Alertmanager
	// if the silence was serialized with the old format we need to move the matchers from sil.Matchers
	// to sil.MatcherSets
	if len(sil.MatcherSets) == 0 && len(sil.Matchers) > 0 {
		sil.MatcherSets = append(sil.MatcherSets, &pb.MatcherSet{Matchers: sil.Matchers})
	}
	sil.Matchers = nil
}

func marshalMeshSilence(e *pb.MeshSilence) ([]byte, error) {
	// Make a copy to avoid modifying the original silence
	meshCopy := &pb.MeshSilence{
		Silence:   cloneSilence(e.Silence),
		ExpiresAt: e.ExpiresAt,
	}
	prepareSilenceForMarshalling(meshCopy.Silence)
	var buf bytes.Buffer
	if _, err := protodelim.MarshalTo(&buf, meshCopy); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// replaceFile wraps a file that is moved to another filename on closing.
type replaceFile struct {
	*os.File
	filename string
}

func (f *replaceFile) Close() error {
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.File.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), f.filename)
}

// openReplace opens a new temporary file that is moved to filename on closing.
func openReplace(filename string) (*replaceFile, error) {
	tmpFilename := fmt.Sprintf("%s.%x", filename, uint64(rand.Int63()))

	f, err := os.Create(tmpFilename)
	if err != nil {
		return nil, err
	}

	rf := &replaceFile{
		File:     f,
		filename: filename,
	}
	return rf, nil
}
