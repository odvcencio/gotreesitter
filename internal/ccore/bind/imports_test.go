package bind

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestProductionImports: the root package imports this package in every
// build, so everything these files import is linked into every user's binary.
// They must not import test support or the testing package.
func TestProductionImports(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	dir := filepath.Dir(self)
	root := filepath.Clean(filepath.Join(dir, "..", "..", ".."))
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var prod []string
	for _, f := range files {
		if len(f) > len("_test.go") && f[len(f)-len("_test.go"):] == "_test.go" {
			continue
		}
		prod = append(prod, f)
	}
	rootFiles, err := filepath.Glob(filepath.Join(root, "ccore_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rootFiles) == 0 {
		t.Fatal("no ccore_*.go file in the repository root")
	}
	for _, f := range rootFiles {
		if !strings.HasSuffix(f, "_test.go") {
			prod = append(prod, f)
		}
	}
	banned := map[string]bool{
		"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest":     true,
		"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest": true,
		"testing": true,
	}
	for _, f := range prod {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("%v", err)
		}
		af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, imp := range af.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if banned[path] {
				t.Errorf("%s imports %s", filepath.Base(f), path)
			}
		}
	}
}
