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

package dispatch

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventrecorder"
)

var hexKeyRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

func mustRoute(t *testing.T, in string) *Route {
	t.Helper()
	cr := config.Route{}
	require.NoError(t, yaml.Unmarshal([]byte(in), &cr))
	return NewRoute(&cr, nil)
}

// The configuration from #3817: siblings that share matchers and therefore a
// path, differing only in receiver, group_by, or nothing that matters.
const siblingRoutes = `
receiver: default
routes:
- matchers: [foo=bar]
  receiver: test1
  continue: true
- matchers: [foo=bar]
  receiver: test2
  continue: true
- matchers: [foo=bar]
  receiver: test1
  group_by: [cluster]
  continue: true
- matchers: [foo=bar]
  receiver: test1
  mute_time_intervals: [weekends]
  continue: true
`

func TestRouteKeyDistinguishesReceivers(t *testing.T) {
	root := mustRoute(t, siblingRoutes)
	require.Len(t, root.Routes, 4)
	byReceiver, byReceiver2, byGroupBy, identical := root.Routes[0], root.Routes[1], root.Routes[2], root.Routes[3]

	// All four siblings share the same human readable path.
	for _, r := range root.Routes {
		require.Equal(t, `{}/{foo="bar"}`, r.Path())
		require.Regexp(t, hexKeyRE, r.Key())
	}
	require.Regexp(t, hexKeyRE, root.Key())
	require.Equal(t, "{}", root.Path())

	// The receiver distinguishes the keys.
	require.NotEqual(t, byReceiver.Key(), byReceiver2.Key(), "different receivers must have different keys")
	for _, r := range root.Routes {
		require.NotEqual(t, root.Key(), r.Key(), "a child never shares its parent's key")
	}

	// Siblings with the same matchers and receiver deliberately share a key:
	// they share notification state today and the time-of-day pattern (same
	// receiver, complementary active time intervals) relies on that. group_by
	// is not part of the route key; it shows up in the group key through the
	// group labels.
	require.Equal(t, byReceiver.Key(), identical.Key())
	require.Equal(t, byReceiver.Key(), byGroupBy.Key())
}

func TestRouteKeyStable(t *testing.T) {
	root := mustRoute(t, siblingRoutes)
	keys := map[string]string{}
	for _, r := range root.Routes {
		keys[r.RouteOpts.Receiver] = r.Key()
	}

	t.Run("reordering siblings", func(t *testing.T) {
		reordered := mustRoute(t, `
receiver: default
routes:
- matchers: [foo=bar]
  receiver: test1
  group_by: [cluster]
  continue: true
- matchers: [foo=bar]
  receiver: test2
  continue: true
- matchers: [foo=bar]
  receiver: test1
  continue: true
`)
		for _, r := range reordered.Routes {
			require.Equal(t, keys[r.RouteOpts.Receiver], r.Key())
		}
	})

	t.Run("unrelated options", func(t *testing.T) {
		edited := mustRoute(t, `
receiver: default
group_wait: 1m
repeat_interval: 12h
routes:
- matchers: [foo=bar]
  receiver: test1
  group_wait: 5s
  group_interval: 1m
  repeat_interval: 2h
  mute_time_intervals: [weekends]
  active_time_intervals: [weekdays]
  routes:
  - matchers: [bar=baz]
`)
		require.Equal(t, root.Key(), edited.Key(), "root key must not depend on timings")
		require.Equal(t, keys["test1"], edited.Routes[0].Key())
	})

	t.Run("ancestor receiver edit", func(t *testing.T) {
		const tree = `
receiver: default
routes:
- matchers: [foo=bar]
  receiver: %s
  routes:
  - matchers: [bar=baz]
    receiver: test2
  - matchers: [qux=quux]
`
		before := mustRoute(t, fmt.Sprintf(tree, "test1"))
		after := mustRoute(t, fmt.Sprintf(tree, "test3"))
		overriding, inheriting := 0, 1
		// The route whose receiver changed, and the child inheriting it, are
		// re-keyed; the child that overrides the receiver is not.
		require.NotEqual(t, before.Routes[0].Key(), after.Routes[0].Key())
		require.NotEqual(t, before.Routes[0].Routes[inheriting].Key(), after.Routes[0].Routes[inheriting].Key())
		require.Equal(t, before.Routes[0].Routes[overriding].Key(), after.Routes[0].Routes[overriding].Key())
		require.Equal(t, before.Key(), after.Key())
	})

	t.Run("golden", func(t *testing.T) {
		// Pin the exact values: keys are compared across Alertmanager
		// versions and peers, so any change to the hashed fields or their
		// encoding must be deliberate.
		require.Equal(t, "19ebb401e58a6be784f2217787af5bb19d61d7f45bb12400d28e513e99fd5582", root.Key())
		require.Equal(t, "96daf0306afc65e0443273eb72c896473720f6d782ba5ae38725a74a3fae9760", keys["test1"])
		require.Equal(t, "89e87e863e5dc8a95fa40d8ee3291f045c95d5c504834c9d0f78b2646267bbbe", keys["test2"])
	})
}

func TestKeyHashFieldBoundary(t *testing.T) {
	// The length prefix keeps the boundary between path and receiver
	// unambiguous; plain concatenation would make these collide.
	require.NotEqual(t, keyHash("ab", "c"), keyHash("a", "bc"))
	require.NotEqual(t, keyHash("", "ab"), keyHash("ab", ""))
}

func TestAggrGroupKeyDistinguishesReceivers(t *testing.T) {
	root := mustRoute(t, siblingRoutes)
	labels := model.LabelSet{"alertname": "HighLatency"}
	timeout := func(d time.Duration) time.Duration { return d }

	var groups []*aggrGroup
	for _, r := range root.Routes {
		ag := newAggrGroup(context.Background(), labels, r, timeout, eventrecorder.NopRecorder(), promslog.NewNopLogger(), nil)
		groups = append(groups, ag)
	}

	// Same labels on sibling routes: the path always collides, the key only
	// for siblings with the same receiver.
	for _, ag := range groups {
		require.Equal(t, `{}/{foo="bar"}:{alertname="HighLatency"}`, ag.GroupPath())
		require.Equal(t, ag.GroupPath(), ag.String())
		require.Regexp(t, hexKeyRE, ag.GroupKey())
	}
	require.NotEqual(t, groups[0].GroupKey(), groups[1].GroupKey())
	require.Equal(t, groups[0].GroupKey(), groups[2].GroupKey())
	require.Equal(t, groups[0].GroupKey(), groups[3].GroupKey())
}
