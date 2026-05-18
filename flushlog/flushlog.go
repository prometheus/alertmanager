// Copyright 2016 Prometheus Team
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

// Package flushlog implements a garbage-collected and snapshottable log of flushes.
package flushlog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/alertmanager/cluster/clusterutil"
	pb "github.com/prometheus/alertmanager/flushlog/flushlogpb"
)

// ErrNotFound is returned for empty query results.
var ErrNotFound = errors.New("not found")

// ErrInvalidState is returned if the state isn't valid.
var ErrInvalidState = errors.New("invalid state")

// FlushLog holds the flush log state for groups that are active.
type FlushLog struct {
	clock clock.Clock

	logger    log.Logger
	metrics   *metrics
	retention time.Duration

	// For now we only store the most recently added log entry.
	// The key is a serialized concatenation of group key and receiver.
	mtx                sync.RWMutex
	st                 state
	broadcast          func([]byte)
	isReliableDelivery func([]byte) bool
}

// MaintenanceFunc represents the function to run as part of the periodic maintenance for the flushlog.
// It returns the size of the snapshot taken or an error if it failed.
type MaintenanceFunc func() (int64, error)

type metrics struct {
	gcDuration              prometheus.Summary
	snapshotDuration        prometheus.Summary
	snapshotSize            prometheus.Gauge
	queriesTotal            prometheus.Counter
	queryErrorsTotal        prometheus.Counter
	queryDuration           prometheus.Histogram
	propagatedMessagesTotal prometheus.Counter
	maintenanceTotal        prometheus.Counter
	maintenanceErrorsTotal  prometheus.Counter
}

func newMetrics(r prometheus.Registerer) *metrics {
	m := &metrics{}

	m.gcDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "alertmanager_flushlog_gc_duration_seconds",
		Help:       "Duration of the last flush log garbage collection cycle.",
		Objectives: map[float64]float64{},
	})
	m.snapshotDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "alertmanager_flushlog_snapshot_duration_seconds",
		Help:       "Duration of the last flush log snapshot.",
		Objectives: map[float64]float64{},
	})
	m.snapshotSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "alertmanager_flushlog_snapshot_size_bytes",
		Help: "Size of the last flush log snapshot in bytes.",
	})
	m.maintenanceTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_flushlog_maintenance_total",
		Help: "How many maintenances were executed for the flush log.",
	})
	m.maintenanceErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_flushlog_maintenance_errors_total",
		Help: "How many maintenances were executed for the flush log that failed.",
	})
	m.queriesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_flushlog_queries_total",
		Help: "Number of flush log queries were received.",
	})
	m.queryErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_flushlog_query_errors_total",
		Help: "Number flush log received queries that failed.",
	})
	m.queryDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                            "alertmanager_flushlog_query_duration_seconds",
		Help:                            "Duration of flush log query evaluation.",
		Buckets:                         prometheus.DefBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	})
	m.propagatedMessagesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_flushlog_gossip_messages_propagated_total",
		Help: "Number of received gossip messages that have been further gossiped.",
	})

	if r != nil {
		r.MustRegister(
			m.gcDuration,
			m.snapshotDuration,
			m.snapshotSize,
			m.queriesTotal,
			m.queryErrorsTotal,
			m.queryDuration,
			m.propagatedMessagesTotal,
			m.maintenanceTotal,
			m.maintenanceErrorsTotal,
		)
	}
	return m
}

type state map[uint64]*pb.MeshFlushLog

func (s state) clone() state {
	c := make(state, len(s))
	for k, v := range s {
		c[k] = v
	}
	return c
}

