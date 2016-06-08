package mesh

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
	"time"

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
