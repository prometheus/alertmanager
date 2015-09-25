package main

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

var DefaultRouteOpts = RouteOpts{
	GroupWait:      10 * time.Second,
	RepeatInterval: 10 * time.Second,
}

type Routes []*Route

func (rs Routes) Match(lset model.LabelSet) []*RouteOpts {
	fakeParent := &Route{
		Routes:    rs,
		RouteOpts: DefaultRouteOpts,
	}
	return fakeParent.Match(lset)
}

// A Route is a node that contains definitions of how to handle alerts.
type Route struct {
	// The configuration parameters for matches of this route.
	RouteOpts RouteOpts

	// Equality or regex matchers an alert has to fulfill to match
	// this route.
	Matchers types.Matchers

	// If true, an alert matches further routes on the same level.
	Continue bool

	// Children routes of this route.
	Routes Routes
}

func NewRoute(cr *config.Route) *Route {
	groupBy := map[model.LabelName]struct{}{}
	for _, ln := range cr.GroupBy {
		groupBy[ln] = struct{}{}
	}

	opts := RouteOpts{
		SendTo:      cr.SendTo,
		GroupBy:     groupBy,
		hasWait:     cr.GroupWait != nil,
		hasInterval: cr.RepeatInterval != nil,
	}
	if opts.hasWait {
		opts.GroupWait = time.Duration(*cr.GroupWait)
	}
	if opts.hasInterval {
		opts.RepeatInterval = time.Duration(*cr.RepeatInterval)
	}

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

	return &Route{
		RouteOpts: opts,
		Matchers:  matchers,
		Continue:  cr.Continue,
		Routes:    NewRoutes(cr.Routes),
	}
}

func NewRoutes(croutes []*config.Route) Routes {
	res := Routes{}
	for _, cr := range croutes {
		res = append(res, NewRoute(cr))
	}
	return res
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

		for _, ro := range matches {
			ro.populateDefault(&r.RouteOpts)
		}

		all = append(all, matches...)

		if matches != nil && !cr.Continue {
			break
		}
	}

	if len(all) == 0 {
		all = append(all, &r.RouteOpts)
	}

	return all
}

type RouteOpts struct {
	// The identifier of the associated notification configuration
	SendTo string

	// What labels to group alerts by for notifications.
	GroupBy map[model.LabelName]struct{}

	// How long to wait to group matching alerts before sending
	// a notificaiton
	GroupWait      time.Duration
	RepeatInterval time.Duration

	hasWait, hasInterval bool
}

func (ro *RouteOpts) String() string {
	var labels []model.LabelName
	for ln := range ro.GroupBy {
		labels = append(labels, ln)
	}
	return fmt.Sprintf("<RouteOpts send_to:%q group_by:%q group_wait:%q %q %q>", ro.SendTo, labels, ro.GroupWait)
}

func (ro *RouteOpts) populateDefault(parent *RouteOpts) {
	for ln := range parent.GroupBy {
		if _, ok := ro.GroupBy[ln]; !ok {
			ro.GroupBy[ln] = struct{}{}
		}
	}
	if ro.SendTo == "" {
		ro.SendTo = parent.SendTo
	}
	if !ro.hasWait {
		ro.GroupWait = parent.GroupWait
	}
	if !ro.hasInterval {
		ro.RepeatInterval = parent.RepeatInterval
	}
}
