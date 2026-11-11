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

package tracing

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTracingAttributeConventions keeps attribute and option construction off
// the disabled tracing path: helpers in this package guard it behind
// IsRecording, and code outside this package only records on spans through
// those helpers. Runtime allocation tests cover the actual callers.
func TestTracingAttributeConventions(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "go.mod"))

	var violations []string
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			switch d.Name() {
			case ".git", "ui", "node_modules", "testdata", "vendor":
				return filepath.SkipDir
			}
			if d.Name() != "" && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			// Nested modules are not part of this build.
			if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		found := tracingAttributeViolations(f)
		if !strings.HasPrefix(rel, "tracing"+string(filepath.Separator)) {
			found = append(found, spanRecordingViolations(f)...)
		}
		for _, violation := range found {
			pos := fset.Position(violation.pos)
			violations = append(violations, fmt.Sprintf("%s:%d: %s", rel, pos.Line, violation.message))
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, violations)
}

type tracingAttributeViolation struct {
	pos     token.Pos
	message string
}

func tracingAttributeViolations(f *ast.File) []tracingAttributeViolation {
	traceImports := importNames(f, "go.opentelemetry.io/otel/trace", "trace")
	helperImports := importNames(f, "github.com/prometheus/alertmanager/tracing", "tracing")

	var violations []tracingAttributeViolation
	var parents []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			parents = parents[:len(parents)-1]
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				needsGuard := sel.Sel.Name == "SetAttributes" ||
					((sel.Sel.Name == "AddEvent" || sel.Sel.Name == "RecordError") && len(call.Args) > 1)
				if needsGuard && !isPackageCall(sel, helperImports) && !hasRecordingGuard(call, sel.X, parents) {
					violations = append(violations, tracingAttributeViolation{call.Pos(), sel.Sel.Name + " must be inside a recording guard for the same span"})
				}
			}
		}
		switch traceFunction(n, traceImports) {
		case "WithAttributes":
			// Restrict the constructor itself, including references that could
			// alias it. This catches renamed tracers, option variables and slices
			// without trying to follow the options through arbitrary Go code.
			if !isDirectEventOption(n, parents) {
				violations = append(violations, tracingAttributeViolation{n.Pos(), "construct trace.WithAttributes inline in AddEvent or RecordError, never in Start options"})
			}
		case "WithSpanKind":
			for _, parent := range parents {
				switch parent.(type) {
				case *ast.FuncDecl, *ast.FuncLit:
					violations = append(violations, tracingAttributeViolation{n.Pos(), "construct span kind options at package scope and reuse the full slice"})
					parents = append(parents, n)
					return true
				}
			}
		}
		parents = append(parents, n)
		return true
	})
	return violations
}

// spanRecordingViolations flags code outside this package that records on a
// span directly instead of through the helpers in events.go, and imports of
// the attribute and status code packages those helpers exist to encapsulate.
func spanRecordingViolations(f *ast.File) []tracingAttributeViolation {
	var violations []tracingAttributeViolation
	helperImports := importNames(f, "github.com/prometheus/alertmanager/tracing", "tracing")
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		switch path {
		case "go.opentelemetry.io/otel/attribute", "go.opentelemetry.io/otel/codes":
			violations = append(violations, tracingAttributeViolation{imp.Pos(), "import " + path + " only in the tracing package"})
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "SetAttributes", "AddEvent", "RecordError", "SetStatus":
		default:
			return true
		}
		if isPackageCall(sel, helperImports) {
			return true
		}
		violations = append(violations, tracingAttributeViolation{call.Pos(), "record on spans through the tracing package helpers, not " + sel.Sel.Name})
		return true
	})
	return violations
}

// importNames returns the identifiers under which the file imports path.
func importNames(f *ast.File, path, defaultName string) map[string]bool {
	names := make(map[string]bool)
	for _, imp := range f.Imports {
		if imported, err := strconv.Unquote(imp.Path.Value); err == nil && imported == path {
			name := defaultName
			if imp.Name != nil {
				name = imp.Name.Name
			}
			names[name] = true
		}
	}
	return names
}

