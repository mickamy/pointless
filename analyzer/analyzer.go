// Package analyzer provides a linter that suggests using value types instead of pointers
// when the struct is small enough and doesn't require pointer semantics.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// DefaultThreshold is the default size threshold in bytes.
// Structs smaller than or equal to this are candidates for value types.
const DefaultThreshold = 1024

// Analyzer is the pointless analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "pointless",
	Doc:      "suggests using value types instead of pointers for small structs",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// threshold can be configured via flags
var threshold int

// excludePatterns holds file patterns to exclude from analysis
var (
	excludePatterns []string
	excludeMu       sync.RWMutex
)

// SetConfig sets the exclude patterns from config file.
func SetConfig(exclude []string) {
	excludeMu.Lock()
	defer excludeMu.Unlock()
	excludePatterns = exclude
}

func init() {
	Analyzer.Flags.IntVar(&threshold, "threshold", DefaultThreshold, "size threshold in bytes")
}

func run(pass *analysis.Pass) (interface{}, error) {
	ispct := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Build set of excluded files
	excludedFiles := make(map[string]bool)
	excludeMu.RLock()
	patterns := excludePatterns
	excludeMu.RUnlock()

	if len(patterns) > 0 {
		for _, f := range pass.Files {
			filename := pass.Fset.File(f.Pos()).Name()
			if shouldExclude(filename, patterns) {
				excludedFiles[filename] = true
			}
		}
	}

	// Build nolint comment map (line number -> true if has nolint)
	nolintLines := buildNolintMap(pass)

	// Track nil returns per function to avoid false positives
	nilReturns := findNilReturns(ispct)

	// Track receiver mutations per method
	receiverMutations := findReceiverMutations(pass, ispct)

	// Track nil comparisons/assignments for pointer slices
	nilUsages := findNilUsages(ispct)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.GenDecl)(nil),
		(*ast.AssignStmt)(nil),
	}

	ispct.Preorder(nodeFilter, func(n ast.Node) {
		// Skip excluded files
		filename := pass.Fset.File(n.Pos()).Name()
		if excludedFiles[filename] {
			return
		}

		// Skip if nolint comment is present
		line := pass.Fset.Position(n.Pos()).Line
		if nolintLines[line] {
			return
		}

		switch node := n.(type) {
		case *ast.FuncDecl:
			checkFuncDecl(pass, node, nilReturns, receiverMutations)
		case *ast.GenDecl:
			checkGenDecl(pass, node, nilUsages)
		case *ast.AssignStmt:
			checkAssignStmt(pass, node, nilUsages)
		}
	})

	return nil, nil
}

// checkFuncDecl checks function return types and method receivers.
func checkFuncDecl(pass *analysis.Pass, fn *ast.FuncDecl, nilReturns map[*ast.FuncDecl]bool, receiverMutations map[*ast.FuncDecl]bool) {
	// Check method receiver
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		checkMethodReceiver(pass, fn, receiverMutations)
	}

	// Check return type
	if fn.Type.Results != nil {
		checkReturnType(pass, fn, nilReturns)
	}
}

// checkMethodReceiver checks if a pointer receiver could be a value receiver.
func checkMethodReceiver(pass *analysis.Pass, fn *ast.FuncDecl, receiverMutations map[*ast.FuncDecl]bool) {
	recv := fn.Recv.List[0]

	star, ok := recv.Type.(*ast.StarExpr)
	if !ok {
		return // already a value receiver
	}

	// Skip if receiver is mutated
	if receiverMutations[fn] {
		return
	}

	// Get the underlying type
	tv, ok := pass.TypesInfo.Types[star.X]
	if !ok {
		return
	}

	size := sizeOf(pass, tv.Type)
	if size > int64(threshold) {
		return // struct is too large
	}

	typeName := types.TypeString(tv.Type, types.RelativeTo(pass.Pkg))
	pass.Reportf(fn.Pos(), "consider using value receiver: %s is %d bytes (threshold: %d bytes) and method doesn't mutate receiver", typeName, size, threshold)
}

