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
	"bufio"
	"bytes"
	"errors"
	"io"
	"time"

	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/prometheus/alertmanager/cluster"
	pb "github.com/prometheus/alertmanager/silence/silencepb"
)

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
		b, err := marshalMeshSilence(e)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
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
	// The nil check is here because of rare cases where this function
	// is called on a nil silence. It's up to the caller to decide if it's a bug
	if sil != nil && len(sil.MatcherSets) > 0 {
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
