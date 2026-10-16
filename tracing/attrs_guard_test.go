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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNoAttributesPassedToTracerStart enforces the convention that span
// attributes are never passed to tracer.Start. The arguments of a Start call
// are evaluated at every call site even when tracing is disabled, which on
// the per-alert ingestion path means formatting fingerprints and building
// option slices for nothing. Attributes must instead be set afterwards with
// span.SetAttributes behind a span.IsRecording() check.
func TestNoAttributesPassedToTracerStart(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ""))
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
			case ".git", "ui", "node_modules", "testdata":
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
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isTracerStart(call) {
				return true
			}
			for _, arg := range call.Args {
				if isWithAttributesCall(arg) {
					rel, _ := filepath.Rel(root, path)
					pos := fset.Position(arg.Pos())
					violations = append(violations, fmt.Sprintf("%s:%d", rel, pos.Line))
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, violations,
		"trace.WithAttributes passed to tracer.Start; set attributes with span.SetAttributes behind span.IsRecording() instead")
}

// isTracerStart reports whether call is tracer.Start(...).
func isTracerStart(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Start" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "tracer"
}

// isWithAttributesCall reports whether expr is a call to a selector named
// WithAttributes, e.g. trace.WithAttributes(...).
func isWithAttributesCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "WithAttributes"
}
