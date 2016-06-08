package mesh

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/log"
	"github.com/weaveworks/mesh"
)

func TestNotificationInfosOnGossip(t *testing.T) {
	var (
		t0 = time.Now()
		t1 = t0.Add(time.Minute)
	)
	cases := []struct {
		initial map[string]notificationEntry
		msg     map[string]notificationEntry
		delta   map[string]notificationEntry
		final   map[string]notificationEntry
	}{
		{
			initial: map[string]notificationEntry{},
			msg: map[string]notificationEntry{
				"123:recv1": {true, t0},
			},
			delta: map[string]notificationEntry{
				"123:recv1": {true, t0},
			},
			final: map[string]notificationEntry{
				"123:recv1": {true, t0},
			},
		}, {
			initial: map[string]notificationEntry{
				"123:recv1": {true, t0},
			},
			msg: map[string]notificationEntry{
				"123:recv1": {false, t1},
			},
			delta: map[string]notificationEntry{
				"123:recv1": {false, t1},
			},
			final: map[string]notificationEntry{
				"123:recv1": {false, t1},
			},
		}, {
			initial: map[string]notificationEntry{
				"123:recv1": {true, t1},
			},
			msg: map[string]notificationEntry{
				"123:recv1": {false, t0},
			},
			delta: map[string]notificationEntry{},
			final: map[string]notificationEntry{
				"123:recv1": {true, t1},
			},
		},
	}

	for _, c := range cases {
		ni := NewNotificationInfos(log.Base())

		ni.st.mergeComplete(c.initial)
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(c.msg); err != nil {
			t.Fatal(err)
		}
		// OnGossip expects the delta but an empty set to be replaced with nil.
		d, err := ni.OnGossip(buf.Bytes())
		if err != nil {
			t.Errorf("%v OnGossip %v: %s", c.initial, c.msg, err)
			continue
		}
		want := c.final
		if have := ni.st.set; !reflect.DeepEqual(want, have) {
			t.Errorf("%v OnGossip %v: want %v, have %v", c.initial, c.msg, want, have)
		}

		want = c.delta
		if len(c.delta) == 0 {
			want = nil
		}
		if d != nil {
			if have := d.(*notificationState).set; !reflect.DeepEqual(want, have) {
				t.Errorf("%v OnGossip %v: want %v, have %v", c.initial, c.msg, want, have)
			}
		} else if want != nil {
			t.Errorf("%v OnGossip %v: want nil", c.initial, c.msg)
		}
	}

	for _, c := range cases {
		ni := NewNotificationInfos(log.Base())

		ni.st.mergeComplete(c.initial)
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(c.msg); err != nil {
			t.Fatal(err)
		}

		// OnGossipBroadcast expects the provided delta as is.
		d, err := ni.OnGossipBroadcast(mesh.UnknownPeerName, buf.Bytes())
		if err != nil {
			t.Errorf("%v OnGossipBroadcast %v: %s", c.initial, c.msg, err)
			continue
		}
		want := c.final
		if have := ni.st.set; !reflect.DeepEqual(want, have) {
			t.Errorf("%v OnGossip %v: want %v, have %v", c.initial, c.msg, want, have)
		}

		want = c.delta
		if have := d.(*notificationState).set; !reflect.DeepEqual(want, have) {
			t.Errorf("%v OnGossipBroadcast %v: want %v, have %v", c.initial, c.msg, want, have)
		}
	}

	for _, c := range cases {
		ni := NewNotificationInfos(log.Base())

		ni.st.mergeComplete(c.initial)
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(c.msg); err != nil {
			t.Fatal(err)
		}
		// OnGossipUnicast always expects the full state back.
		err := ni.OnGossipUnicast(mesh.UnknownPeerName, buf.Bytes())
		if err != nil {
			t.Errorf("%v OnGossip %v: %s", c.initial, c.msg, err)
			continue
		}

		want := c.final
		if have := ni.st.set; !reflect.DeepEqual(want, have) {
			t.Errorf("%v OnGossip %v: want %v, have %v", c.initial, c.msg, want, have)
		}
	}
}

func TestNotificationInfosSet(t *testing.T) {
	var (
		t0 = time.Now()
		t1 = t0.Add(10 * time.Minute)
		// t2 = t0.Add(20 * time.Minute)
		// t3 = t0.Add(30 * time.Minute)
	)
	cases := []struct {
		initial map[string]notificationEntry
		input   []*types.NotifyInfo
		update  map[string]notificationEntry
		final   map[string]notificationEntry
	}{
		{
			initial: map[string]notificationEntry{},
			input: []*types.NotifyInfo{
				{
					Alert:     0x10,
					Receiver:  "recv1",
					Resolved:  false,
					Timestamp: t0,
				},
			},
			update: map[string]notificationEntry{
				"0000000000000010:recv1": {false, t0},
			},
			final: map[string]notificationEntry{
				"0000000000000010:recv1": {false, t0},
			},
		},
		{
			// In this testcase we the second input update is already state
			// respective to the current state. We currently do not prune it
			// from the update as it's not a common occurrence.
			// The update is okay to propagate but the final state must correctly
			// drop it.
			initial: map[string]notificationEntry{
				"0000000000000010:recv1": {false, t0},
				"0000000000000010:recv2": {false, t1},
			},
			input: []*types.NotifyInfo{
				{
					Alert:     0x10,
					Receiver:  "recv1",
					Resolved:  true,
					Timestamp: t1,
				},
				{
					Alert:     0x10,
					Receiver:  "recv2",
					Resolved:  true,
					Timestamp: t0,
				},
				{
					Alert:     0x20,
					Receiver:  "recv2",
					Resolved:  false,
					Timestamp: t0,
				},
			},
			update: map[string]notificationEntry{
				"0000000000000010:recv1": {true, t1},
				"0000000000000010:recv2": {true, t0},
				"0000000000000020:recv2": {false, t0},
			},
			final: map[string]notificationEntry{
				"0000000000000010:recv1": {true, t1},
				"0000000000000010:recv2": {false, t1},
				"0000000000000020:recv2": {false, t0},
			},
		},
	}

	for _, c := range cases {
		ni := NewNotificationInfos(log.Base())
		tg := &testGossip{}
		ni.Register(tg)
		ni.st = &notificationState{set: c.initial}

		if err := ni.Set(c.input...); err != nil {
			t.Errorf("Insert failed: %s", err)
			continue
		}
		// Verify the correct state afterwards.
		if have := ni.st.set; !reflect.DeepEqual(have, c.final) {
			t.Errorf("Wrong final state %v, expected %v", have, c.final)
			continue
		}

		// Verify that we gossiped the correct update.
		if have := tg.updates[0].(*notificationState).set; !reflect.DeepEqual(have, c.update) {
			t.Errorf("Wrong gossip update %v, expected %v", have, c.update)
			continue
		}
	}
}

// testGossip implements the mesh.Gossip interface. Received broadcast
// updates are appended to a list.
type testGossip struct {
	updates []mesh.GossipData
}

func (g *testGossip) GossipUnicast(dst mesh.PeerName, msg []byte) error {
	panic("not implemented")
}

func (g *testGossip) GossipBroadcast(update mesh.GossipData) {
	g.updates = append(g.updates, update)
}
