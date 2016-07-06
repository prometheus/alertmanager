package mesh

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/satori/go.uuid"
	"github.com/weaveworks/mesh"
)

func utcNow() time.Time { return time.Now().UTC() }

type notificationKey struct {
	Receiver string
	Alert    model.Fingerprint
}

type notificationEntry struct {
	Resolved  bool
	Timestamp time.Time
}

type notificationState struct {
	mtx sync.RWMutex
	set map[notificationKey]notificationEntry
	now func() time.Time // test injection hook
}

func newNotificationState() *notificationState {
	return &notificationState{
		set: map[notificationKey]notificationEntry{},
		now: utcNow,
	}
}

func decodeNotificationSet(b []byte) (map[notificationKey]notificationEntry, error) {
	var v map[notificationKey]notificationEntry
	err := gob.NewDecoder(bytes.NewReader(b)).Decode(&v)
	return v, err
}

func (st *notificationState) gc(retention time.Duration) {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	t := st.now().Add(-retention)
	for k, v := range st.set {
		if v.Timestamp.Before(t) {
			delete(st.set, k)
		}
	}
}

func (st *notificationState) snapshot(w io.Writer) error {
	st.mtx.RLock()
	defer st.mtx.RUnlock()

	enc := gob.NewEncoder(w)
	for k, n := range st.set {
		if err := enc.Encode(k); err != nil {
			return err
		}
		if err := enc.Encode(n); err != nil {
			return err
		}
	}
	return nil
}

func (st *notificationState) loadSnapshot(r io.Reader) error {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	dec := gob.NewDecoder(r)
	for {
		var k notificationKey
		var n notificationEntry
		if err := dec.Decode(&k); err != nil {
			// Only EOF at the start of new pair is correct.
			if err == io.EOF {
				break
			}
			return err
		}
		if err := dec.Decode(&n); err != nil {
			return err
		}
		// TODO(fabxc): remove serialization of alert/receiver into
		// string key.
		st.set[k] = n
	}
	return nil
}

// copy returns a deep copy of the notification state.
func (st *notificationState) copy() *notificationState {
	st.mtx.RLock()
	defer st.mtx.RUnlock()

	res := &notificationState{
		set: make(map[notificationKey]notificationEntry, len(st.set)),
	}
	for k, v := range st.set {
		res.set[k] = v
	}
	return res
}

// Encode the notification state into a single byte slices.
func (st *notificationState) Encode() [][]byte {
	st.mtx.RLock()
	defer st.mtx.RUnlock()

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&st.set); err != nil {
		panic(err)
	}
	return [][]byte{buf.Bytes()}
}

// Merge the notification set with gossip data and return a new notification
// state. The original state remains unchanged.
// The state is based in LWW manner using the timestamp.
func (st *notificationState) Merge(other mesh.GossipData) mesh.GossipData {
	o := other.(*notificationState)
	o.mtx.RLock()
	defer o.mtx.RUnlock()

	return st.mergeComplete(o.set)
}

func (st *notificationState) mergeComplete(set map[notificationKey]notificationEntry) *notificationState {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	for k, v := range set {
		if prev, ok := st.set[k]; !ok || prev.Timestamp.Before(v.Timestamp) {
			st.set[k] = v
		}
	}
	// XXX(fabxc): from what I understand we merge into the receiver and what
	// we return should be exactly that.
	// As all operations are locked, this should be fine.
	return st
}

func (st *notificationState) mergeDelta(set map[notificationKey]notificationEntry) *notificationState {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	d := map[notificationKey]notificationEntry{}

	for k, v := range set {
		if prev, ok := st.set[k]; !ok || prev.Timestamp.Before(v.Timestamp) {
			st.set[k] = v
			d[k] = v
		}
	}
	return &notificationState{set: d}
}

type silenceState struct {
	mtx sync.RWMutex
	m   map[uuid.UUID]*types.Silence
	now func() time.Time // now function for test injection
}

func newSilenceState() *silenceState {
	return &silenceState{
		m:   map[uuid.UUID]*types.Silence{},
		now: utcNow,
	}
}

func (st *silenceState) gc(retention time.Duration) {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	t := st.now().Add(-retention)
	for k, v := range st.m {
		if v.EndsAt.Before(t) {
			delete(st.m, k)
		}
	}
}

func (st *silenceState) snapshot(w io.Writer) error {
	st.mtx.RLock()
	defer st.mtx.RUnlock()

	enc := gob.NewEncoder(w)
	for _, s := range st.m {
		if err := enc.Encode(s); err != nil {
			return err
		}
	}
	return nil
}

func (st *silenceState) loadSnapshot(r io.Reader) error {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	dec := gob.NewDecoder(r)
	for {
		var s types.Silence
		if err := dec.Decode(&s); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if err := s.Init(); err != nil {
			return fmt.Errorf("iniializing silence failed: %s", err)
		}
		st.m[s.ID] = &s
	}
	return nil
}

