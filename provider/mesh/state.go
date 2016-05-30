package mesh

import (
	"bytes"
	"encoding/gob"
	"sync"
	"time"

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