// checkReturnType checks if a pointer return type could be a value type.
func checkReturnType(pass *analysis.Pass, fn *ast.FuncDecl, nilReturns map[*ast.FuncDecl]bool) {
	for _, result := range fn.Type.Results.List {
		switch t := result.Type.(type) {
		case *ast.StarExpr:
			checkPointerReturn(pass, fn, t, nilReturns)
		case *ast.ArrayType:
			checkSliceReturn(pass, fn, t, nilReturns)
		}
	}
}

// checkPointerReturn checks a pointer return type.
func checkPointerReturn(pass *analysis.Pass, fn *ast.FuncDecl, star *ast.StarExpr, nilReturns map[*ast.FuncDecl]bool) {
	// Skip if function returns nil
	if nilReturns[fn] {
		return
	}

	tv, ok := pass.TypesInfo.Types[star.X]
	if !ok {
		return
	}

	// Only check structs
	if _, ok := tv.Type.Underlying().(*types.Struct); !ok {
		return
	}

	size := sizeOf(pass, tv.Type)
	if size > int64(threshold) {
		return
	}

	typeName := types.TypeString(tv.Type, types.RelativeTo(pass.Pkg))
	pass.Reportf(star.Pos(), "consider returning value instead of pointer: %s is %d bytes (threshold: %d bytes)", typeName, size, threshold)
}

// checkSliceReturn checks a slice return type for pointer elements.
func checkSliceReturn(pass *analysis.Pass, fn *ast.FuncDecl, arr *ast.ArrayType, nilReturns map[*ast.FuncDecl]bool) {
	if arr.Len != nil {
		return // array, not slice
	}

	star, ok := arr.Elt.(*ast.StarExpr)
	if !ok {
		return // not a pointer slice
	}

	// Skip if function returns nil (for the slice itself)
	if nilReturns[fn] {
		return
	}

	tv, ok := pass.TypesInfo.Types[star.X]
	if !ok {
		return
	}

	// Only check structs
	if _, ok := tv.Type.Underlying().(*types.Struct); !ok {
		return
	}

	size := sizeOf(pass, tv.Type)
	if size > int64(threshold) {
		return
	}

	typeName := types.TypeString(tv.Type, types.RelativeTo(pass.Pkg))
	pass.Reportf(arr.Pos(), "consider using []%s instead of []*%s: better cache locality and lower GC pressure (%d bytes, threshold: %d bytes)", typeName, typeName, size, threshold)
}

// checkGenDecl checks variable declarations for pointer slices.
func checkGenDecl(pass *analysis.Pass, decl *ast.GenDecl, nilUsages map[token.Pos]bool) {
	if decl.Tok != token.VAR {
		return
	}

	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		arr, ok := vs.Type.(*ast.ArrayType)
		if !ok || arr.Len != nil {
			continue
		}

		star, ok := arr.Elt.(*ast.StarExpr)
		if !ok {
			continue
		}

		// Check if any of the declared names have nil usage
		hasNilUsage := false
		for _, name := range vs.Names {
			if obj := pass.TypesInfo.Defs[name]; obj != nil {
				if nilUsages[obj.Pos()] {
					hasNilUsage = true
					break
				}
			}
		}
		if hasNilUsage {
			continue
		}

		tv, ok := pass.TypesInfo.Types[star.X]
		if !ok {
			continue
		}

		if _, ok := tv.Type.Underlying().(*types.Struct); !ok {
			continue
		}

		size := sizeOf(pass, tv.Type)
		if size > int64(threshold) {
			continue
		}

		typeName := types.TypeString(tv.Type, nil)
		pass.Reportf(arr.Pos(), "consider using []%s instead of []*%s: better cache locality and lower GC pressure (%d bytes, threshold: %d bytes)", typeName, typeName, size, threshold)
	}
}

