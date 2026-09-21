//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	grt "github.com/odvcencio/gotreesitter/grammars/runtime"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestParityDoxygenRecoverEOFRootMatchesC is the doxygen witness from pine's
// 2026-09-21 diagnosis (task #70). The shipped doxygen.bin has zero
// ExternalLexStates rows, so the C-recovery cost-competition gate is off
// today and this source never reaches cRecoverEOFAccept. Regenerating the
// blob from the locked parser.c restores the 8 ExternalLexStates rows and
// flips the gate on (generatedCRecoveryDefaultSafe, parser_recover_c.go);
// this test proves the Go port then matches tree-sitter C exactly — a bare
// childless ERROR spanning the whole source — instead of nesting it under a
// synthetic "document" wrapper (tryBuildExpectedRootFromSingleError,
// parser_result_root_build.go).
//
// The exercised blob is generated into a t.TempDir() and never committed.
// Regenerate it by hand with:
//
//	cgo_harness/seed_parity_repos.sh --langs doxygen
//	go run ./cmd/ts2go -input /tmp/grammar_parity/doxygen/src/parser.c \
//	  -output <scratch>/doxygen.go -name doxygen -package grammars
func TestParityDoxygenRecoverEOFRootMatchesC(t *testing.T) {
	const parserSrc = "/tmp/grammar_parity/doxygen/src/parser.c"
	if _, err := os.Stat(parserSrc); err != nil {
		t.Skipf("doxygen grammar not seeded (run cgo_harness/seed_parity_repos.sh --langs doxygen first): %v", err)
	}

	scratch := t.TempDir()
	outputGo := filepath.Join(scratch, "doxygen.go")
	genCmd := exec.Command("go", "run", "./cmd/ts2go",
		"-input", parserSrc,
		"-output", outputGo,
		"-name", "doxygen",
		"-package", "grammars",
	)
	genCmd.Dir = ".." // cmd/ts2go lives in the top-level module, not cgo_harness's.
	if out, err := genCmd.CombinedOutput(); err != nil {
		t.Fatalf("go run ./cmd/ts2go: %v\n%s", err, out)
	}
	blobPath := filepath.Join(scratch, "grammar_blobs", "doxygen.bin")
	blob, err := os.ReadFile(blobPath)
	if err != nil {
		t.Fatalf("read regenerated blob: %v", err)
	}

	regen, err := gotreesitter.LoadLanguage(blob)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if !grt.AttachLanguageSupport("doxygen", regen) {
		t.Fatal("AttachLanguageSupport(doxygen) failed to attach the external scanner")
	}
	if len(regen.ExternalLexStates) == 0 {
		t.Fatal("regenerated doxygen blob has zero ExternalLexStates rows; this test needs the fresh 8-row table pine's diagnosis describes")
	}

	// Force the gate on regardless of the language's own default: this test
	// exercises the recover_eof unwrapping mechanism, not doxygen's opt-out
	// default (covered separately by TestCRecoveryGateDoxygenOptOut).
	t.Setenv("GOT_C_RECOVERY", "doxygen")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)

	source := []byte(`/** Adds all words in \a s to document \a doc with weight \a wfd */`)

	cLanguage, err := COracleLanguage("doxygen")
	if err != nil {
		t.Fatalf("COracleLanguage: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("SetLanguage: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot.Kind() != "ERROR" || !cRoot.HasError() || cRoot.ChildCount() != 0 {
		t.Fatalf("C oracle shape changed: kind=%s hasError=%v children=%d sexp=%s; update this receipt's expectation",
			cRoot.Kind(), cRoot.HasError(), cRoot.ChildCount(), cRoot.ToSexp())
	}

	parser := gotreesitter.NewParser(regen)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()

	if got := root.Type(regen); got != "ERROR" {
		t.Fatalf("root type = %q, want ERROR (matching C's bare recover_eof root); sexp=%s", got, root.SExpr(regen))
	}
	if !root.HasError() {
		t.Fatalf("root.HasError() = false, want true; sexp=%s", root.SExpr(regen))
	}
	if got := root.ChildCount(); got != 0 {
		t.Fatalf("root child count = %d, want 0 (matching C's childless ERROR); sexp=%s", got, root.SExpr(regen))
	}
	if root.StartByte() != 0 || int(root.EndByte()) != len(source) {
		t.Fatalf("root span = %d..%d, want 0..%d", root.StartByte(), root.EndByte(), len(source))
	}
}
