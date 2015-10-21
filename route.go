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

package main

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

var DefaultRouteOpts = RouteOpts{
	GroupWait:      30 * time.Second,
	GroupInterval:  5 * time.Minute,
	RepeatInterval: 4 * time.Hour,
	SendResolved:   true,
	GroupBy: map[model.LabelName]struct{}{
		model.AlertNameLabel: struct{}{},
	},
}

// A Route is a node that contains definitions of how to handle alerts.
type Route struct {
	parent *Route

	// The configuration parameters for matches of this route.
	RouteOpts RouteOpts

	// Equality or regex matchers an alert has to fulfill to match
	// this route.
	Matchers types.Matchers

	// If true, an alert matches further routes on the same level.
	Continue bool

	// Children routes of this route.
	Routes []*Route
}

func NewRoute(cr *config.Route, parent *Route) *Route {
	// Create default and overwrite with configured settings.
	opts := DefaultRouteOpts
	if parent != nil {
		opts = parent.RouteOpts
	}

	if cr.SendTo != "" {
		opts.SendTo = cr.SendTo
	}
	if cr.GroupBy != nil {
		opts.GroupBy = map[model.LabelName]struct{}{}
		for _, ln := range cr.GroupBy {
			opts.GroupBy[ln] = struct{}{}
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
	if cr.SendResolved != nil {
		opts.SendResolved = *cr.SendResolved
	}

	// Build matchers.
	var matchers types.Matchers

	for ln, lv := range cr.Match {
		matchers = append(matchers, types.NewMatcher(model.LabelName(ln), lv))
	}
	for ln, lv := range cr.MatchRE {
		m, err := types.NewRegexMatcher(model.LabelName(ln), lv.String())
		if err != nil {
			// Must have been sanitized during config validation.
			panic(err)
		}
		matchers = append(matchers, m)
	}

	route := &Route{
		parent:    parent,
		RouteOpts: opts,
		Matchers:  matchers,
		Continue:  cr.Continue,
	}

	route.Routes = NewRoutes(cr.Routes, route)

	return route
}

func NewRoutes(croutes []*config.Route, parent *Route) []*Route {
	res := []*Route{}
	for _, cr := range croutes {
		res = append(res, NewRoute(cr, parent))
	}
	return res
}

func (r *Route) UIRoute() *UIRoute {
	var subs []*UIRoute
	for _, sr := range r.Routes {
		subs = append(subs, sr.UIRoute())
	}

	uir := &UIRoute{
		RouteOpts: &r.RouteOpts,
		Matchers:  r.Matchers,
		Routes:    subs,
	}
	return uir
}

// Match does a depth-first left-to-right search through the route tree
// and returns the flattened configuration for the reached node.
func (r *Route) Match(lset model.LabelSet) []*RouteOpts {
	if !r.Matchers.Match(lset) {
		return nil
	}

	var all []*RouteOpts

	for _, cr := range r.Routes {
		matches := cr.Match(lset)

		all = append(all, matches...)

		if matches != nil && !cr.Continue {
			break
		}
	}

	// If no child nodes were matches, the current node itself is
	// a match.
	if len(all) == 0 {
		all = append(all, &r.RouteOpts)
	}

	return all
}

func (r *Route) SquashMatchers() types.Matchers {
	var res types.Matchers
	res = append(res, r.Matchers...)

	if r.parent == nil {
		return res
	}

	pm := r.parent.SquashMatchers()
	res = append(pm, res...)

	return res
}

func (r *Route) Fingerprint() model.Fingerprint {
	lset := make(model.LabelSet, len(r.RouteOpts.GroupBy))

	for ln := range r.RouteOpts.GroupBy {
		lset[ln] = ""
	}

	return r.SquashMatchers().Fingerprint() ^ lset.Fingerprint()
}

type RouteOpts struct {
	// The identifier of the associated notification configuration
	SendTo       string
	SendResolved bool

	// What labels to group alerts by for notifications.
	GroupBy map[model.LabelName]struct{}

	// How long to wait to group matching alerts before sending
	// a notificaiton
	GroupWait      time.Duration
	GroupInterval  time.Duration
	RepeatInterval time.Duration
}

func (ro *RouteOpts) String() string {
	var labels []model.LabelName
	for ln := range ro.GroupBy {
		labels = append(labels, ln)
	}
	return fmt.Sprintf("<RouteOpts send_to:%q group_by:%q timers:%q|%q>", ro.SendTo, labels, ro.GroupWait, ro.GroupInterval)
}
