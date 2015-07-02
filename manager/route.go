package manager

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"
)

type Routes []*Route

func (rs Routes) Match(lset model.LabelSet) []*RouteOpts {
	fakeParent := &Route{
		Routes: rs,
	}
	return fakeParent.Match(lset)
}

// A Route is a node that contains definitions of how to handle alerts.
type Route struct {
	// The configuration parameters for matches of this route.
	RouteOpts RouteOpts

	// Equality or regex matchers an alert has to fulfill to match
	// this route.
	Matchers Matchers

	// If true, an alert matches further routes on the same level.
	Continue bool

	// Children routes of this route.
	Routes Routes
}

// Match does a depth-first left-to-right search through the route tree
// and returns the flattened configuration for the reached node.
func (r *Route) Match(lset model.LabelSet) []*RouteOpts {
	if !r.Matchers.MatchAll(lset) {
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

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *Route) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type route struct {
		SendTo    string            `yaml:"send_to,omitempty"`
		GroupBy   []model.LabelName `yaml:"group_by,omitempty"`
		GroupWait *model.Duration   `yaml:"group_wait,omitempty"`

		Match    map[string]string `yaml:"match,omitempty"`
		MatchRE  map[string]string `yaml:"match_re,omitempty"`
		Continue bool              `yaml:"continue,omitempty"`
		Routes   []*Route          `yaml:"routes,omitempty"`

		// Catches all undefined fields and must be empty after parsing.
		XXX map[string]interface{} `yaml:",inline"`
	}
	var v route
	if err := unmarshal(&v); err != nil {
		return err
	}

	for k, val := range v.Match {
		if !model.LabelNameRE.MatchString(k) {
			fmt.Errorf("invalid label name %q", k)
		}
		ln := model.LabelName(k)
		r.Matchers = append(r.Matchers, NewMatcher(ln, val))
	}

	for k, val := range v.MatchRE {
		if !model.LabelNameRE.MatchString(k) {
			fmt.Errorf("invalid label name %q", k)
		}
		ln := model.LabelName(k)

		m, err := NewRegexMatcher(ln, val)
		if err != nil {
			return err
		}
		r.Matchers = append(r.Matchers, m)
	}

	r.RouteOpts.GroupBy = make(map[model.LabelName]struct{}, len(v.GroupBy))

	for _, ln := range v.GroupBy {
		if _, ok := r.RouteOpts.GroupBy[ln]; ok {
			return fmt.Errorf("duplicated label %q in group_by", ln)
		}
		r.RouteOpts.GroupBy[ln] = struct{}{}
	}

	r.RouteOpts.groupWait = (*time.Duration)(v.GroupWait)
	r.RouteOpts.SendTo = v.SendTo

	r.Continue = v.Continue
	r.Routes = v.Routes

	return checkOverflow(v.XXX, "route")
}

type RouteOpts struct {
	// The identifier of the associated notification configuration
	SendTo string

	// What labels to group alerts by for notifications.
	GroupBy map[model.LabelName]struct{}

	// How long to wait to group matching alerts before sending
	// a notificaiton
	groupWait *time.Duration
}

func (ro *RouteOpts) String() string {
	var labels []model.LabelName
	for ln := range ro.GroupBy {
		labels = append(labels, ln)
	}
	return fmt.Sprintf("<RouteOpts send_to:%q group_by:%q group_wait:%q>", ro.SendTo, labels, ro.groupWait)
}

func (ro *RouteOpts) GroupWait() time.Duration {
	if ro.groupWait == nil {
		return 0
	}
	return *ro.groupWait
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
	if ro.groupWait == nil {
		ro.groupWait = parent.groupWait
	}
}
