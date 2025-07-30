// Copyright 2024 Prometheus Team
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

// Package slack provides storage for Slack message metadata, which can share its
// state over a mesh network and snapshot it.
package slack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/coder/quartz"
	uuid "github.com/gofrs/uuid"
	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/prometheus/client_golang/prometheus"

	pb "github.com/prometheus/alertmanager/notify/slack/slackpb"
)

// ErrNotFound is returned if a slack message was not found.
var ErrNotFound = errors.New("slack message not found")

// ErrInvalidState is returned if the state isn't valid.
var ErrInvalidState = errors.New("invalid state")

// SlackMessages holds slack message state that can be modified, queried, and snapshot.
type SlackMessages struct {
	clock quartz.Clock

	logger    *slog.Logger
	metrics   *metrics
	retention time.Duration

	mtx       sync.RWMutex
	st        state
	version   int // Increments whenever messages are added.
	broadcast func([]byte)
}

// state is the internal representation of the slack message state.
type state map[string]*pb.MeshSlackMessage

// Options contains the configuration for slack message storage.
type Options struct {
	SnapshotFile string
	Retention    time.Duration
	Logger       *slog.Logger
	Metrics      prometheus.Registerer
	Clock        quartz.Clock
}

type metrics struct {
	gcDuration              prometheus.Summary
	snapshotDuration        prometheus.Summary
	snapshotSize            prometheus.Gauge
	queriesTotal            prometheus.Counter
	queryErrorsTotal        prometheus.Counter
	queryDuration           prometheus.Histogram
	messagesActive          prometheus.GaugeFunc
	propagatedMessagesTotal prometheus.Counter
	maintenanceTotal        prometheus.Counter
	maintenanceErrorsTotal  prometheus.Counter
}

func newSlackMessageMetrics(r prometheus.Registerer, s *SlackMessages) *metrics {
	m := &metrics{}

	m.gcDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "alertmanager_slack_message_gc_duration_seconds",
		Help: "Duration of the last slack message garbage collection cycle.",
	})
	m.snapshotDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "alertmanager_slack_message_snapshot_duration_seconds",
		Help: "Duration of the last slack message snapshot.",
	})
	m.snapshotSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "alertmanager_slack_message_snapshot_size_bytes",
		Help: "Size of the last slack message snapshot in bytes.",
	})
	m.queriesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_slack_message_queries_total",
		Help: "How many slack message queries were received.",
	})
	m.queryErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_slack_message_query_errors_total",
		Help: "How many slack message received queries did not succeed.",
	})
	m.queryDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "alertmanager_slack_message_query_duration_seconds",
		Help: "Duration of slack message query evaluation.",
	})
	m.messagesActive = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "alertmanager_slack_messages_active",
			Help: "How many slack messages are currently active.",
		},
		func() float64 {
			return float64(s.CountState())
		},
	)
	m.propagatedMessagesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_slack_messages_propagated_total",
		Help: "Number of slack messages propagated to other instances.",
	})
	m.maintenanceTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_slack_message_maintenance_total",
		Help: "How many slack message maintenance cycles have been executed.",
	})
	m.maintenanceErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "alertmanager_slack_message_maintenance_errors_total",
		Help: "How many slack message maintenance cycles have failed.",
	})

	if r != nil {
		r.MustRegister(
			m.gcDuration, m.snapshotDuration, m.snapshotSize,
			m.queriesTotal, m.queryErrorsTotal, m.queryDuration,
			m.messagesActive, m.propagatedMessagesTotal,
			m.maintenanceTotal, m.maintenanceErrorsTotal,
		)
	}

	return m
}

// New creates a new slack message storage.
func NewSlackMessages(o Options) *SlackMessages {
	if o.Clock == nil {
		o.Clock = quartz.NewReal()
	}
	if o.Retention == 0 {
		o.Retention = 168 * time.Hour // 7 days default
	}

	s := &SlackMessages{
		clock:     o.Clock,
		logger:    o.Logger,
		retention: o.Retention,
		st:        state{},
		version:   0,
		broadcast: func([]byte) {},
	}

	s.metrics = newSlackMessageMetrics(o.Metrics, s)

	return s
}

// nowUTC returns the current UTC time.
func (s *SlackMessages) nowUTC() time.Time {
	return s.clock.Now().UTC()
}

// Maintenance runs the periodic maintenance for slack messages.
func (s *SlackMessages) Maintenance(interval time.Duration, snapf string, stopc <-chan struct{}, override MaintenanceFunc) {
	doMaintenance := func() (int64, error) {
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

	runMaintenance := func() error {
		s.metrics.maintenanceTotal.Inc()
		start := s.nowUTC()
		var size int64
		var err error
		if override != nil {
			size, err = override()
		} else {
			size, err = doMaintenance()
		}
		s.metrics.snapshotSize.Set(float64(size))
		if err != nil {
			s.metrics.maintenanceErrorsTotal.Inc()
			s.logger.Error("Running slack message maintenance failed", "err", err)
		} else {
			s.logger.Debug("Slack message maintenance completed", "duration", s.nowUTC().Sub(start), "size", size)
		}
		return err
	}

	if err := runMaintenance(); err != nil {
		s.logger.Error("Initial slack message maintenance failed", "err", err)
	}

	if interval == 0 {
		return
	}

	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		select {
		case <-stopc:
			return
		case <-tick.C:
			if err := runMaintenance(); err != nil {
				s.logger.Error("Periodic slack message maintenance failed", "err", err)
			}
		}
	}
}

// MaintenanceFunc represents the function to run as part of the periodic maintenance for slack messages.
// It returns the size of the snapshot taken or an error if it failed.
type MaintenanceFunc func() (int64, error)

