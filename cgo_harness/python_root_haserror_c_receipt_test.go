//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestPythonRootHasErrorLockedCParity pins the finding's own hand example
// (finding.pre-existing-route-divergences-2026-08-02 section 3) as a locked,
// structural C-parity regression: the Go tree must match the C reference
// parser node-for-node, AND the root HasError() flag must agree.
//
// The two checks are deliberately both present and independent.
// runParityCase's compareNodes walks Type/StartByte/EndByte/IsNamed/
// IsMissing/ChildCount/FieldName but never inspects HasError() at any node,
// root included -- confirmed by reading compareNodes (parity_cgo_test.go):
// it is structurally blind to exactly the class of bug this test exists to
// pin. runParityCase alone would have reported this witness as passing even
// before the fix (verified interactively), because the pre-fix Go tree was
// already node-for-node identical to C's -- only the root's HasError() flag
// disagreed. The explicit HasError comparison below is what actually pins
// the regression; runParityCase is kept alongside it as a stronger, general
// structural-parity corroboration for this exact input.
func TestPythonRootHasErrorLockedCParity(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		// The finding's own hand example: a Go tree that matches C
		// node-for-node, where only root HasError() diverged pre-fix.
		{name: "dotted_import_trailing_dot", source: "from a import b, .c"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(t, parityCase{name: "python"}, test.name, []byte(test.source))

			cLang, err := ParityCLanguage("python")
			if err != nil {
				t.Fatalf("load C reference python parser: %v", err)
			}
			goTree, _, err := parseWithGo(parityCase{name: "python"}, []byte(test.source), nil)
			if err != nil {
				t.Fatalf("gotreesitter parse error: %v", err)
			}
			defer releaseGoTree(goTree)
			goRoot := goTree.RootNode()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("C parser SetLanguage error: %v", err)
			}
			cTree := cParser.Parse([]byte(test.source), nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatalf("C reference parser returned nil tree")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()

			if !cRoot.HasError() {
				t.Fatal("expected C reference root HasError()=true for this witness; witness may be stale")
			}
			if goRoot.HasError() != cRoot.HasError() {
				t.Errorf("root HasError() disagreement: go=%v c=%v", goRoot.HasError(), cRoot.HasError())
			}
		})
	}
}

// TestPythonRootHasErrorLockedCParityByteDrop pins the finding's second named
// witness: deleting byte 834 of the incremental-gate python_setup.py fixture
// (a call[632:1712] HasError=true nested under a module root that, pre-fix,
// read HasError=false). Reads the fixture relative to the top-level module's
// testdata directory since cgo_harness is its own Go module.
//
// This checks root HasError() agreement only, NOT full node-for-node parity
// (runParityCase's compareNodes): at this exact site the Go root also has a
// pre-existing, unrelated ChildCount divergence from the C reference (Go
// keeps 8 top-level siblings where C's error recovery groups them under
// fewer nodes) that is a structural-shape gap, not a false-clean HasError
// bug, and out of scope for the root-HasError propagation repair this test
// pins. The finding itself only ever claimed HasError agreement for this
// witness.
func TestPythonRootHasErrorLockedCParityByteDrop(t *testing.T) {
	src, err := os.ReadFile("../testdata/incremental_gate/python_setup.py")
	if err != nil {
		t.Fatalf("read python_setup.py fixture: %v", err)
	}
	if len(src) <= 834 {
		t.Fatalf("fixture too short for byte-834 witness: %d bytes", len(src))
	}
	edited := make([]byte, 0, len(src)-1)
	edited = append(edited, src[:834]...)
	edited = append(edited, src[835:]...)

	cLang, err := ParityCLanguage("python")
	if err != nil {
		t.Fatalf("load C reference python parser: %v", err)
	}
	goTree, _, err := parseWithGo(parityCase{name: "python"}, edited, nil)
	if err != nil {
		t.Fatalf("gotreesitter parse error: %v", err)
	}
	defer releaseGoTree(goTree)
	goRoot := goTree.RootNode()

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C parser SetLanguage error: %v", err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatalf("C reference parser returned nil tree")
	}
	defer cTree.Close()
	cRoot := cTree.RootNode()

	if !cRoot.HasError() {
		t.Fatal("expected C reference root HasError()=true for the byte-834-deleted witness; witness may be stale")
	}
	if goRoot.HasError() != cRoot.HasError() {
		t.Errorf("root HasError() disagreement: go=%v c=%v", goRoot.HasError(), cRoot.HasError())
	}
}

