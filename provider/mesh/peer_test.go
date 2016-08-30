package mesh

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/log"
	"github.com/satori/go.uuid"
	"github.com/weaveworks/mesh"
)

func TestReplaceFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "replace_file")
	if err != nil {
		t.Fatal(err)
	}
	origFilename := filepath.Join(dir, "testfile")

	of, err := os.Create(origFilename)
	if err != nil {
		t.Fatal(err)
	}

	nf, err := openReplace(filepath.Join(dir, "testfile"))
	if err != nil {
		t.Fatalf("Creating test file failed: %s", err)
	}
	if _, err := nf.Write([]byte("test")); err != nil {
		t.Fatalf("Writing replace file failed: %s", err)
	}

	if nf.Name() == of.Name() {
		t.Fatalf("Replacement file must not have same name as original")
	}
	if err := nf.Close(); err != nil {
		t.Fatalf("Closing replace file failed: %s", err)
	}
	of.Close()

	ofr, err := os.Open(origFilename)
	if err != nil {
		t.Fatal(err)
	}
	defer ofr.Close()

	res, err := ioutil.ReadAll(ofr)
	if err != nil {
		t.Fatal(err)
	}
	if string(res) != "test" {
		t.Fatalf("File contents do not match; got %q, expected %q", string(res), "test")
	}
}

func TestSilencesSet(t *testing.T) {
	var (
		now      = utcNow()
		id1      = uuid.NewV4()
		matchers = types.NewMatchers(types.NewMatcher("a", "b"))
	)
	cases := []struct {
		input  *types.Silence
		update map[uuid.UUID]*types.Silence
		fail   bool
	}{
		{
			// Set an invalid silence.
			input: &types.Silence{},
			fail:  true,
		},
		{
			// Set a silence including ID.
			input: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(time.Minute),
				EndsAt:    now.Add(time.Hour),
				CreatedBy: "x",
				Comment:   "x",
			},
			update: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(time.Minute),
					EndsAt:    now.Add(time.Hour),
					UpdatedAt: now,
					CreatedBy: "x",
					Comment:   "x",
				},
			},
		},
	}
	for i, c := range cases {
		t.Logf("Test case %d", i)

		s, err := NewSilences(nil, log.Base(), time.Hour, "")
		if err != nil {
			t.Fatal(err)
		}
		tg := &testGossip{}
		s.Register(tg)
		s.st.now = func() time.Time { return now }

		beforeID := c.input.ID

		uid, err := s.Set(c.input)
		if err != nil {
			if c.fail {
				continue
			}
			t.Errorf("Unexpected error: %s", err)
			continue
		}
		if c.fail {
			t.Errorf("Expected error but got none")
			continue
		}

		if beforeID != uuid.Nil && uid != beforeID {
			t.Errorf("Silence ID unexpectedly changed: before %q, after %q", beforeID, uid)
			continue
		}

		// Verify the update propagated.
		if have := tg.updates[0].(*silenceState).m; !reflect.DeepEqual(have, c.update) {
			t.Errorf("Update did not match")
			t.Errorf("%s", pretty.Compare(have, c.update))
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
