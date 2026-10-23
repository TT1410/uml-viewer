package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// function is one func or method of a source file. Name follows crap4go and
// mutate4go: `Name` for a function, `Type.Name` for a method.
type function struct {
	Name       string
	File       string // module-relative, slash-separated
	Namespace  string // import path of the package, dotted
	StartLine  int
	EndLine    int
	Complexity int
}

// moduleFunctions lists every function of non-test Go files under src.
func moduleFunctions(modDir, modPath, src string) ([]function, error) {
	var out []function
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return skipDir(path, src, d.Name())
		}
		if !isSource(d.Name()) {
			return nil
		}
		fns, err := fileFunctions(modDir, modPath, path)
		out = append(out, fns...)
		return err
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, err
}

func skipDir(path, root, name string) error {
	if path == root {
		return nil
	}
	if name == "vendor" || name == "testdata" || name == "target" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return filepath.SkipDir
	}
	return nil
}

func isSource(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func fileFunctions(modDir, modPath, path string) ([]function, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(modDir, path)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)
	ns := namespaceOf(modPath, rel)
	var out []function
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		out = append(out, function{
			Name:       functionName(fn),
			File:       rel,
			Namespace:  ns,
			StartLine:  fset.Position(fn.Pos()).Line,
			EndLine:    fset.Position(fn.End()).Line,
			Complexity: complexity(fn),
		})
	}
	return out, nil
}

// namespaceOf is the dotted import path of the package that holds rel.
func namespaceOf(modPath, rel string) string {
	dir := filepath.ToSlash(filepath.Dir(rel))
	path := modPath
	if dir != "." {
		path = modPath + "/" + dir
	}
	return strings.ReplaceAll(path, "/", ".")
}

// complexity counts decisions the way crap4go does.
func complexity(fn *ast.FuncDecl) int {
	n := 1
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch v := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			n++
		case *ast.BinaryExpr:
			if v.Op == token.LAND || v.Op == token.LOR {
				n++
			}
		}
		return true
	})
	return n
}

func functionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	}
	return "?"
}
