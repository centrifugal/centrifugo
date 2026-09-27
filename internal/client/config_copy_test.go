package client

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHandlerDoesNotCopyConfig checks the client handler reads the config
// through ConfigRef rather than Config.
//
// Config returns the whole config by value, and it is large: a function
// holding a copy reserves stack space for it. The handler's functions run on
// connection goroutines, which start on a small stack, so a copy in any of them
// makes every connection grow its stack on its first command.
func TestHandlerDoesNotCopyConfig(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Config" {
				return true
			}
			if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "cfgContainer" {
				t.Errorf("%s: cfgContainer.Config() copies the whole config, use ConfigRef()", fset.Position(call.Pos()))
			}
			return true
		})
	}
}