// merge returns true or false whether the MeshFlushLog was merged or
// not. This information is used to decide to gossip the message further.
//
// Tombstones (entries with zero ExpiresAt) are retained in state rather than
// being removed: a deletion that drops the local entry would let an
// in-flight refresh broadcast for the same group_fingerprint re-add the
// entry, causing a gossip ping-pong (the deleted and refreshed messages
// would oscillate between peers). Keeping the tombstone with a Timestamp
// at least as recent as the deleted entry's lets the entry-path's
// Timestamp.Before check reject stale refreshes naturally. Tombstones are
// GC'd in FlushLog.GC once Timestamp + retention has elapsed.
func (s state) merge(e *pb.MeshFlushLog, now time.Time) bool {
	if e.ExpiresAt.IsZero() { // tombstone
		prev, ok := s[e.FlushLog.GroupFingerprint]
		if !ok {
			// No prior knowledge of this group; record the tombstone so any
			// future stale refresh broadcast can be rejected by Timestamp.
			s[e.FlushLog.GroupFingerprint] = e
			return true
		}
		if prev.ExpiresAt.IsZero() {
			// Prev is already a tombstone; only a strictly newer Timestamp
			// is propagated further to avoid endless re-gossip of identical
			// tombstones.
			if prev.FlushLog.Timestamp.Before(e.FlushLog.Timestamp) {
				s[e.FlushLog.GroupFingerprint] = e
				return true
			}
			return false
		}
		// Prev is a live entry; tombstone wins if its Timestamp is at least
		// as new as the entry's (since on near-expiry the flushlog gets
		// re-broadcast with the same Timestamp, the delete needs to defeat
		// in-flight refreshes that carry the same Timestamp).
		if prev.FlushLog.Timestamp.Before(e.FlushLog.Timestamp) || prev.FlushLog.Timestamp.Equal(e.FlushLog.Timestamp) {
			s[e.FlushLog.GroupFingerprint] = e
			return true
		}
		return false
	} else if e.ExpiresAt.Before(now) {
		return false
	}

	prev, ok := s[e.FlushLog.GroupFingerprint]
	if !ok || prev.FlushLog.Timestamp.Before(e.FlushLog.Timestamp) {
		s[e.FlushLog.GroupFingerprint] = e
		return true
	}
	return false
}

func (s state) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	for _, e := range s {
		if _, err := pbutil.WriteDelimited(&buf, e); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func decodeState(r io.Reader) (state, error) {
	st := state{}
	for {
		var e pb.MeshFlushLog
		_, err := pbutil.ReadDelimited(r, &e)
		if err == nil {
			if e.FlushLog == nil || e.FlushLog.GroupFingerprint == 0 || e.FlushLog.Timestamp.IsZero() {
				return nil, ErrInvalidState
			}
			st[e.FlushLog.GroupFingerprint] = &e
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return nil, err
	}
	return st, nil
}

func marshalMeshFlushLog(e *pb.MeshFlushLog) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := pbutil.WriteDelimited(&buf, e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Options configures a new Log implementation.
type Options struct {
	SnapshotReader io.Reader
	SnapshotFile   string

	Retention time.Duration

	Logger  log.Logger
	Metrics prometheus.Registerer
}

func (o *Options) validate() error {
	if o.SnapshotFile != "" && o.SnapshotReader != nil {
		return errors.New("only one of SnapshotFile and SnapshotReader must be set")
	}

	return nil
}

// New creates a new flush log based on the provided options.
// The snapshot is loaded into the Log if it is set.
func New(o Options) (*FlushLog, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}

	l := &FlushLog{
		clock:              clock.New(),
		retention:          o.Retention,
		logger:             log.NewNopLogger(),
		st:                 state{},
		broadcast:          func([]byte) {},
		isReliableDelivery: clusterutil.OversizedMessage,
		metrics:            newMetrics(o.Metrics),
	}

	if o.Logger != nil {
		l.logger = o.Logger
	}

	if o.Retention != 0 {
		l.retention = o.Retention
	}

	if o.SnapshotFile != "" {
		if r, err := os.Open(o.SnapshotFile); err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
			level.Debug(l.logger).Log("msg", "flush log snapshot file doesn't exist", "err", err)
		} else {
			o.SnapshotReader = r
			defer r.Close()
		}
	}

	if o.SnapshotReader != nil {
		if err := l.loadSnapshot(o.SnapshotReader); err != nil {
			return l, err
		}
	}

	return l, nil
}

func (l *FlushLog) now() time.Time {
	return l.clock.Now()
}

// Maintenance garbage collects the flush log state at the given interval. If the snapshot
// file is set, a snapshot is written to it afterwards.
// Terminates on receiving from stopc.
// If not nil, the last argument is an override for what to do as part of the maintenance - for advanced usage.
func (l *FlushLog) Maintenance(interval time.Duration, snapf string, stopc <-chan struct{}, override MaintenanceFunc) {
	if interval == 0 || stopc == nil {
		level.Error(l.logger).Log("msg", "interval or stop signal are missing - not running maintenance")
		return
	}
	t := l.clock.Ticker(interval)
	defer t.Stop()

	var doMaintenance MaintenanceFunc
	doMaintenance = func() (int64, error) {
		var size int64
		if _, err := l.GC(); err != nil {
			return size, err
		}
		if snapf == "" {
			return size, nil
		}
		f, err := openReplace(snapf)
		if err != nil {
			return size, err
		}
		if size, err = l.Snapshot(f); err != nil {
			f.Close()
			return size, err
		}
		return size, f.Close()
	}

	if override != nil {
		doMaintenance = override
	}

	runMaintenance := func(do func() (int64, error)) error {
		l.metrics.maintenanceTotal.Inc()
		start := l.now().UTC()
		level.Debug(l.logger).Log("msg", "Running maintenance")
		size, err := do()
		l.metrics.snapshotSize.Set(float64(size))
		if err != nil {
			l.metrics.maintenanceErrorsTotal.Inc()
			return err
		}
		level.Debug(l.logger).Log("msg", "Maintenance done", "duration", l.now().Sub(start), "size", size)
		return nil
	}

Loop:
	for {
		select {
		case <-stopc:
			break Loop
		case <-t.C:
			if err := runMaintenance(doMaintenance); err != nil {
				level.Error(l.logger).Log("msg", "Running maintenance failed", "err", err)
			}
		}
	}

	// No need to run final maintenance if we don't want to snapshot.
	if snapf == "" {
		return
	}
	if err := runMaintenance(doMaintenance); err != nil {
		level.Error(l.logger).Log("msg", "Creating shutdown snapshot failed", "err", err)
	}
}

func (l *FlushLog) Log(groupFingerprint uint64, flushTime, expiryThreshold time.Time, expiry time.Duration) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	now := l.now()

	expiresAt := now.Add(l.retention)
	if expiry > 0 && expiry < l.retention {
		expiresAt = now.Add(expiry)
	}

	e := &pb.MeshFlushLog{
		FlushLog: &pb.FlushLog{
			GroupFingerprint: uint64(groupFingerprint),
			Timestamp:        flushTime,
		},
		ExpiresAt: expiresAt,
	}

	if prevle, ok := l.st[groupFingerprint]; ok && !prevle.ExpiresAt.IsZero() {
		// minimize gossip by logging once per expiry period
		// - expiry is based on the time given by the flush log clock and is set on the mesh struct.
		// - we don't have any of that here, so based on flush time (which is before the flushlog clock time)
		// the idea is to keep the logging frequency low but also ensure that entries that shouldn't expire don't
		// (on expire, the flushlog gets recreated but that introduces drift / de-syncs the flushes)
		closeToExpiry := expiryThreshold.After(prevle.ExpiresAt)

		// FlushLog already exists, only overwrite if timestamp is newer.
		// This may happen with raciness or clock-drift across AM nodes.
		if prevle.FlushLog.Timestamp.After(flushTime) || !closeToExpiry {
			return nil
		} else if closeToExpiry {
			e.FlushLog = prevle.FlushLog // keep previous timestamp
		}
	}
	// If prevle is a tombstone we intentionally fall through here: the new
	// flush must use the fresh flushTime so its Timestamp is strictly newer
	// than the tombstone's, allowing peers to accept it via state.merge's
	// entry-path. Carrying the tombstone's Timestamp would deadlock the
	// group on every peer until GC sweeps the tombstone.

	b, err := marshalMeshFlushLog(e)
	if err != nil {
		return err
	}
	l.st.merge(e, l.now())
	l.broadcast(b)

	return nil
}

