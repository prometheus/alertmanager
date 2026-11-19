// Copyright 2015 Prometheus Team
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
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/pkg/labels"
)

// DefaultRouteOpts are the defaulting routing options which apply
// to the root route of a routing tree.
var DefaultRouteOpts = RouteOpts{
	GroupWait:         30 * time.Second,
	GroupInterval:     5 * time.Minute,
	RepeatInterval:    4 * time.Hour,
	GroupBy:           map[model.LabelName]struct{}{},
	GroupByAll:        false,
	MuteTimeIntervals: []string{},
	Labels:            model.LabelSet{},
}

// A Route is a node that contains definitions of how to handle alerts.
type Route struct {
	parent *Route

	// The configuration parameters for matches of this route.
	RouteOpts RouteOpts

	// Matchers an alert has to fulfill to match
	// this route.
	Matchers labels.Matchers

	// If true, an alert matches further routes on the same level.
	Continue bool

	// Children routes of this route.
	Routes []*Route

	// Idx contains the index of this route in the config
	Idx int

	// slot holds the groups of this route in the dispatcher the route is
	// bound to, shared with the routes with the same key and equal options.
	slot *slot
}

// NewRoute returns a new route.
func NewRoute(cr *config.Route, parent *Route) *Route {
	counter := 0
	return newRoute(cr, parent, &counter)
}

func newRoute(cr *config.Route, parent *Route, counter *int) *Route {
	// Create default and overwrite with configured settings.
	opts := DefaultRouteOpts
	if parent != nil {
		opts = parent.RouteOpts
	}

	// Merge parent route labels and cr.Labels into opts.Labels. Always merge
	// into a fresh LabelSet so we never mutate the parent's map.
	if len(cr.Labels) != 0 {
		merged := model.LabelSet{}
		maps.Copy(merged, opts.Labels)
		maps.Copy(merged, cr.Labels)
		opts.Labels = merged
	}

	if cr.Receiver != "" {
		opts.Receiver = cr.Receiver
	}

	if cr.GroupBy != nil {
		opts.GroupBy = map[model.LabelName]struct{}{}
		for _, ln := range cr.GroupBy {
			opts.GroupBy[ln] = struct{}{}
		}
		opts.GroupByAll = false
	} else {
		if cr.GroupByAll {
			opts.GroupByAll = cr.GroupByAll
		}
	}

	if cr.GroupWait != nil {
		opts.GroupWait = time.Duration(*cr.GroupWait)
	}
	if cr.GroupInterval != nil {
		opts.GroupInterval = time.Duration(*cr.GroupInterval)
	}
	if cr.RepeatInterval != nil {
		opts.RepeatInterval = time.Duration(*cr.RepeatInterval)
	}

	matchers := cr.AllMatchers()

	opts.MuteTimeIntervals = cr.MuteTimeIntervals
	opts.ActiveTimeIntervals = cr.ActiveTimeIntervals

	route := &Route{
		parent:    parent,
		RouteOpts: opts,
		Matchers:  matchers,
		Continue:  cr.Continue,
	}

	// Create child routes first (they get lower indices)
	route.Routes = newRoutes(cr.Routes, route, counter)

	// Assign index to this route after all children have been indexed
	route.Idx = *counter
	*counter++

	return route
}

// newRoutes returns a slice of routes.
func newRoutes(croutes []*config.Route, parent *Route, counter *int) []*Route {
	res := []*Route{}
	for _, cr := range croutes {
		res = append(res, newRoute(cr, parent, counter))
	}
	return res
}

// Match does a depth-first left-to-right search through the route tree
// and returns the matching routing nodes.
func (r *Route) Match(lset model.LabelSet) []*Route {
	var all []*Route
	r.match(lset, func(m *Route) { all = append(all, m) })
	return all
}

// match calls visit for every route of the tree the label set is routed to,
// in Walk order, and reports whether r itself matched the label set.
func (r *Route) match(lset model.LabelSet, visit func(*Route)) bool {
	if !r.Matchers.Matches(lset) {
		return false
	}

	matched := false
	for _, cr := range r.Routes {
		if !cr.match(lset, visit) {
			continue
		}
		matched = true
		if !cr.Continue {
			break
		}
	}

	// If no child nodes were matches, the current node itself is a match.
	if !matched {
		visit(r)
	}
	return true
}

// Key returns a key for the route. It does not uniquely identify the route in general.
func (r *Route) Key() string {
	b := strings.Builder{}

	if r.parent != nil {
		b.WriteString(r.parent.Key())
		b.WriteRune('/')
	}
	b.WriteString(r.Matchers.String())
	return b.String()
}

// ID returns a unique identifier for the route.
func (r *Route) ID() string {
	b := strings.Builder{}

	if r.parent != nil {
		b.WriteString(r.parent.ID())
		b.WriteRune('/')
	}

	b.WriteString(r.Matchers.String())

	if r.parent != nil {
		for i := range r.parent.Routes {
			if r == r.parent.Routes[i] {
				b.WriteRune('/')
				b.WriteString(strconv.Itoa(i))
				break
			}
		}
	}

	return b.String()
}

// Walk traverses the route tree in depth-first order.
func (r *Route) Walk(visit func(*Route)) {
	visit(r)
	for i := range r.Routes {
		r.Routes[i].Walk(visit)
	}
}

