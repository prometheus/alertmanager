package trigger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/prometheus/alertmanager/nflog"
	pb "github.com/prometheus/alertmanager/nflog/nflogpb"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/common/model"
)

// Need to implement cluster.State interface
// SetBroadcast fn

// Trigger lets alertmanagers trigger the processing of a pipeline, if
// inter-node processing for a single pipeline falls out of sync.
type Trigger struct {
	st          state
	now         func() time.Time
	subscribers map[model.Fingerprint]chan *pb.MeshEntry
	broadcast   func([]byte)

	sync.Mutex
}

func New() *Trigger {
	return &Trigger{
		st:          state{},
		now:         utcNow,
		subscribers: make(map[model.Fingerprint]chan *pb.MeshEntry),
		broadcast:   func(_ []byte) {},
	}
}

func utcNow() time.Time {
	return time.Now().UTC()
}

// MarshalBinary implements the cluster.State interface.
func (t *Trigger) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer

	t.Lock()
	defer t.Unlock()
	for _, e := range t.st {
		if _, err := pbutil.WriteDelimited(&buf, e); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// Merge implements the cluster.State interface.
func (t *Trigger) Merge(b []byte) error {
	st, err := decodeState(bytes.NewReader(b))
	if err != nil {
		return err
	}
	t.Lock()
	defer t.Unlock()

	for _, e := range st {
		// TODO: Is there a purpose in storing these? I think we just
		// want to send a message and then move on with our lives.
		// t.st.merge(e)
		fp, err := model.FingerprintFromString(string(e.Entry.GroupKey))
		if err != nil {
			return fmt.Errorf("failed to parse groupkey to fingerprint %v", err)
		}
		s, ok := t.subscribers[fp]
		if !ok {
			return fmt.Errorf("subscriber for %s does not exist", string(e.Entry.GroupKey))
		}
		s <- e
	}
	return nil
}

// Trigger sends a message to the other members in the mesh, and triggers any
// existing aggrGroup with group_by fingerprint==fp.
func (t *Trigger) Trigger(fp model.Fingerprint) error {
	now := t.now()

	t.Lock()
	defer t.Unlock()

	e := &pb.MeshEntry{
		Entry: &pb.Entry{
			Receiver:       nil,
			GroupKey:       []byte(fp.String()),
			Timestamp:      now,
			FiringAlerts:   nil,
			ResolvedAlerts: nil,
		},
		ExpiresAt: now,
	}

	b, err := marshalMeshEntry(e)
	if err != nil {
		return err
	}
	t.broadcast(b)

	return nil
}

func marshalMeshEntry(e *pb.MeshEntry) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := pbutil.WriteDelimited(&buf, e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SetBroadcast is used to broadcast state.
func (t *Trigger) SetBroadcast(f func([]byte)) {
	t.Lock()
	t.broadcast = f
	t.Unlock()
}

// Subscribe returns a channel indicating incoming triggers.
func (t *Trigger) Subscribe(fp model.Fingerprint) provider.TriggerIterator {
	var (
		ch          = make(chan *pb.MeshEntry)
		ctx, cancel = context.WithCancel(context.Background())
	)

	t.Lock()
	t.subscribers[fp] = ch
	t.Unlock()

	go func() {
		<-ctx.Done()

		t.Lock()
		delete(t.subscribers, fp)
		close(ch)
		t.Unlock()
	}()

	return &triggerIterator{
		ch:     ch,
		cancel: cancel,
	}
}

// triggerListener alerts subscribers of a particular labelset when a new
// message arrives.
type triggerIterator struct {
	ch     chan *pb.MeshEntry
	cancel context.CancelFunc
}

// Next implements the TriggerIterator interface.
func (t *triggerIterator) Next() <-chan *pb.MeshEntry {
	return t.ch
}

// Err implements the Iterator interface.
func (t *triggerIterator) Err() error {
	return nil
}

// Close implements the Iterator interface.
func (t *triggerIterator) Close() {
	t.cancel()
}

// String is the label fingerprint. Can probably make this "typesafe" later.
type state map[string]*pb.MeshEntry

func (s state) merge(e *pb.MeshEntry) {
	k := string(e.Entry.GroupKey)

	prev, ok := s[k]
	if !ok || prev.Entry.Timestamp.Before(e.Entry.Timestamp) {
		s[k] = e
	}
}

func decodeState(r io.Reader) (state, error) {
	t := state{}
	for {
		var e pb.MeshEntry
		_, err := pbutil.ReadDelimited(r, &e)
		if err == nil {
			if e.Entry == nil {
				return nil, nflog.ErrInvalidState
			}
			// Create own protobuf def, use fingerprint instead of groupkey
			t[string(e.Entry.GroupKey)] = &e
			continue
		}
		if err == io.EOF {
			break
		}
		return nil, err
	}
	return t, nil
}