// Delete marks the entry for the given group fingerprint as a tombstone and
// broadcasts it. The tombstone is retained in state (rather than being
// removed) so that stale refresh broadcasts arriving after the delete
// cannot resurrect the entry — see state.merge for the full rationale.
//
// The tombstone's FlushLog.Timestamp is bumped to now (or kept if already
// later — possible under clock skew across peers) so FlushLog.GC sweeps it
// at deletion_time + retention regardless of how long the underlying entry
// was alive. Without this, a long-firing alert whose FlushLog.Timestamp
// stays pinned to its original flush time would produce a tombstone whose
// Timestamp + retention is already in the past, and GC would sweep it on
// the next tick — letting in-flight refresh broadcasts resurrect the entry.
//
// The tombstone is built as a fresh MeshFlushLog (including a fresh inner
// FlushLog) rather than mutating the in-map pointer. Query returns the
// inner *pb.FlushLog by reference and releases the read lock before the
// caller dereferences fields like Timestamp; mutating the shared pointer
// would race those readers on time.Time's multi-word value.
func (l *FlushLog) Delete(groupFingerprint uint64) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	fl, ok := l.st[groupFingerprint]
	if !ok {
		return ErrNotFound
	}
	if fl.ExpiresAt.IsZero() {
		// Already tombstoned; nothing to do.
		return nil
	}

	now := l.now()
	ts := fl.FlushLog.Timestamp
	if ts.Before(now) {
		ts = now
	}
	tomb := &pb.MeshFlushLog{
		FlushLog: &pb.FlushLog{
			GroupFingerprint: fl.FlushLog.GroupFingerprint,
			Timestamp:        ts,
		},
		ExpiresAt: time.Time{},
	}
	l.st[groupFingerprint] = tomb

	b, err := marshalMeshFlushLog(tomb)
	if err != nil {
		return err
	}
	l.broadcast(b)

	return nil
}

