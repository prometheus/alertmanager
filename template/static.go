// Copyright 2026 Prometheus Team
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

package template

import (
	"text/template/parse"

	"github.com/prometheus/common/model"
)

// staticDataFields are the fields of Data whose value is fixed for the
// lifetime of an aggregation group: they depend on the route and the group
// labels only, never on the alerts in the group. Everything else in Data
// (Status, Alerts, CommonLabels, CommonAnnotations, NotificationReason) is
// derived from the alerts and is therefore dynamic.
var staticDataFields = map[string]bool{
	"Receiver":    true,
	"GroupLabels": true,
	"RouteLabels": true, // raw (unrendered) route label values
	"ExternalURL": true,
}

// pureTemplateFuncs are the template functions whose result depends only on
// their arguments. A pipeline through one of them is static when all of its
// arguments are. Functions that read the clock (now, since) are deliberately
// absent, as is routeLabels, which is handled by recursing into the referenced
// label, and call, which invokes an arbitrary function value.
//
// This is an allowlist on purpose: a function missing from it is treated as
// dynamic, which is always safe. TestPureTemplateFuncsCoverDefaultFuncs keeps
// it in sync with DefaultFuncs so that adding a function forces a decision.
var pureTemplateFuncs = map[string]bool{
	// text/template builtins.
	"and": true, "or": true, "not": true, "len": true, "index": true,
	"slice": true, "print": true, "printf": true, "println": true,
	"eq": true, "ne": true, "lt": true, "le": true, "gt": true, "ge": true,
	"html": true, "js": true, "urlquery": true,
	// DefaultFuncs.
	"toUpper": true, "toLower": true, "title": true, "trimSpace": true,
	"join": true, "match": true, "safeHtml": true, "safeUrl": true,
	"urlUnescape": true, "reReplaceAll": true, "stringSlice": true,
	"date": true, "tz": true, "humanizeDuration": true, "toDate": true,
	"mustToDate": true, "addDuration": true, "toJson": true,
	"base64encode": true, "base64decode": true, "list": true, "append": true,
	"dict": true,
}

// impureTemplateFuncs are the DefaultFuncs intentionally excluded from
// pureTemplateFuncs.
var impureTemplateFuncs = map[string]bool{
	"now":         true,
	"since":       true,
	"routeLabels": true,
}

// SplitRouteLabels partitions a route's labels by whether their rendered value
// can depend on the alerts of an aggregation group.
//
// A label is static when its template, parsed with this Template's functions,
// only produces text from constants, the per-group fields of Data (.Receiver,
// .GroupLabels, .RouteLabels, .ExternalURL) and their methods, pure template
// functions applied to static values, control structures over static
// pipelines, and routeLabels references to labels that are themselves static.
// Static labels render to the same value for the lifetime of a group and can
// be rendered once without any alert data. Every other label, including one
// that fails to parse, is dynamic. The analysis is conservative: whenever it
// cannot prove a construct static it is treated as dynamic.
func (t *Template) SplitRouteLabels(labels model.LabelSet) (static, dynamic model.LabelSet) {
	static = make(model.LabelSet, len(labels))
	dynamic = model.LabelSet{}
	c := &routeLabelClassifier{
		t:          t,
		raw:        labels,
		memo:       make(map[model.LabelName]bool, len(labels)),
		inProgress: map[model.LabelName]struct{}{},
	}
	for name, value := range labels {
		if c.isStatic(name) {
			static[name] = value
		} else {
			dynamic[name] = value
		}
	}
	return static, dynamic
}

// routeLabelClassifier classifies the labels of one route, memoizing results
// so a label cross-referenced via routeLabels is analyzed once, and tracking
// in-progress labels so a reference cycle terminates (as dynamic).
type routeLabelClassifier struct {
	t          *Template
	raw        model.LabelSet
	memo       map[model.LabelName]bool
	inProgress map[model.LabelName]struct{}
}

func (c *routeLabelClassifier) isStatic(name model.LabelName) bool {
	if v, ok := c.memo[name]; ok {
		return v
	}
	raw, ok := c.raw[name]
	if !ok {
		// An unknown label renders to the empty string.
		return true
	}
	if _, busy := c.inProgress[name]; busy {
		// A cycle renders to an error; be conservative.
		return false
	}
	c.inProgress[name] = struct{}{}
	v := c.textIsStatic(string(raw))
	delete(c.inProgress, name)
	c.memo[name] = v
	return v
}

// textIsStatic parses text exactly as execText would and walks the tree.
func (c *routeLabelClassifier) textIsStatic(text string) bool {
	if text == "" {
		return true
	}
	tmpl, err := c.t.text.Clone()
	if err != nil {
		return false
	}
	tmpl, err = tmpl.New("").Parse(text)
	if err != nil {
		return false
	}
	if tmpl.Tree == nil {
		return true
	}
	w := &staticWalker{c: c}
	return w.list(tmpl.Tree.Root, staticScope{})
}

