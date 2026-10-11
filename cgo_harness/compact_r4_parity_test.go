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
)

// Run one language per process. This checks the candidate route, including
// fallback, against C on every structural axis rather than legacy Go alone.
func TestCompactR4StructuralLockedC(t *testing.T) {
	name := strings.TrimSpace(os.Getenv("GTS_ADMISSION_REAL_CORPUS_LANGS"))
	if name == "" {
		t.Skip("set GTS_ADMISSION_REAL_CORPUS_LANGS to one R4 language")
	}
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
	if diffs := compactT3StructuralDivergences(tree.RootNode(), lang, cTree.RootNode()); len(diffs) != 0 {
		for _, diff := range diffs[:min(len(diffs), 8)] {
			t.Error(compactT3FormatDivergence(diff))
		}
		t.Fatalf("%d structural differences", len(diffs))
	}
}