// isPackageCall reports whether sel selects a function from one of the
// named package imports rather than a method on a value.
func isPackageCall(sel *ast.SelectorExpr, packages map[string]bool) bool {
	id, ok := sel.X.(*ast.Ident)
	return ok && packages[id.Name]
}

func traceFunction(n ast.Node, imports map[string]bool) string {
	switch n := n.(type) {
	case *ast.SelectorExpr:
		if id, ok := n.X.(*ast.Ident); ok && imports[id.Name] {
			return n.Sel.Name
		}
	case *ast.Ident:
		if imports["."] {
			return n.Name
		}
	}
	return ""
}

func isDirectEventOption(n ast.Node, parents []ast.Node) bool {
	if len(parents) < 2 {
		return false
	}
	option, ok := parents[len(parents)-1].(*ast.CallExpr)
	if !ok || option.Fun != n {
		return false
	}
	call, ok := parents[len(parents)-2].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && (sel.Sel.Name == "AddEvent" || sel.Sel.Name == "RecordError")
}

func hasRecordingGuard(call *ast.CallExpr, receiver ast.Expr, parents []ast.Node) bool {
	for i, parent := range parents {
		stmt, ok := parent.(*ast.IfStmt)
		if ok && stmt.Body.Pos() < call.Pos() && call.End() < stmt.Body.End() &&
			isRecordingCondition(stmt.Cond, receiver, parents[:i+1]) {
			return true
		}
	}
	return false
}

// isRecordingCondition recognizes positive IsRecording checks, including a
// cached result declared in an enclosing block. This is a source convention
// check, not a general control-flow analysis.
func isRecordingCondition(expr, receiver ast.Expr, parents []ast.Node) bool {
	switch expr := ast.Unparen(expr).(type) {
	case *ast.CallExpr:
		sel, ok := expr.Fun.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "IsRecording" && len(expr.Args) == 0 && sameReceiver(sel.X, receiver)
	case *ast.BinaryExpr:
		return expr.Op == token.LAND &&
			(isRecordingCondition(expr.X, receiver, parents) || isRecordingCondition(expr.Y, receiver, parents))
	case *ast.Ident:
		for i, parent := range slices.Backward(parents) {
			var statements []ast.Stmt
			switch parent := parent.(type) {
			case *ast.BlockStmt:
				statements = parent.List
			case *ast.IfStmt:
				if parent.Init != nil {
					statements = []ast.Stmt{parent.Init}
				}
			}
			for _, stmt := range slices.Backward(statements) {
				if stmt.End() >= expr.Pos() {
					continue
				}
				if value, found := assignedValue(stmt, expr.Name); found {
					return value != nil && isRecordingCondition(value, receiver, parents[:i+1])
				}
			}
		}
	}
	return false
}

func assignedValue(stmt ast.Stmt, name string) (ast.Expr, bool) {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		for i, lhs := range stmt.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
				if len(stmt.Rhs) == len(stmt.Lhs) {
					return stmt.Rhs[i], true
				}
				return nil, true
			}
		}
	case *ast.DeclStmt:
		if decl, ok := stmt.Decl.(*ast.GenDecl); ok {
			for _, spec := range decl.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok {
					for i, id := range value.Names {
						if id.Name == name {
							if len(value.Values) == len(value.Names) {
								return value.Values[i], true
							}
							return nil, true
						}
					}
				}
			}
		}
	}
	return nil, false
}

func sameReceiver(a, b ast.Expr) bool {
	switch a := ast.Unparen(a).(type) {
	case *ast.Ident:
		b, ok := ast.Unparen(b).(*ast.Ident)
		return ok && a.Name == b.Name
	case *ast.SelectorExpr:
		b, ok := ast.Unparen(b).(*ast.SelectorExpr)
		return ok && a.Sel.Name == b.Sel.Name && sameReceiver(a.X, b.X)
	}
	return false
}
