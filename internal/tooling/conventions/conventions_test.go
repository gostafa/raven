package conventions

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type source struct {
	file    *ast.File
	imports map[string]string
}

func parseSource(name, text string) (source, error) {
	f, err := parser.ParseFile(token.NewFileSet(), name, text, parser.ParseComments)
	if err != nil {
		return source{}, err
	}
	s := source{file: f, imports: map[string]string{}}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return source{}, err
		}
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		s.imports[name] = path
	}
	return s, nil
}

func filenameError(name string, s source) error {
	pkg := strings.TrimSuffix(s.file.Name.Name, "_test")
	platform := `(?:_(?:darwin|linux|windows|android|ios|freebsd|openbsd|netbsd|dragonfly|solaris|illumos|aix|plan9|js|wasip1|other|server|amd64|arm64|386|arm|wasm))*`
	if strings.HasSuffix(name, "_test.go") {
		kind := ""
		if s.file.Name.Name != pkg {
			kind = "_integration"
		}
		pattern := "^" + regexp.QuoteMeta(pkg+kind) + platform + `(?:_bench)?_test\.go$`
		if !regexp.MustCompile(pattern).MatchString(name) {
			return fmt.Errorf("%s: use package-based test filenames", name)
		}
		return nil
	}
	if !regexp.MustCompile(`^(funcs|types|consts|vars)` + platform + `\.go$`).MatchString(name) {
		return fmt.Errorf("%s: use a role-based Go filename", name)
	}
	role := strings.Split(strings.TrimSuffix(name, ".go"), "_")[0]
	if s.file.Doc != nil && role != "types" {
		return fmt.Errorf("%s: package documentation belongs in types.go", name)
	}
	for _, d := range s.file.Decls {
		want := "funcs"
		if g, ok := d.(*ast.GenDecl); ok {
			if g.Tok == token.IMPORT {
				continue
			}
			want = strings.ToLower(g.Tok.String()) + "s"
		}
		if role != want {
			return fmt.Errorf("%s: %s declarations belong in %s.go", name, want, want)
		}
	}
	return nil
}

func coreDependencyError(path string, s source) error {
	if !strings.HasPrefix(path, "internal/") || strings.Contains(path, "/adapters/") ||
		strings.HasPrefix(path, "internal/tooling/conventions/") {
		return nil
	}
	for _, dependency := range s.imports {
		if strings.Contains(dependency, "/adapters/") ||
			strings.Contains(strings.Split(dependency, "/")[0], ".") {
			return fmt.Errorf("%s: core imports adapter dependency %s", path, dependency)
		}
	}
	return nil
}

func featureLayoutError(path string) error {
	if strings.HasPrefix(path, "internal/features/") && len(strings.Split(path, "/")) < 5 {
		return fmt.Errorf("%s: feature roots contain only layer directories", path)
	}
	return nil
}

func facadeErrors(sources []source) []error {
	types := map[string]struct {
		expr  ast.Expr
		owner source
	}{}
	for _, s := range sources {
		for _, d := range s.file.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
				for _, spec := range g.Specs {
					v := spec.(*ast.TypeSpec)
					types[v.Name.Name] = struct {
						expr  ast.Expr
						owner source
					}{v.Type, s}
				}
			}
		}
	}
	errors := []error{}
	var check func(ast.Expr, source, map[string]bool)
	check = func(expr ast.Expr, owner source, seen map[string]bool) {
		switch v := expr.(type) {
		case *ast.Ident:
			if seen[v.Name] {
				return
			}
			seen[v.Name] = true
			if t, ok := types[v.Name]; ok {
				if !ast.IsExported(v.Name) {
					errors = append(errors, fmt.Errorf("facade exposes private type %s", v.Name))
				}
				check(t.expr, t.owner, seen)
			}
		case *ast.SelectorExpr:
			if pkg, ok := v.X.(*ast.Ident); ok {
				path := owner.imports[pkg.Name]
				if (strings.HasPrefix(path, "raven/") && !strings.HasPrefix(path, "raven/api/")) ||
					strings.Contains(strings.Split(path, "/")[0], ".") {
					errors = append(errors, fmt.Errorf("facade exposes %s.%s", path, v.Sel.Name))
				}
			}
		case *ast.StarExpr:
			check(v.X, owner, seen)
		case *ast.ArrayType:
			check(v.Elt, owner, seen)
		case *ast.MapType:
			check(v.Key, owner, seen)
			check(v.Value, owner, seen)
		case *ast.ChanType:
			check(v.Value, owner, seen)
		case *ast.Ellipsis:
			check(v.Elt, owner, seen)
		case *ast.StructType:
			for _, field := range v.Fields.List {
				public := len(field.Names) == 0
				for _, name := range field.Names {
					public = public || ast.IsExported(name.Name)
				}
				if public {
					check(field.Type, owner, seen)
				}
			}
		case *ast.InterfaceType:
			for _, field := range v.Methods.List {
				check(field.Type, owner, seen)
			}
		case *ast.FuncType:
			for _, fields := range []*ast.FieldList{v.Params, v.Results} {
				if fields != nil {
					for _, field := range fields.List {
						check(field.Type, owner, seen)
					}
				}
			}
		case *ast.IndexExpr:
			check(v.X, owner, seen)
			check(v.Index, owner, seen)
		case *ast.IndexListExpr:
			check(v.X, owner, seen)
			for _, index := range v.Indices {
				check(index, owner, seen)
			}
		}
	}
	for name, t := range types {
		if ast.IsExported(name) {
			check(t.expr, t.owner, map[string]bool{})
		}
	}
	for _, s := range sources {
		for _, d := range s.file.Decls {
			f, ok := d.(*ast.FuncDecl)
			if !ok || !ast.IsExported(f.Name.Name) {
				continue
			}
			if f.Recv != nil {
				recv := f.Recv.List[0].Type
				if p, ok := recv.(*ast.StarExpr); ok {
					recv = p.X
				}
				if name, ok := recv.(*ast.Ident); ok && !ast.IsExported(name.Name) {
					continue
				}
			}
			check(f.Type, s, map[string]bool{})
		}
	}
	return errors
}