// GC implements the Log interface. Live entries are swept when their
// ExpiresAt has passed. Tombstones (ExpiresAt zero) are swept once
// FlushLog.Timestamp + retention has passed — long enough for in-flight
// gossip refresh messages to drain so they can't resurrect a deleted entry.
func (l *FlushLog) GC() (int, error) {
	start := time.Now()
	defer func() { l.metrics.gcDuration.Observe(time.Since(start).Seconds()) }()

	now := l.now()
	var n int

	l.mtx.Lock()
	defer l.mtx.Unlock()

	for k, le := range l.st {
		if le.ExpiresAt.IsZero() {
			if le.FlushLog.Timestamp.Add(l.retention).Before(now) {
				delete(l.st, k)
				n++
			}
			continue
		}
		if !le.ExpiresAt.After(now) {
			delete(l.st, k)
			n++
		}
	}

	return n, nil
}

// Query implements the Log interface.
func (l *FlushLog) Query(groupFingerprint uint64) ([]*pb.FlushLog, error) {
	start := time.Now()
	l.metrics.queriesTotal.Inc()

	entries, err := func() ([]*pb.FlushLog, error) {
		// receiver/group_key combination.
		if groupFingerprint == 0 {
			return nil, errors.New("invalid group fingerprint")
		}

		l.mtx.RLock()
		defer l.mtx.RUnlock()

		if le, ok := l.st[groupFingerprint]; ok && !le.ExpiresAt.IsZero() {
			return []*pb.FlushLog{le.FlushLog}, nil
		}
		return nil, ErrNotFound
	}()
	if err != nil {
		l.metrics.queryErrorsTotal.Inc()
	}
	l.metrics.queryDuration.Observe(time.Since(start).Seconds())
	return entries, err
}

// loadSnapshot loads a snapshot generated by Snapshot() into the state.
func (l *FlushLog) loadSnapshot(r io.Reader) error {
	st, err := decodeState(r)
	if err != nil {
		return err
	}

	l.mtx.Lock()
	l.st = st
	l.mtx.Unlock()

	return nil
}

// Snapshot implements the Log interface.
func (l *FlushLog) Snapshot(w io.Writer) (int64, error) {
	start := time.Now()
	defer func() { l.metrics.snapshotDuration.Observe(time.Since(start).Seconds()) }()

	l.mtx.RLock()
	defer l.mtx.RUnlock()

	b, err := l.st.MarshalBinary()
	if err != nil {
		return 0, err
	}

	return io.Copy(w, bytes.NewReader(b))
}

// MarshalBinary serializes all contents of the flush log.
func (l *FlushLog) MarshalBinary() ([]byte, error) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	return l.st.MarshalBinary()
}

// Merge merges flush log state received from the cluster with the local state.
func (l *FlushLog) Merge(b []byte) error {
	st, err := decodeState(bytes.NewReader(b))
	if err != nil {
		return err
	}
	l.mtx.Lock()
	defer l.mtx.Unlock()
	now := l.now()

	for _, e := range st {
		if merged := l.st.merge(e, now); merged && !l.isReliableDelivery(b) {
			// If this is the first we've seen the message and it was
			// not sent reliably to all nodes, gossip it to other nodes.
			// We don't propagate reliable messages because they're
			// sent to all nodes already.
			l.broadcast(b)
			l.metrics.propagatedMessagesTotal.Inc()
			level.Debug(l.logger).Log("msg", "gossiping new entry", "entry", e)
		}
	}
	return nil
}

// SetBroadcast sets a broadcast callback that will be invoked with serialized state
// on updates.
func (l *FlushLog) SetBroadcast(f func([]byte)) {
	l.mtx.Lock()
	l.broadcast = f
	l.mtx.Unlock()
}

// SetIsReliableDelivery sets a callback that returns true if the given message
// was delivered reliably to all peers and should not be re-broadcast.
func (l *FlushLog) SetIsReliableDelivery(f func([]byte) bool) {
	l.mtx.Lock()
	l.isReliableDelivery = f
	l.mtx.Unlock()
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
