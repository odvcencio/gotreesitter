package bind

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The root package imports this package in every build, so everything it
// depends on is linked into every user's binary. Test support and the testing
// package must not be among those dependencies.
var bannedProductionImports = map[string]bool{
	"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest":     true,
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest": true,
	"testing": true,
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(self), "..", "..", ".."))
}

// TestProductionImports checks the direct imports of this package's files and
// of the root's ccore_*.go files. It needs no go tool, so it runs everywhere.
func TestProductionImports(t *testing.T) {
	root := repositoryRoot(t)
	dir := filepath.Join(root, "internal", "ccore", "bind")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	rootFiles, err := filepath.Glob(filepath.Join(root, "ccore_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rootFiles) == 0 {
		t.Fatal("no ccore_*.go file in the repository root")
	}
	var prod []string
	for _, f := range append(files, rootFiles...) {
		if !strings.HasSuffix(f, "_test.go") {
			prod = append(prod, f)
		}
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
			if bannedProductionImports[path] {
				t.Errorf("%s imports %s", filepath.Base(f), path)
			}
		}
	}
}

// TestRootDependencyClosure asks the go tool for every package the root
// package depends on (default build) and fails if test support or the testing
// package is among them, however indirectly it is reached.
func TestRootDependencyClosure(t *testing.T) {
	gotool := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(gotool); err != nil {
		var lookErr error
		if gotool, lookErr = exec.LookPath("go"); lookErr != nil {
			t.Skip("no go tool available to list dependencies")
		}
	}
	cmd := exec.Command(gotool, "list", "-deps", ".")
	cmd.Dir = repositoryRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps .: %v", err)
	}
	deps := strings.Fields(string(out))
	if len(deps) < 10 {
		t.Fatalf("go list -deps . listed %d packages; expected the root's dependencies", len(deps))
	}
	for _, d := range deps {
		if bannedProductionImports[d] {
			t.Errorf("the root package depends on %s", d)
		}
	}
}
