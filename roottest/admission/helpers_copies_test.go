package admission_test

// Copies of declarations from root test files that stay in the root package.
// Each entry names its source file in a "// from" comment.

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// from admission_switch_test.go
func newAdmissionDFAParser(t *testing.T) (*gts.Parser, []byte) {
	t.Helper()
	for _, name := range []string{"go", "lua", "toml", "ini", "json5", "css"} {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			continue
		}
		lang := entry.Language()
		if grammars.EvaluateParseSupport(*entry, lang).Backend != grammars.ParseBackendDFA {
			continue
		}
		source := []byte(grammars.ParseSmokeSample(name))
		probe := gts.NewParser(lang)
		probe.SetAdmissionCandidateRoute(false)
		tree, err := probe.Parse(source)
		if err != nil || tree == nil || tree.RootNode() == nil {
			continue
		}
		clean := tree.RootNode().EndByte() == uint32(len(source)) && !tree.RootNode().HasError()
		tree.Release()
		if !clean {
			continue
		}
		return gts.NewParser(lang), source
	}
	t.Skip("no clean DFA-backed language available for the routing test")
	return nil, nil
}

func requireCleanFullTree(t *testing.T, tree *gts.Tree, source []byte, label string) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s: nil tree/root", label)
	}
	root := tree.RootNode()
	if root.EndByte() != uint32(len(source)) {
		t.Fatalf("%s: truncated root end=%d source=%d", label, root.EndByte(), len(source))
	}
	if root.HasError() {
		t.Fatalf("%s: tree has error nodes", label)
	}
}

// from parser_external_scanner_incremental_test.go
func requireIncrementalDeepTreeMatchesFresh(t *testing.T, incremental, fresh *gts.Tree, lang *gts.Language) {
	t.Helper()
	incrementalInspection, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	freshInspection, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	if incrementalInspection.SHA256 != freshInspection.SHA256 {
		t.Fatalf("incremental/fresh deep tree mismatch: incremental=%s fresh=%s", incrementalInspection.SHA256, freshInspection.SHA256)
	}
}

// from benchmark_test.go
func pointAtOffset(src []byte, offset int) gts.Point {
	var row uint32
	var col uint32
	for i := 0; i < offset && i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
		i += size
	}
	return gts.Point{Row: row, Column: col}
}

// repoRootPath joins elements onto the module root, found by walking up from
// the working directory to go.mod. Tests moved out of the root directory use
// it to reach files that stay at the root.
func repoRootPath(elem ...string) string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(append([]string{dir}, elem...)...)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found above working directory")
		}
		dir = parent
	}
}
