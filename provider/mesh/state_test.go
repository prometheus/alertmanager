package mesh

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/prometheus/alertmanager/types"
	"github.com/satori/go.uuid"
)

func TestSilenceStateSet(t *testing.T) {
	var (
		now      = time.Now()
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
				ID:        id1,
				Matchers:  matchers,
				StartsAt:  now.Add(time.Minute),
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