// staticScope tracks, while walking a template tree, whether dot currently
// refers to a static value (inside a with or range over a static pipeline) and
// which declared variables hold static values.
type staticScope struct {
	dotStatic bool
	vars      map[string]bool
}

func (s staticScope) withDot(static bool) staticScope {
	return staticScope{dotStatic: static, vars: s.vars}
}

func (s staticScope) withVars(decl []*parse.VariableNode, static bool) staticScope {
	if len(decl) == 0 {
		return s
	}
	vars := make(map[string]bool, len(s.vars)+len(decl))
	for k, v := range s.vars {
		vars[k] = v
	}
	for _, d := range decl {
		vars[d.Ident[0]] = static
	}
	return staticScope{dotStatic: s.dotStatic, vars: vars}
}

type staticWalker struct {
	c *routeLabelClassifier
}

func (w *staticWalker) list(l *parse.ListNode, s staticScope) bool {
	if l == nil {
		return true
	}
	for _, n := range l.Nodes {
		// A variable declared by an action is visible to the nodes that follow
		// it in the same list. The action itself emits nothing, so a dynamic
		// declaration only taints uses of the variable.
		if a, ok := n.(*parse.ActionNode); ok && a.Pipe != nil && len(a.Pipe.Decl) > 0 {
			s = s.withVars(a.Pipe.Decl, w.pipe(a.Pipe, s))
			continue
		}
		if !w.node(n, s) {
			return false
		}
	}
	return true
}

func (w *staticWalker) node(n parse.Node, s staticScope) bool {
	switch n := n.(type) {
	case *parse.TextNode, *parse.CommentNode, *parse.BreakNode, *parse.ContinueNode:
		return true
	case *parse.ActionNode:
		return w.pipe(n.Pipe, s)
	case *parse.IfNode:
		static := w.pipe(n.Pipe, s)
		if !static {
			return false
		}
		inner := s.withVars(n.Pipe.Decl, true)
		return w.list(n.List, inner) && w.list(n.ElseList, inner)
	case *parse.WithNode:
		if !w.pipe(n.Pipe, s) {
			return false
		}
		// Inside the body dot is the (static) pipeline value; the else branch
		// keeps the enclosing dot.
		inner := s.withDot(true).withVars(n.Pipe.Decl, true)
		return w.list(n.List, inner) && w.list(n.ElseList, s)
	case *parse.RangeNode:
		if !w.pipe(n.Pipe, s) {
			return false
		}
		inner := s.withDot(true).withVars(n.Pipe.Decl, true)
		return w.list(n.List, inner) && w.list(n.ElseList, s)
	default:
		// TemplateNode and anything unexpected.
		return false
	}
}

func (w *staticWalker) pipe(p *parse.PipeNode, s staticScope) bool {
	if p == nil {
		return true
	}
	for _, c := range p.Cmds {
		if !w.cmd(c, s) {
			return false
		}
	}
	return true
}

func (w *staticWalker) cmd(c *parse.CommandNode, s staticScope) bool {
	if len(c.Args) == 0 {
		return false
	}
	args := c.Args[1:]
	switch f := c.Args[0].(type) {
	case *parse.IdentifierNode:
		if f.Ident == "routeLabels" {
			if len(args) != 1 {
				return false
			}
			str, ok := args[0].(*parse.StringNode)
			if !ok {
				return false
			}
			return w.c.isStatic(model.LabelName(str.Text))
		}
		if !pureTemplateFuncs[f.Ident] {
			return false
		}
	default:
		// A field, variable, or parenthesized pipeline, possibly a method
		// call with the remaining args as arguments.
		if !w.arg(c.Args[0], s) {
			return false
		}
	}
	for _, a := range args {
		if !w.arg(a, s) {
			return false
		}
	}
	return true
}

func (w *staticWalker) arg(n parse.Node, s staticScope) bool {
	switch n := n.(type) {
	case *parse.StringNode, *parse.NumberNode, *parse.BoolNode, *parse.NilNode:
		return true
	case *parse.DotNode:
		return s.dotStatic
	case *parse.FieldNode:
		// .A.B.C: methods and fields of a static value are static, so only the
		// root matters, and the root is dot.
		if s.dotStatic {
			return true
		}
		return len(n.Ident) > 0 && staticDataFields[n.Ident[0]]
	case *parse.VariableNode:
		if n.Ident[0] == "$" {
			// $ is always the root data, whatever dot currently is.
			return len(n.Ident) > 1 && staticDataFields[n.Ident[1]]
		}
		return s.vars[n.Ident[0]]
	case *parse.ChainNode:
		return w.arg(n.Node, s)
	case *parse.PipeNode:
		return w.pipe(n, s)
	case *parse.IdentifierNode:
		// A niladic function used as an argument, e.g. {{ printf "%v" now }}.
		return pureTemplateFuncs[n.Ident]
	default:
		return false
	}
}
