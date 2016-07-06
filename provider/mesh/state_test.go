package mesh

import (
	"bytes"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/provider"
	"github.com/prometheus/alertmanager/types"
	"github.com/satori/go.uuid"
)

func TestNotificationStateGC(t *testing.T) {
	now := utcNow()

	initial := map[notificationKey]notificationEntry{
		{"", 1}: {true, now},
		{"", 2}: {true, now.Add(30 * time.Minute)},
		{"", 3}: {true, now.Add(-30 * time.Minute)},
		{"", 4}: {true, now.Add(-60 * time.Minute)},
		{"", 5}: {true, now.Add(-61 * time.Minute)},
		{"", 6}: {true, now.Add(-100 * time.Hour)},
	}
	final := map[notificationKey]notificationEntry{
		{"", 1}: {true, now},
		{"", 2}: {true, now.Add(30 * time.Minute)},
		{"", 3}: {true, now.Add(-30 * time.Minute)},
		{"", 4}: {true, now.Add(-60 * time.Minute)},
	}

	st := newNotificationState()
	st.now = func() time.Time { return now }
	st.set = initial
	st.gc(time.Hour)

	if !reflect.DeepEqual(st.set, final) {
		t.Errorf("Unexpected state after GC")
		t.Errorf("%s", pretty.Compare(st.set, final))
	}
}

func TestNotificationStateSnapshot(t *testing.T) {
	now := utcNow()

	initial := map[notificationKey]notificationEntry{
		{"abc", 123}: {false, now.Add(30 * time.Minute)},

		{"xyz", 789}: {false, now},
	}

	st := newNotificationState()
	st.now = func() time.Time { return now }
	st.set = initial

	var buf bytes.Buffer

	if err := st.snapshot(&buf); err != nil {
		t.Fatalf("Snapshotting failed: %s", err)
	}

	st = newNotificationState()

	if err := st.loadSnapshot(&buf); err != nil {
		t.Fatalf("Loading snapshot failed: %s", err)
	}

	if !reflect.DeepEqual(st.set, initial) {
		t.Errorf("Loaded snapshot did not match")
		t.Errorf("%s", pretty.Compare(st.set, initial))
	}
}

func TestSilenceStateGC(t *testing.T) {
	var (
		now = utcNow()

		id1 = uuid.NewV4()
		id2 = uuid.NewV4()
		id3 = uuid.NewV4()
		id4 = uuid.NewV4()
		id5 = uuid.NewV4()
	)
	silence := func(id uuid.UUID, t time.Time) *types.Silence {
		return &types.Silence{
			ID:        id,
			Matchers:  types.NewMatchers(types.NewMatcher("a", "c")),
			StartsAt:  now.Add(-100 * time.Hour),
			EndsAt:    t,
			UpdatedAt: now,
			CreatedBy: "x",
			Comment:   "x",
		}
	}

	initial := map[uuid.UUID]*types.Silence{
		id1: silence(id1, now.Add(10*time.Minute)),
		id2: silence(id2, now),
		id3: silence(id3, now.Add(-10*time.Minute)),
		id4: silence(id4, now.Add(-1*time.Hour)),
		id5: silence(id5, now.Add(-2*time.Hour)),
	}
	final := map[uuid.UUID]*types.Silence{
		id1: silence(id1, now.Add(10*time.Minute)),
		id2: silence(id2, now),
		id3: silence(id3, now.Add(-10*time.Minute)),
		id4: silence(id4, now.Add(-1*time.Hour)),
	}

	st := newSilenceState()
	st.now = func() time.Time { return now }
	st.m = initial
	st.gc(time.Hour)

	if !reflect.DeepEqual(st.m, final) {
		t.Errorf("Unexpected state after GC")
		t.Errorf("%s", pretty.Compare(st.m, final))
	}
}

func TestSilenceStateSnapshot(t *testing.T) {
	var (
		now      = utcNow()
		id1      = uuid.NewV4()
		id2      = uuid.NewV4()
		matchers = types.NewMatchers(
			types.NewMatcher("a", "b"),
			types.NewRegexMatcher("label", regexp.MustCompile("abc[^a].+")),
		)
	)
	initial := map[uuid.UUID]*types.Silence{
		id1: &types.Silence{
			ID:        id1,
			Matchers:  matchers,
			StartsAt:  now.Add(time.Minute),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now,
			CreatedBy: "x",
			Comment:   "x",
		},
		id2: &types.Silence{
			ID:        id2,
			Matchers:  matchers,
			StartsAt:  now.Add(-time.Hour),
			EndsAt:    now.Add(time.Minute),
			UpdatedAt: now,
			CreatedBy: "creator X",
			Comment:   "comment comment comment",
		},
	}

	st := newSilenceState()
	st.now = func() time.Time { return now }
	st.m = initial

	var buf bytes.Buffer

	if err := st.snapshot(&buf); err != nil {
		t.Fatalf("Snapshotting failed: %s", err)
	}

	st = newSilenceState()

	if err := st.loadSnapshot(&buf); err != nil {
		t.Fatalf("Loading snapshot failed: %s", err)
	}

	if !reflect.DeepEqual(st.m, initial) {
		t.Errorf("Loaded snapshot did not match")
		t.Errorf("%s", pretty.Compare(st.m, initial))
	}
}