// TestPythonRootHasErrorAgreementCReceipts is the broader C-receipt sample
// required by the python root-HasError repair: for a spread of the
// incremental-gate corpus edit sites that were false-clean before the fix
// (root HasError()=false despite the tree containing ERROR/MISSING), assert
// specifically that the fixed Go root's HasError() now agrees with the C
// reference parser's root HasError() on the identical edited text. This is a
// narrower check than full structural parity (compareNodes) deliberately:
// the point is root-flag agreement, not full node-for-node identity, which a
// handful of these arbitrary byte-level error-recovery sites are not
// expected to have with the C reference (a separate, out-of-scope surface --
// see TestParityIncrementalParse and the incremental invariant gate for
// structural-shape parity).
func TestPythonRootHasErrorAgreementCReceipts(t *testing.T) {
	type site struct {
		path  string
		pos   int
		class string
	}
	sites := []site{
		{"python_setup.py", 12, "delete"},
		{"python_setup.py", 181, "delete"},
		{"python_setup.py", 269, "delete"},
		{"python_setup.py", 370, "replace"},
		{"python_setup.py", 463, "delete"},
		{"python_setup.py", 515, "delete"},
		{"python_setup.py", 550, "replace"},
		{"python_setup.py", 637, "insert"},
		{"python_setup.py", 683, "replace"},
		{"python_setup.py", 744, "insert"},
		{"python_setup.py", 1690, "replace"},
		{"python_setup.py", 1712, "delete"},
		{"python_large_indent.py", 901, "delete"},
		{"python_large_indent.py", 5401, "insert"},
		{"python_large_indent.py", 8101, "replace"},
		{"python_large_indent.py", 26101, "delete"},
		{"python_large_indent.py", 29701, "replace"},
	}

	cLang, err := ParityCLanguage("python")
	if err != nil {
		t.Fatalf("load C reference python parser: %v", err)
	}

	fileCache := map[string][]byte{}
	for _, s := range sites {
		s := s
		t.Run(fmt.Sprintf("%s_pos%d_%s", strings.TrimSuffix(s.path, ".py"), s.pos, s.class), func(t *testing.T) {
			src, ok := fileCache[s.path]
			if !ok {
				var err error
				src, err = os.ReadFile(filepath.Join("..", "testdata", "incremental_gate", s.path))
				if err != nil {
					t.Fatalf("read fixture %s: %v", s.path, err)
				}
				fileCache[s.path] = src
			}
			edited := pythonReceiptBuildEdit(src, s.pos, s.class)

			goTree, goLang, err := parseWithGo(parityCase{name: "python"}, edited, nil)
			if err != nil {
				t.Fatalf("gotreesitter parse error: %v", err)
			}
			defer releaseGoTree(goTree)
			goRoot := goTree.RootNode()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("C parser SetLanguage error: %v", err)
			}
			cTree := cParser.Parse(edited, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatalf("C reference parser returned nil tree")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()

			if goRoot.HasError() != cRoot.HasError() {
				t.Errorf("root HasError() disagreement: go=%v c=%v (go type=%q c kind=%q, lang=%v)",
					goRoot.HasError(), cRoot.HasError(), goRoot.Type(goLang), cRoot.Kind(), goLang != nil)
			}
			if !cRoot.HasError() {
				t.Errorf("expected this site to be a pre-fix false-clean witness (C root HasError=true); "+
					"C now reports false for %s pos=%d class=%s -- sample site list may be stale", s.path, s.pos, s.class)
			}
		})
	}
}

func pythonReceiptBuildEdit(src []byte, i int, class string) []byte {
	switch class {
	case "delete":
		edited := make([]byte, 0, len(src)-1)
		edited = append(edited, src[:i]...)
		edited = append(edited, src[i+1:]...)
		return edited
	case "insert":
		edited := make([]byte, 0, len(src)+1)
		edited = append(edited, src[:i]...)
		edited = append(edited, src[i])
		edited = append(edited, src[i:]...)
		return edited
	case "replace":
		repByte := byte('z')
		if src[i] == 'z' {
			repByte = 'y'
		}
		edited := append([]byte{}, src...)
		edited[i] = repByte
		return edited
	default:
		panic("pythonReceiptBuildEdit: unknown edit class " + class)
	}
}