func TestRepositoryConventions(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	var facade []source
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".cache", ".task", "taskotter", "node_modules", "frontend", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := featureLayoutError(rel); err != nil {
			t.Error(err)
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s, err := parseSource(path, string(data))
		if err != nil {
			return err
		}
		if err := filenameError(d.Name(), s); err != nil {
			t.Error(err)
		}
		if !strings.HasSuffix(path, "_test.go") {
			if err := coreDependencyError(rel, s); err != nil {
				t.Error(err)
			}
			if filepath.Dir(path) == root {
				facade = append(facade, s)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range facadeErrors(facade) {
		t.Error(err)
	}
}

func TestConventionFixtures(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		valid      bool
	}{
		{"funcs.go", "package example; func f(){}", true},
		{"types.go", "package example; type Value struct{}", true},
		{"errors.go", "package example; var Err error", false},
		{"funcs.go", "package example; type Value struct{}", false},
		{"funcs_review.go", "package example; func f(){}", false},
		{"example_integration_test.go", "package example_test; func f(){}", true},
		{"extra_test.go", "package example; func f(){}", false},
	} {
		t.Run(tc.name+tc.text, func(t *testing.T) {
			s, err := parseSource(tc.name, tc.text)
			if err != nil {
				t.Fatal(err)
			}
			if got := filenameError(tc.name, s) == nil; got != tc.valid {
				t.Fatalf("valid=%v want %v", got, tc.valid)
			}
		})
	}
	for _, text := range []string{
		`package main; import library "raven/internal/features/library/domain"; type Card = library.Card`,
		`package main; import library "raven/internal/features/library/domain"; type Envelope struct{ Items []Card }; type Card struct{ Value library.Card }`,
		`package main; import ext "example.com/vendor"; type Port interface{ Read() ext.Result }`,
	} {
		s, err := parseSource("types.go", text)
		if err != nil {
			t.Fatal(err)
		}
		if len(facadeErrors([]source{s})) == 0 {
			t.Fatal("accepted leaking facade", text)
		}
	}
	s, err := parseSource(
		"types.go",
		`package main; import "time"; type Card struct{Due time.Time}; type Holder struct{private interface{}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if errors := facadeErrors([]source{s}); len(errors) > 0 {
		t.Fatal(errors)
	}
	s, err = parseSource(
		"funcs.go",
		`package main; import "raven/api/cards"; func Review(input *cards.ReviewInput) (cards.Card, error) { panic("fixture") }`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if errors := facadeErrors([]source{s}); len(errors) > 0 {
		t.Fatal(errors)
	}
	s, err = parseSource(
		"funcs.go",
		`package library; import _ "raven/internal/features/library/adapters/boltstore"`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if coreDependencyError("internal/features/library/domain/funcs.go", s) == nil {
		t.Fatal("accepted core-to-adapter dependency")
	}
}

func TestFeatureLayoutFixtures(t *testing.T) {
	for _, tc := range []struct {
		path  string
		valid bool
	}{
		{"internal/features/library/types.go", false},
		{"internal/features/library/library_test.go", false},
		{"internal/features/library/README.md", false},
		{"internal/features/README.md", false},
		{"internal/features/library/domain/types.go", true},
		{"internal/features/library/domain/domain_integration_test.go", true},
		{"internal/features/library/adapters/boltstore/types.go", true},
		{"internal/shared/filesystem/types.go", true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := featureLayoutError(tc.path) == nil; got != tc.valid {
				t.Fatalf("valid=%v want %v", got, tc.valid)
			}
		})
	}
}