// GC runs garbage collection and removes expired slack messages.
func (s *SlackMessages) GC() (int, error) {
	start := s.nowUTC()
	defer func() { s.metrics.gcDuration.Observe(s.nowUTC().Sub(start).Seconds()) }()

	now := s.nowUTC()
	var n int

	s.mtx.Lock()
	defer s.mtx.Unlock()

	for id, m := range s.st {
		if m.ExpiresAt.Before(now) {
			delete(s.st, id)
			n++
		}
	}

	s.logger.Debug("Slack message garbage collection completed", "deleted", n)
	return n, nil
}

// CountState returns the number of slack messages in the state.
func (s *SlackMessages) CountState() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return len(s.st)
}

// Version returns the current version of the slack message state.
func (s *SlackMessages) Version() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.version
}

// Set stores a slack message timestamp for a group key and channel.
func (s *SlackMessages) Set(groupKey, channel, ts string) error {
	now := s.nowUTC()

	uid, err := uuid.NewV4()
	if err != nil {
		return err
	}

	id := uid.String()

	msg := &pb.SlackMessage{
		GroupKey:  groupKey,
		Channel:   channel,
		Ts:        ts,
		CreatedAt: now,
		UpdatedAt: now,
	}

	meshMsg := &pb.MeshSlackMessage{
		Message:   msg,
		ExpiresAt: now.Add(s.retention),
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()

	// Check if we already have a message for this group key and channel
	for _, existingMsg := range s.st {
		if existingMsg.Message.GroupKey == groupKey && existingMsg.Message.Channel == channel {
			// Update existing message
			existingMsg.Message.Ts = ts
			existingMsg.Message.UpdatedAt = now
			existingMsg.ExpiresAt = now.Add(s.retention)

			b, err := existingMsg.Marshal()
			if err != nil {
				return err
			}
			s.broadcast(b)
			return nil
		}
	}

	// Create new message
	s.st[id] = meshMsg
	s.version++

	b, err := meshMsg.Marshal()
	if err != nil {
		return err
	}
	s.broadcast(b)

	return nil
}

// Get retrieves a slack message timestamp for a group key and channel.
func (s *SlackMessages) Get(groupKey, channel string) (string, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	for _, msg := range s.st {
		if msg.Message.GroupKey == groupKey && msg.Message.Channel == channel {
			return msg.Message.Ts, nil
		}
	}

	return "", ErrNotFound
}

// Snapshot writes the current state to a snapshot file.
func (s *SlackMessages) Snapshot(w io.Writer) (int64, error) {
	start := s.nowUTC()
	defer func() { s.metrics.snapshotDuration.Observe(s.nowUTC().Sub(start).Seconds()) }()

	s.mtx.RLock()
	defer s.mtx.RUnlock()

	var buf bytes.Buffer
	for _, msg := range s.st {
		if _, err := pbutil.WriteDelimited(&buf, msg); err != nil {
			return 0, err
		}
	}

	n, err := w.Write(buf.Bytes())
	if err != nil {
		return 0, err
	}

	return int64(n), nil
}

// LoadSnapshot loads state from a snapshot file.
func (s *SlackMessages) LoadSnapshot(r io.Reader) error {
	st := state{}

	for {
		var msg pb.MeshSlackMessage
		_, err := pbutil.ReadDelimited(r, &msg)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		// Skip expired messages
		if msg.ExpiresAt.Before(s.nowUTC()) {
			continue
		}

		id := fmt.Sprintf("%s:%s", msg.Message.GroupKey, msg.Message.Channel)
		st[id] = &msg
	}

	s.mtx.Lock()
	s.st = st
	s.mtx.Unlock()

	return nil
}

// MarshalBinary serializes the entire state for cluster synchronization.
func (s *SlackMessages) MarshalBinary() ([]byte, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	var buf bytes.Buffer
	for _, msg := range s.st {
		if _, err := pbutil.WriteDelimited(&buf, msg); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// Merge merges serialized state from another instance.
func (s *SlackMessages) Merge(b []byte) error {
	r := bytes.NewReader(b)
	now := s.nowUTC()

	s.mtx.Lock()
	defer s.mtx.Unlock()

	for {
		var msg pb.MeshSlackMessage
		_, err := pbutil.ReadDelimited(r, &msg)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		// Skip expired messages
		if msg.ExpiresAt.Before(now) {
			continue
		}

		id := fmt.Sprintf("%s:%s", msg.Message.GroupKey, msg.Message.Channel)

		// Use last-write-wins conflict resolution
		if existing, ok := s.st[id]; ok {
			if existing.Message.UpdatedAt.Before(msg.Message.UpdatedAt) {
				s.st[id] = &msg
			}
		} else {
			s.st[id] = &msg
		}
	}

	return nil
}

// SetBroadcast sets the broadcast function for cluster synchronization.
func (s *SlackMessages) SetBroadcast(f func([]byte)) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.broadcast = f
}

// replaceFile represents a file that is moved to replace another file on Close.
type replaceFile struct {
	*os.File
	filename string
}

func (f *replaceFile) Close() error {
	if err := f.File.Sync(); err != nil {
		return err
	}
	if err := f.File.Close(); err != nil {
		return err
	}
	return os.Rename(f.File.Name(), f.filename)
}

// openReplace opens a file that will replace the given file on Close.
func openReplace(filename string) (*replaceFile, error) {
	tmpFilename := fmt.Sprintf("%s.%x", filename, uint64(rand.Int63()))
	f, err := os.Create(tmpFilename)
	if err != nil {
		return nil, err
	}
	return &replaceFile{f, filename}, nil
}