func decodeSilenceSet(b []byte) (map[uuid.UUID]*types.Silence, error) {
	var v map[uuid.UUID]*types.Silence
	err := gob.NewDecoder(bytes.NewReader(b)).Decode(&v)
	return v, err
}

func (st *silenceState) Encode() [][]byte {
	st.mtx.RLock()
	defer st.mtx.RUnlock()

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&st.m); err != nil {
		panic(err)
	}
	return [][]byte{buf.Bytes()}
}

const timestampTolerance = time.Second

// silenceModAllowed checks whether silence a may be changed to silence b.
// Returns an error stating the reason if not.
// The silences are guaranteed to be valid. Silence a may be nil if b is a new.
func silenceModAllowed(a, b *types.Silence, now time.Time) error {
	if a == nil {
		if b.StartsAt.Before(now) {
			// From b being valid it follows that EndsAt will also be
			// in the future.
			return fmt.Errorf("new silence may not start in the past")
		}
		return nil
	}
	if a.ID != b.ID {
		return fmt.Errorf("IDs do not match")
	}

	almostEqual := func(s, t time.Time) bool {
		d := s.Sub(t)
		return d <= timestampTolerance && d >= -timestampTolerance
	}
	if almostEqual(a.StartsAt, b.StartsAt) {
		// Always pick original timestamp so we cannot drift the time
		// by spamming edits.
		b.StartsAt = a.StartsAt
	} else {
		if a.StartsAt.Before(now) {
			return fmt.Errorf("start time of active silence must not be modified")
		}
		if b.StartsAt.Before(now) {
			return fmt.Errorf("start time cannot be moved into the past")
		}
	}
	if almostEqual(a.EndsAt, b.EndsAt) {
		// Always pick original timestamp so we cannot drift the time
		// by spamming edits.
		b.EndsAt = a.EndsAt
	} else {
		if a.EndsAt.Before(now) {
			return fmt.Errorf("end time must not be modified for elapsed silence")
		}
		if b.EndsAt.Before(now) {
			return fmt.Errorf("end time must not be set into the past")
		}
	}

	if !a.Matchers.Equal(b.Matchers) {
		return fmt.Errorf("matchers must not be modified")
	}
	return nil
}

func (st *silenceState) set(s *types.Silence) error {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	s.StartsAt = s.StartsAt.UTC()
	s.EndsAt = s.EndsAt.UTC()

	now := st.now()
	s.UpdatedAt = now

	prev, ok := st.m[s.ID]
	// Silence start for new silences must not be before now.
	// Simplest solution is to reset it here if necessary.
	if !ok && s.StartsAt.Before(now) {
		s.StartsAt = now
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if err := silenceModAllowed(prev, s, now); err != nil {
		return err
	}
	st.m[s.ID] = s
	return nil
}

func (st *silenceState) del(id uuid.UUID) (*types.Silence, error) {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	prev, ok := st.m[id]
	if !ok {
		return nil, provider.ErrNotFound
	}
	now := st.now()

	// Silences are immutable by contract so we create a
	// shallow copy.
	sil := *prev
	sil.UpdatedAt = now

	// If silence hasn't started yet, terminate it at
	// its starting time.
	if sil.StartsAt.After(now) {
		sil.EndsAt = sil.StartsAt
	} else {
		sil.EndsAt = now
	}

	if err := sil.Validate(); err != nil {
		return nil, err
	}
	if err := silenceModAllowed(prev, &sil, now); err != nil {
		return nil, err
	}
	st.m[sil.ID] = &sil
	return &sil, nil
}

func (st *silenceState) Merge(other mesh.GossipData) mesh.GossipData {
	o := other.(*silenceState)
	o.mtx.RLock()
	defer o.mtx.RUnlock()

	return st.mergeComplete(o.m)
}

func (st *silenceState) mergeComplete(set map[uuid.UUID]*types.Silence) *silenceState {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	for k, v := range set {
		if prev, ok := st.m[k]; !ok || prev.UpdatedAt.Before(v.UpdatedAt) {
			st.m[k] = v
		}
	}
	return st
}

func (st *silenceState) mergeDelta(set map[uuid.UUID]*types.Silence) *silenceState {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	d := map[uuid.UUID]*types.Silence{}

	for k, v := range set {
		if prev, ok := st.m[k]; !ok || prev.UpdatedAt.Before(v.UpdatedAt) {
			st.m[k] = v
			d[k] = v
		}

	}
	return &silenceState{m: d}
}

func (st *silenceState) copy() *silenceState {
	st.mtx.RLock()
	defer st.mtx.RUnlock()

	res := &silenceState{
		m: make(map[uuid.UUID]*types.Silence, len(st.m)),
	}
	for k, v := range st.m {
		res.m[k] = v
	}
	return res
}
