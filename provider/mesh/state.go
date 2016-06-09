package mesh

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/alertmanager/types"
	"github.com/satori/go.uuid"
	"github.com/weaveworks/mesh"
)

type notificationEntry struct {
	Resolved  bool
	Timestamp time.Time
}

type notificationState struct {
	mtx sync.RWMutex
	set map[string]notificationEntry
}

func newNotificationState() *notificationState {
	return &notificationState{
		set: map[string]notificationEntry{},
	}
}

func decodeNotificationSet(b []byte) (map[string]notificationEntry, error) {
	var v map[string]notificationEntry
	err := gob.NewDecoder(bytes.NewReader(b)).Decode(&v)
	return v, err
}

// copy returns a deep copy of the notification state.
func (s *notificationState) copy() *notificationState {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	res := &notificationState{
		set: make(map[string]notificationEntry, len(s.set)),
	}
	for k, v := range s.set {
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

func (st *notificationState) mergeComplete(set map[string]notificationEntry) *notificationState {
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

func (st *notificationState) mergeDelta(set map[string]notificationEntry) *notificationState {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	d := map[string]notificationEntry{}

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
		now: time.Now,
	}
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

// silenceModAllowed checks whether silence a may be changed to silence b.
// Returns an error stating the reason if not.
func (st *silenceState) silenceModAllowed(a, b *types.Silence) error {
	if !b.StartsAt.Equal(a.StartsAt) {
		return fmt.Errorf("silence start time must not be modified")
	}
	now := st.now()
	if a.EndsAt.Before(now) {
		return fmt.Errorf("end time must not be modified for elapsed silence")
	}
	if b.EndsAt.Before(now) {
		return fmt.Errorf("end time must not be in the past")
	}
	if !a.Matchers.Equal(b.Matchers) {
		return fmt.Errorf("matchers must not be modified")
	}
	return nil
}

func (st *silenceState) set(s *types.Silence) error {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	s.UpdatedAt = st.now()

	if err := s.Validate(); err != nil {
		return err
	}

	prev, ok := st.m[s.ID]
	if !ok {
		st.m[s.ID] = s
		return nil
	}
	if err := st.silenceModAllowed(prev, s); err != nil {
		return err
	}
	st.m[s.ID] = s
	return nil
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

func (s *silenceState) copy() *silenceState {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	res := &silenceState{
		m: make(map[uuid.UUID]*types.Silence, len(s.m)),
	}
	for k, v := range s.m {
		res.m[k] = v
	}
	return res
}