func TestSilenceStateSet(t *testing.T) {
	var (
		now      = utcNow()
		id1      = uuid.NewV4()
		matchers = types.NewMatchers(types.NewMatcher("a", "b"))
	)
	cases := []struct {
		initial map[uuid.UUID]*types.Silence
		final   map[uuid.UUID]*types.Silence
		input   *types.Silence
		err     string
	}{
		{
			initial: map[uuid.UUID]*types.Silence{},
			final:   map[uuid.UUID]*types.Silence{},
			// Provide an invalid silence (no matchers).
			input: &types.Silence{
				ID:        id1,
				StartsAt:  now,
				EndsAt:    now.Add(time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "matcher",
		}, {
			initial: map[uuid.UUID]*types.Silence{},
			final: map[uuid.UUID]*types.Silence{
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
			input: &types.Silence{
				ID:       id1,
				Matchers: matchers,
				// Different input timezones must be normalized to UTC.
				StartsAt:  now.Add(time.Minute).In(time.FixedZone("test", 100000)),
				EndsAt:    now.Add(time.Hour).In(time.FixedZone("test", 10000000)),
				CreatedBy: "x",
				Comment:   "x",
			},
		}, {
			initial: map[uuid.UUID]*types.Silence{},
			final: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:       id1,
					Matchers: matchers,
					// StartsAt should be reset to now if it's before
					// now for a new silence.
					StartsAt:  now,
					EndsAt:    now.Add(time.Hour),
					UpdatedAt: now,
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			input: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-time.Second),
				EndsAt:    now.Add(time.Hour),
				CreatedBy: "x",
				Comment:   "x",
			},
		}, {
			initial: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(-time.Hour),
					EndsAt:    now.Add(-time.Minute),
					UpdatedAt: now.Add(-10 * time.Minute),
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			final: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					StartsAt:  now.Add(-time.Hour),
					EndsAt:    now.Add(-time.Minute),
					UpdatedAt: now.Add(-10 * time.Minute),
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			// Do an invalid modification (silence already elapsed).
			input: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-time.Hour),
				EndsAt:    now.Add(time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "elapsed",
		}, {
			initial: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(-time.Minute),
					EndsAt:    now.Add(time.Hour),
					UpdatedAt: now.Add(-time.Minute),
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			final: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(-time.Minute),
					EndsAt:    now.Add(time.Minute),
					UpdatedAt: now,
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			// Do valid modification (alter end time).
			input: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-time.Minute),
				EndsAt:    now.Add(time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
		},
	}

	for i, c := range cases {
		t.Logf("Test case %d", i)
		s := newSilenceState()
		s.m = c.initial
		s.now = func() time.Time { return now }

		if err := s.set(c.input); err != nil {
			if len(c.err) > 0 {
				if strings.Contains(err.Error(), c.err) {
					continue
				}
				t.Errorf("Expected error containing %q, got %q", c.err, err)
				continue
			}
			t.Errorf("Setting failed: %s", err)
			continue
		}

		if !reflect.DeepEqual(s.m, c.final) {
			t.Errorf("Unexpected final state")
			t.Errorf("%s", pretty.Compare(s.m, c.final))
			continue
		}
	}
}

func TestSilenceStateDel(t *testing.T) {
	var (
		now      = utcNow()
		id1      = uuid.NewV4()
		matchers = types.NewMatchers(types.NewMatcher("a", "b"))
	)
	cases := []struct {
		initial map[uuid.UUID]*types.Silence
		final   map[uuid.UUID]*types.Silence
		input   uuid.UUID
		err     string
	}{
		{
			initial: map[uuid.UUID]*types.Silence{},
			final:   map[uuid.UUID]*types.Silence{},
			// Provide a non-existant ID.
			input: id1,
			err:   provider.ErrNotFound.Error(),
		}, {
			initial: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(time.Minute),
					EndsAt:    now.Add(2 * time.Minute),
					UpdatedAt: now.Add(-time.Minute),
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			// Deleting unstarted silence sets end timestamp to start time.
			final: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(time.Minute),
					EndsAt:    now.Add(time.Minute),
					UpdatedAt: now,
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			input: id1,
		},
		{
			initial: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(-time.Minute),
					EndsAt:    now.Add(time.Minute),
					UpdatedAt: now.Add(-time.Minute),
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			final: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(-time.Minute),
					EndsAt:    now,
					UpdatedAt: now,
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			input: id1,
		}, {
			// Attempt deleting an elapsed silence.
			initial: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(-10 * time.Minute),
					EndsAt:    now.Add(-5 * time.Minute),
					UpdatedAt: now.Add(-10 * time.Minute),
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			final: map[uuid.UUID]*types.Silence{
				id1: &types.Silence{
					ID:        id1,
					Matchers:  matchers,
					StartsAt:  now.Add(-10 * time.Minute),
					EndsAt:    now.Add(-5 * time.Minute),
					UpdatedAt: now.Add(-10 * time.Minute),
					CreatedBy: "x",
					Comment:   "x",
				},
			},
			input: id1,
			err:   "end time",
		},
	}

	for i, c := range cases {
		t.Logf("Test case %d", i)
		s := newSilenceState()
		s.m = c.initial
		s.now = func() time.Time { return now }

		sil, err := s.del(c.input)
		if err != nil {
			if len(c.err) > 0 {
				if strings.Contains(err.Error(), c.err) {
					continue
				}
				t.Errorf("Expected error containing %q, got %q", c.err, err)
				continue
			}
			t.Errorf("Setting failed: %s", err)
			continue
		}

		if !reflect.DeepEqual(s.m, c.final) {
			t.Errorf("Unexpected final state")
			t.Errorf("%s", pretty.Compare(s.m, c.final))
			continue
		}
		if !reflect.DeepEqual(sil, s.m[c.input]) {
			t.Errorf("Returned silence doesn't match stored silence")
		}
	}
}

func TestSilenceModAllowed(t *testing.T) {
	var (
		now      = utcNow()
		id1      = uuid.NewV4()
		matchers = types.NewMatchers(types.NewMatcher("a", "b"))
	)
	cases := []struct {
		a, b *types.Silence
		err  string
	}{
		{
			a: nil,
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(1 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "x",
				Comment:   "x",
			},
		},
		{
			// Modify silence comment and creator and set not-yet started
			// end time into future.
			a: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(100 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "y",
				Comment:   "y",
			},
		},
		{
			a: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(-5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(-5 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "y",
				Comment:   "y",
			},
		},
		{
			a: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(-5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			// Timestamp tolerance must be respected.
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-10*time.Minute + timestampTolerance),
				EndsAt:    now.Add(-5*time.Minute - timestampTolerance),
				UpdatedAt: now,
				CreatedBy: "x",
				Comment:   "x",
			},
		},
		{
			a: nil,
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "start in the past",
		},
		{
			a: &types.Silence{
				ID:        uuid.NewV4(),
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(-5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			b: &types.Silence{
				ID:        uuid.NewV4(),
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(-5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "IDs do not match",
		},
		{
			a: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-10 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(1 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "start time of active silence",
		},
		{
			a: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(1 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-1 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "start time cannot be moved into the past",
		},
		{
			a: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-5 * time.Minute),
				EndsAt:    now.Add(-1 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-5 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "end time must not be modified for elapsed silence",
		},
		{
			a: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-5 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			b: &types.Silence{
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(-5 * time.Minute),
				EndsAt:    now.Add(-1 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "end time must not be set into the past",
		},
		{
			a: &types.Silence{
				ID:        id1,
				Matchers:  types.NewMatchers(types.NewMatcher("a", "b")),
				StartsAt:  now.Add(-5 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now.Add(-10 * time.Minute),
				CreatedBy: "x",
				Comment:   "x",
			},
			b: &types.Silence{
				ID:        id1,
				Matchers:  types.NewMatchers(types.NewMatcher("a", "c")),
				StartsAt:  now.Add(-5 * time.Minute),
				EndsAt:    now.Add(5 * time.Minute),
				UpdatedAt: now,
				CreatedBy: "x",
				Comment:   "x",
			},
			err: "matchers must not be modified",
		},
	}
	for _, c := range cases {
		got := silenceModAllowed(c.a, c.b, now)
		if got == nil {
			if c.err != "" {
				t.Errorf("Expected error containing %q but got none", c.err)
			}
			continue
		}
		if c.err == "" {
			t.Errorf("Expected no error but got %q", got)
		} else if !strings.Contains(got.Error(), c.err) {
			t.Errorf("Expected error containing %q but got %q", c.err, got)
		}
	}
}