// checkAssignStmt checks short variable declarations for pointer slices.
func checkAssignStmt(pass *analysis.Pass, stmt *ast.AssignStmt, nilUsages map[token.Pos]bool) {
	if stmt.Tok != token.DEFINE {
		return
	}

	for i, rhs := range stmt.Rhs {
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			continue
		}

		// Check for make([]*T, ...)
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "make" {
			continue
		}

		if len(call.Args) < 1 {
			continue
		}

		arr, ok := call.Args[0].(*ast.ArrayType)
		if !ok || arr.Len != nil {
			continue
		}

		star, ok := arr.Elt.(*ast.StarExpr)
		if !ok {
			continue
		}

		// Check if the variable has nil usage
		if i < len(stmt.Lhs) {
			if ident, ok := stmt.Lhs[i].(*ast.Ident); ok {
				if obj := pass.TypesInfo.Defs[ident]; obj != nil {
					if nilUsages[obj.Pos()] {
						continue
					}
				}
			}
		}

		tv, ok := pass.TypesInfo.Types[star.X]
		if !ok {
			continue
		}

		if _, ok := tv.Type.Underlying().(*types.Struct); !ok {
			continue
		}

		size := sizeOf(pass, tv.Type)
		if size > int64(threshold) {
			continue
		}

		typeName := types.TypeString(tv.Type, nil)
		pass.Reportf(arr.Pos(), "consider using []%s instead of []*%s: better cache locality and lower GC pressure (%d bytes, threshold: %d bytes)", typeName, typeName, size, threshold)
	}
}

// findNilReturns finds all functions that return nil.
func findNilReturns(inspect *inspector.Inspector) map[*ast.FuncDecl]bool {
	result := make(map[*ast.FuncDecl]bool)
	var currentFunc *ast.FuncDecl

	inspect.Preorder([]ast.Node{(*ast.FuncDecl)(nil), (*ast.ReturnStmt)(nil)}, func(n ast.Node) {
		switch node := n.(type) {
		case *ast.FuncDecl:
			currentFunc = node
		case *ast.ReturnStmt:
			if currentFunc == nil {
				return
			}
			for _, expr := range node.Results {
				if isNil(expr) {
					result[currentFunc] = true
					return
				}
			}
		}
	})

	return result
}

// findReceiverMutations finds all methods that mutate their receiver.
func findReceiverMutations(pass *analysis.Pass, inspect *inspector.Inspector) map[*ast.FuncDecl]bool {
	result := make(map[*ast.FuncDecl]bool)
	var currentFunc *ast.FuncDecl
	var receiverObj types.Object

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.AssignStmt)(nil),
		(*ast.IncDecStmt)(nil),
	}

	inspect.Preorder(nodeFilter, func(n ast.Node) {
		switch node := n.(type) {
		case *ast.FuncDecl:
			currentFunc = node
			receiverObj = nil
			if node.Recv != nil && len(node.Recv.List) > 0 {
				recv := node.Recv.List[0]
				if len(recv.Names) > 0 {
					receiverObj = pass.TypesInfo.Defs[recv.Names[0]]
				}
			}
		case *ast.AssignStmt:
			if currentFunc == nil || receiverObj == nil {
				return
			}
			for _, lhs := range node.Lhs {
				if refersToReceiver(pass, lhs, receiverObj) {
					result[currentFunc] = true
					return
				}
			}
		case *ast.IncDecStmt:
			if currentFunc == nil || receiverObj == nil {
				return
			}
			if refersToReceiver(pass, node.X, receiverObj) {
				result[currentFunc] = true
			}
		}
	})

	return result
}

// refersToReceiver checks if an expression refers to the receiver or its fields.
func refersToReceiver(pass *analysis.Pass, expr ast.Expr, receiverObj types.Object) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		if obj := pass.TypesInfo.Uses[e]; obj == receiverObj {
			return true
		}
	case *ast.SelectorExpr:
		return refersToReceiver(pass, e.X, receiverObj)
	case *ast.IndexExpr:
		return refersToReceiver(pass, e.X, receiverObj)
	case *ast.StarExpr:
		return refersToReceiver(pass, e.X, receiverObj)
	}
	return false
}

