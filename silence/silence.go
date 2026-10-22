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
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/prometheus/alertmanager/eventrecorder"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
	"github.com/prometheus/alertmanager/tracing"
)

var tracer = tracing.NewTracer("github.com/prometheus/alertmanager/silence")

// ErrNotFound is returned if a silence was not found.
var ErrNotFound = errors.New("silence not found")

// ErrInvalidState is returned if the state isn't valid.
var ErrInvalidState = errors.New("invalid state")

// Silences holds a silence state that can be modified, queried, and snapshot.
type Silences struct {
	logger    *slog.Logger
	metrics   *metrics
	retention time.Duration
	limits    Limits
	logging   bool

	mtx       sync.RWMutex
	st        state
	version   int // Increments whenever silences are added.
	broadcast func([]byte)
	mi        matcherIndex
	vi        versionIndex
	recorder  eventrecorder.Recorder
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

	Logging bool

	// A logger used by background processing.
	Logger  *slog.Logger
	Metrics prometheus.Registerer

	// EventRecorder records silence-related events to the event recorder.
	EventRecorder eventrecorder.Recorder
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
		mi:        make(matcherIndex, 512),
		vi:        make(versionIndex, 0, 512),
		logger:    promslog.NewNopLogger(),
		retention: o.Retention,
		limits:    o.Limits,
		logging:   o.Logging,
		broadcast: func([]byte) {},
		st:        state{},
		recorder:  o.EventRecorder,
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
	return time.Now().UTC()
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

func (s *Silences) setSilence(msil *pb.MeshSilence, now time.Time) (changed, added bool, err error) {
	b, err := marshalMeshSilence(msil)
	if err != nil {
		return false, false, err
	}
	changed, added = s.st.merge(msil, now)
	if added {
		s.indexSilence(msil.Silence)
		s.updateSizeMetrics()
	}
	if changed {
		s.broadcast(b)
	}
	return changed, added, nil
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
		if s.logging {
			s.logSilence("update silence", sil)
		}
		changed, _, err := s.setSilence(msil, now)
		if err != nil {
			return err
		}
		if changed {
			s.recorder.RecordEvent(ctx, func() eventrecorder.EventData {
				return eventrecorder.NewSilenceUpdatedEvent(sil)
			})
		}
		return nil
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

	if ok && StateFromProtoAt(prev, now) != SilenceStateExpired {
		// We cannot update the silence, expire the old one to leave a history of
		// the silence before modification.
		if err := s.expire(prev.Id, now); err != nil {
			return fmt.Errorf("expire previous silence: %w", err)
		}
	}

	if s.logging {
		s.logSilence("create silence", sil)
	}
	_, added, err := s.setSilence(msil, now)
	if err != nil {
		return err
	}
	if added {
		s.recorder.RecordEvent(ctx, func() eventrecorder.EventData {
			return eventrecorder.NewSilenceCreatedEvent(sil)
		})
	}
	return nil
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
	switch st := StateFromProtoAt(a, now); st {
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

	return s.expire(id, s.nowUTC())
}

// Expire the silence with the given ID immediately.
// It is idempotent, nil is returned if the silence already expired before it is GC'd.
// If the silence is not found an error is returned.
func (s *Silences) expire(id string, now time.Time) error {
	sil, ok := s.getSilence(id)
	if !ok {
		return ErrNotFound
	}
	sil = cloneSilence(sil)
	switch StateFromProtoAt(sil, now) {
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
	if s.logging {
		s.logSilence("expire silence", sil)
	}
	_, _, err := s.setSilence(s.toMeshSilence(sil), now)
	return err
}

// logging silence status changes.
//
// XXX: This rewrites some code present in cli/format/format.go,
// labelsMatcher() and cli/format/format_extended.go,
// FormatSilences(). Refactoring this seems a little out of scope for
// now.
func (s *Silences) logSilence(msg string, sil *pb.Silence) {
	var listMatchers []string
	matcherTypeOperator := map[string]string{
		"EQUAL":      "=",
		"REGEXP":     "=~",
		"NOT_EQUAL":  "!=",
		"NOT_REGEXP": "!~",
	}
	for _, ms := range sil.MatcherSets {
		for _, matcher := range ms.Matchers {
			m := matcher.Name + matcherTypeOperator[matcher.Type.String()] + matcher.Pattern
			listMatchers = append(listMatchers, m)
		}
	}
	strMatchers := strings.Join(listMatchers, `,`)

	s.logger.Info(
		msg,
		"Id", sil.Id,
		"CreatedBy", sil.CreatedBy,
		"Comment", sil.Comment,
		"StartsAt", sil.StartsAt.AsTime().Format(time.RFC3339),
		"EndsAt", sil.EndsAt.AsTime().Format(time.RFC3339),
		"Matchers", strMatchers,
	)
}
