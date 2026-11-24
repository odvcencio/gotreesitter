//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// Run one language per process. This checks the candidate route, including
// fallback, against C on every structural axis rather than legacy Go alone.
func TestCompactR4StructuralLockedC(t *testing.T) {
	name := strings.TrimSpace(os.Getenv("GTS_ADMISSION_REAL_CORPUS_LANGS"))
	if name == "" {
		t.Skip("set GTS_ADMISSION_REAL_CORPUS_LANGS to one R4 language")
	}
	testCompactR4StructuralLockedC(t, name, false)
}

func TestCompactYAMLVersionOwnershipCOracle(t *testing.T) {
	t.Cleanup(core.SetExternalLexerOwnershipProbeForTest(true))
	testCompactR4StructuralLockedC(t, "yaml", true)
}

func testCompactR4StructuralLockedC(t *testing.T, name string, requireCompact bool) {
	t.Helper()
	entry := grammars.DetectLanguageByName(name)
	if entry == nil || strings.Contains(name, ",") {
		t.Fatal("select exactly one registered R4 language")
	}
	raw, err := os.ReadFile("../internal/benchfixtures/real_corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language, Role, SHA256 string
			Bytes                  int
			Path                   string `json:"committed_path"`
		}
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	var source []byte
	for _, row := range manifest.Entries {
		if row.Language != name || row.Role != "sample" || row.Path == "" {
			continue
		}
		source, err = os.ReadFile(filepath.Join("../internal/benchfixtures", row.Path))
		if err != nil {
			t.Fatal(err)
		}
		if len(source) != row.Bytes || fmt.Sprintf("%x", sha256.Sum256(source)) != row.SHA256 {
			t.Fatal("R4 sample identity changed")
		}
		break
	}
	if len(source) == 0 {
		t.Fatal("language has no committed real R4 sample")
	}
	cLang, err := ParityCLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cTree := compactT3ParseC(t, cLang, source)
	defer cTree.Close()
	lang := entry.Language()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	if requireCompact {
		// The witness pins the compact route. A certified grammar's candidate
		// requests go to the certified core; the telemetry keeps this one compact.
		p.SetCompactCertificationTelemetry(true)
	}
	beforeRouted, beforeFallback := gts.AdmissionCandidateCounters()
	var tree *gts.Tree
	if grammars.EvaluateParseSupport(*entry, lang).Backend == grammars.ParseBackendTokenSource && entry.TokenSourceFactory != nil {
		tree, err = p.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
	} else {
		tree, err = p.Parse(source)
	}
	if err != nil || tree == nil || tree.RootNode() == nil {
		t.Fatalf("parse: tree=%v err=%v", tree != nil, err)
	}
	defer tree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	t.Logf("R4 language=%s bytes=%d compact=%d fallback=%d reason=%q", name, len(source), routed-beforeRouted, fallback-beforeFallback, gts.AdmissionCandidateLastFallbackReason())
	if requireCompact && (routed-beforeRouted != 1 || fallback != beforeFallback) {
		t.Fatal("version ownership witness did not complete on compact")
	}
	if requireCompact && lang.ExternalScanner != nil {
		runtime := tree.ParseRuntime()
		// A scanner can certify transfer without storing checkpoints. The
		// published tree must retain that proof after comparison-only trials.
		if !runtime.CompactExternalScannerCheckpointTransferProven {
			t.Fatal("published scanner transfer lost its proof after acceptance trials")
		}
	}
	if diffs := compactT3StructuralDivergences(tree.RootNode(), lang, cTree.RootNode()); len(diffs) != 0 {
		for _, diff := range diffs[:min(len(diffs), 8)] {
			t.Error(compactT3FormatDivergence(diff))
		}
		t.Fatalf("%d structural differences", len(diffs))
	}
}

func TestGoNestedGenericSelectionLockedC(t *testing.T) {
	source := []byte("package p\n\nfunc f() {\n\t_ = Foo[int](Foo[int](a))\n}\n")
	lang := grammars.GoLanguage()
	cLang, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cTree := compactT3ParseC(t, cLang, source)
	defer cTree.Close()
	for _, candidate := range []bool{false, true} {
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(candidate)
		before, beforeFallback := gts.AdmissionCandidateCounters()
		tree, err := p.Parse(source)
		if err != nil || tree == nil {
			t.Fatalf("parse: %v", err)
		}
		defer tree.Release()
		routed, fallback := gts.AdmissionCandidateCounters()
		t.Logf("candidate=%t route=%d fallback=%d tree=%s", candidate, routed-before, fallback-beforeFallback, tree.RootNode().SExpr(lang))
		diffs := compactT3StructuralDivergences(tree.RootNode(), lang, cTree.RootNode())
		for _, d := range diffs[:min(len(diffs), 8)] {
			t.Error(compactT3FormatDivergence(d))
		}
		if len(diffs) > 0 {
			t.Errorf("candidate=%t differences=%d", candidate, len(diffs))
		}
	}
}