// findNilUsages finds all variables that are used with nil (comparison or assignment).
func findNilUsages(inspect *inspector.Inspector) map[token.Pos]bool {
	result := make(map[token.Pos]bool)

	nodeFilter := []ast.Node{
		(*ast.BinaryExpr)(nil),
		(*ast.AssignStmt)(nil),
	}

	inspect.Preorder(nodeFilter, func(n ast.Node) {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			// Check for slice[i] == nil or slice[i] != nil
			if node.Op == token.EQL || node.Op == token.NEQ {
				if isNil(node.Y) {
					if idx, ok := node.X.(*ast.IndexExpr); ok {
						if ident, ok := idx.X.(*ast.Ident); ok {
							if ident.Obj != nil {
								result[ident.Obj.Pos()] = true
							}
						}
					}
				}
				if isNil(node.X) {
					if idx, ok := node.Y.(*ast.IndexExpr); ok {
						if ident, ok := idx.X.(*ast.Ident); ok {
							if ident.Obj != nil {
								result[ident.Obj.Pos()] = true
							}
						}
					}
				}
			}
		case *ast.AssignStmt:
			// Check for slice[i] = nil
			for i, lhs := range node.Lhs {
				if idx, ok := lhs.(*ast.IndexExpr); ok {
					if i < len(node.Rhs) && isNil(node.Rhs[i]) {
						if ident, ok := idx.X.(*ast.Ident); ok {
							if ident.Obj != nil {
								result[ident.Obj.Pos()] = true
							}
						}
					}
				}
			}
		}
	})

	return result
}

// isNil checks if an expression is the nil identifier.
func isNil(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// sizeOf calculates the size of a type in bytes.
func sizeOf(pass *analysis.Pass, t types.Type) int64 {
	sizes := types.SizesFor("gc", "amd64")
	if sizes == nil {
		// Fallback: assume 64-bit
		sizes = &types.StdSizes{WordSize: 8, MaxAlign: 8}
	}
	return sizes.Sizeof(t)
}

// formatBytes formats a byte count for display.
func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d bytes", bytes)
	}
	return fmt.Sprintf("%d bytes (%.1f KB)", bytes, float64(bytes)/1024)
}

// shouldExclude checks if a file path matches any exclude pattern.
func shouldExclude(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Try matching against full path
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Try matching against base name
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}

// buildNolintMap builds a map of line numbers that have nolint comments.
// Supports both //nolint:pointless and //pointless:ignore formats.
func buildNolintMap(pass *analysis.Pass) map[int]bool {
	result := make(map[int]bool)

	for _, f := range pass.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				text := c.Text
				// Remove // or /* */ markers
				if strings.HasPrefix(text, "//") {
					text = strings.TrimPrefix(text, "//")
				} else if strings.HasPrefix(text, "/*") {
					text = strings.TrimPrefix(text, "/*")
					text = strings.TrimSuffix(text, "*/")
				}
				text = strings.TrimSpace(text)

				if isNolintComment(text) {
					line := pass.Fset.Position(c.Pos()).Line
					result[line] = true
					// Also mark the next line (for comments above declarations)
					result[line+1] = true
				}
			}
		}
	}

	return result
}

// isNolintComment checks if a comment text indicates nolint for pointless.
func isNolintComment(text string) bool {
	// Check for //nolint:pointless or //nolint (blanket)
	if strings.HasPrefix(text, "nolint") {
		// //nolint or //nolint:pointless or //nolint:foo,pointless,bar
		rest := strings.TrimPrefix(text, "nolint")
		if rest == "" || rest[0] == ' ' || rest[0] == '\t' {
			// Blanket nolint
			return true
		}
		if rest[0] == ':' {
			linters := strings.TrimPrefix(rest, ":")
			for _, l := range strings.Split(linters, ",") {
				if strings.TrimSpace(l) == "pointless" {
					return true
				}
			}
		}
	}

	// Check for //pointless:ignore
	if strings.HasPrefix(text, "pointless:ignore") {
		return true
	}

	return false
}