// Warnings returns one warning per set of routes with the same key and
// receiver whose options differ (#3817): groups with the same labels in such
// routes share a group key and can suppress each other's notifications in the
// notification log.
func (r *Route) Warnings() []string {
	type routeKey struct {
		key, receiver string
	}
	var order []routeKey
	sets := map[routeKey][]*Route{}
	r.Walk(func(rt *Route) {
		rk := routeKey{rt.Key(), rt.RouteOpts.Receiver}
		if _, ok := sets[rk]; !ok {
			order = append(order, rk)
		}
		sets[rk] = append(sets[rk], rt)
	})

	var warnings []string
	for _, rk := range order {
		routes := sets[rk]
		if len(routes) < 2 {
			continue
		}
		differ := map[string]struct{}{}
		for _, rt := range routes[1:] {
			for _, name := range differingOptions(&routes[0].RouteOpts, &rt.RouteOpts) {
				differ[name] = struct{}{}
			}
		}
		if len(differ) == 0 {
			continue
		}
		names := slices.DeleteFunc(slices.Clone(optionNames), func(name string) bool {
			_, ok := differ[name]
			return !ok
		})
		ids := make([]string, 0, len(routes))
		for _, rt := range routes {
			ids = append(ids, rt.ID())
		}
		warnings = append(warnings, fmt.Sprintf(
			"routes %s and %s have the same matchers and receiver %q but differ in %s, so their alert groups have the same group key and share notification log entries: a notification sent for one of them can suppress the same notification for the other. Merge the routes or use distinct receivers",
			strings.Join(ids[:len(ids)-1], ", "), ids[len(ids)-1], rk.receiver, strings.Join(names, ", "),
		))
	}
	return warnings
}

var optionNames = []string{"group_by", "group_wait", "group_interval", "repeat_interval", "mute_time_intervals", "active_time_intervals", "labels"}

// differingOptions returns the configuration names of the routing options,
// other than the receiver, in which a and b differ.
func differingOptions(a, b *RouteOpts) []string {
	var names []string
	if a.GroupByAll != b.GroupByAll || (!a.GroupByAll && !maps.Equal(a.GroupBy, b.GroupBy)) {
		names = append(names, "group_by")
	}
	if a.GroupWait != b.GroupWait {
		names = append(names, "group_wait")
	}
	if a.GroupInterval != b.GroupInterval {
		names = append(names, "group_interval")
	}
	if a.RepeatInterval != b.RepeatInterval {
		names = append(names, "repeat_interval")
	}
	if !sameNames(a.MuteTimeIntervals, b.MuteTimeIntervals) {
		names = append(names, "mute_time_intervals")
	}
	if !sameNames(a.ActiveTimeIntervals, b.ActiveTimeIntervals) {
		names = append(names, "active_time_intervals")
	}
	if !maps.Equal(a.Labels, b.Labels) {
		names = append(names, "labels")
	}
	return names
}

// RouteOpts holds various routing options necessary for processing alerts
// that match a given route.
type RouteOpts struct {
	// The identifier of the associated notification configuration.
	Receiver string

	// What labels to group alerts by for notifications.
	GroupBy map[model.LabelName]struct{}

	// Use all alert labels to group.
	GroupByAll bool

	// How long to wait to group matching alerts before sending
	// a notification.
	GroupWait      time.Duration
	GroupInterval  time.Duration
	RepeatInterval time.Duration

	// A list of time intervals for which the route is muted.
	MuteTimeIntervals []string

	// A list of time intervals for which the route is active.
	ActiveTimeIntervals []string

	// Merged labels from this route and all of its parent routes.
	Labels model.LabelSet
}

// equal reports whether ro and p are the same routing options: routes with
// the same key and equal options produce the same groups and notify them the
// same way.
func (ro *RouteOpts) equal(p *RouteOpts) bool {
	return ro.Receiver == p.Receiver && len(differingOptions(ro, p)) == 0
}

// sameNames reports whether a and b hold the same names, in any order.
func sameNames(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

func (ro *RouteOpts) String() string {
	var labels []model.LabelName
	for ln := range ro.GroupBy {
		labels = append(labels, ln)
	}
	return fmt.Sprintf("<RouteOpts send_to:%q group_by:%q group_by_all:%t timers:%q|%q>",
		ro.Receiver, labels, ro.GroupByAll, ro.GroupWait, ro.GroupInterval)
}

// MarshalJSON returns a JSON representation of the routing options.
func (ro *RouteOpts) MarshalJSON() ([]byte, error) {
	v := struct {
		Receiver       string           `json:"receiver"`
		GroupBy        model.LabelNames `json:"groupBy"`
		GroupByAll     bool             `json:"groupByAll"`
		GroupWait      time.Duration    `json:"groupWait"`
		GroupInterval  time.Duration    `json:"groupInterval"`
		RepeatInterval time.Duration    `json:"repeatInterval"`
	}{
		Receiver:       ro.Receiver,
		GroupByAll:     ro.GroupByAll,
		GroupWait:      ro.GroupWait,
		GroupInterval:  ro.GroupInterval,
		RepeatInterval: ro.RepeatInterval,
	}
	for ln := range ro.GroupBy {
		v.GroupBy = append(v.GroupBy, ln)
	}

	return json.Marshal(&v)
}
