//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const sqlC26ahCompactFallback = "compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token"

func TestSQLC26ahGeneratedDollarQuoteBlocker(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	var grammar grammargenCGOGrammar
	for _, candidate := range grammargenCGOGrammars {
		if candidate.name == "sql" {
			grammar = candidate
			break
		}
	}
	if grammar.name == "" {
		t.Fatal("missing sql grammargen CGO config")
	}

	gram, err := importGrammargenSource(grammar)
	if err != nil {
		t.Skipf("import unavailable: %v", err)
	}
	genLang, err := grammargenGenerate(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate grammar: %v", err)
	}
	refLang := grammar.blobFunc()
	adaptGrammargenCGOExternalScanner("sql", refLang, genLang)
	grammarHash, ok := genLang.GrammarBlobSHA256()
	if !ok {
		t.Fatal("generated SQL language has no authenticated grammar identity")
	}
	provider, ok := genLang.ExternalScanner.(gotreesitter.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("generated SQL scanner has no checkpoint identity provider")
	}
	expectedScanner := decodeSQLIdentity(t, "7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d")
	expectedGrammar := decodeSQLIdentity(t, "4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c")
	identity, ok := provider.CheckpointIdentity()
	if !ok || !bytes.Equal(identity.Scanner, expectedScanner) || !bytes.Equal(identity.Grammar, expectedGrammar) || !bytes.Equal(identity.Grammar, grammarHash[:]) {
		t.Fatalf("generated SQL identity changed: ok=%t scanner=%x grammar=%x blob=%x", ok, identity.Scanner, identity.Grammar, grammarHash)
	}

	cLang, err := ParityCLanguage("sql")
	if err != nil {
		t.Skipf("C parser unavailable: %v", err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C SetLanguage: %v", err)
	}

	source := []byte("SELECT $$hey$$;\n")
	sourceHash := fmt.Sprintf("%x", sha256.Sum256(source))
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C parser returned no tree")
	}
	t.Cleanup(cTree.Close)
	if cTree.RootNode().HasError() {
		t.Fatalf("locked C witness has an error: %s", dumpCTree(cTree.RootNode(), 0))
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("locked C digest: %v", err)
	}

	genTree, err := gotreesitter.NewParser(genLang).Parse(source)
	if err != nil {
		t.Fatalf("generated SQL parse: %v", err)
	}
	t.Cleanup(genTree.Release)
	genInspection, err := benchfixtures.InspectGoTree(genTree.RootNode(), genLang)
	if err != nil {
		t.Fatalf("generated SQL digest: %v", err)
	}
	var genVsCErrs []string
	compareNodes(genTree.RootNode(), genLang, cTree.RootNode(), "root", &genVsCErrs)
	wantDivergence := `root: ChildCount go=3 c=2 (goType="source_file" cType="source_file" goBytes=[0-16] cBytes=[0-16])`
	if len(genVsCErrs) == 0 || genVsCErrs[0] != wantDivergence {
		t.Fatalf("generated SQL producer divergence=%v, want [%s]", genVsCErrs, wantDivergence)
	}
	if !genTree.RootNode().HasError() {
		t.Fatal("generated SQL witness unexpectedly has no error")
	}

	blobTree, err := gotreesitter.NewParser(refLang).Parse(source)
	if err != nil {
		t.Fatalf("checked-in SQL parse: %v", err)
	}
	t.Cleanup(blobTree.Release)
	blobInspection, err := benchfixtures.InspectGoTree(blobTree.RootNode(), refLang)
	if err != nil {
		t.Fatalf("checked-in SQL digest: %v", err)
	}
	var blobVsCErrs []string
	compareNodes(blobTree.RootNode(), refLang, cTree.RootNode(), "root", &blobVsCErrs)
	if len(blobVsCErrs) != 0 {
		t.Fatalf("checked-in SQL diverges from locked C: %v", blobVsCErrs)
	}
	if blobTree.RootNode().HasError() {
		t.Fatal("checked-in SQL witness has an unexpected error")
	}
	if blobInspection.SHA256 != cDigest {
		t.Fatalf("checked-in SQL digest=%s, locked C digest=%s", blobInspection.SHA256, cDigest)
	}
	var genVsBlobErrs []string
	compareGoTreesForLangs(genTree.RootNode(), genLang, blobTree.RootNode(), refLang, "root", &genVsBlobErrs)
	if len(genVsBlobErrs) == 0 {
		t.Fatal("generated SQL unexpectedly matches the checked-in SQL tree")
	}

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(genLang)
	compactParser.SetAdmissionCandidateRoute(true)
	compactTree, err := compactParser.Parse(source)
	if err != nil {
		t.Fatalf("generated SQL compact parse: %v", err)
	}
	t.Cleanup(compactTree.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	routed, fallback := routedAfter-routedBefore, fallbackAfter-fallbackBefore
	if routed != 0 || fallback != 1 {
		t.Fatalf("generated SQL compact counter delta=%d/%d, want 0/1", routed, fallback)
	}
	if got := gotreesitter.AdmissionCandidateLastFallbackReason(); got != sqlC26ahCompactFallback {
		t.Fatalf("generated SQL compact fallback reason=%q, want %q", got, sqlC26ahCompactFallback)
	}
	if got, want := compactTree.RootNode().SExpr(genLang), genTree.RootNode().SExpr(genLang); got != want {
		t.Fatalf("compact fallback changed generated tree:\n got %s\nwant %s", got, want)
	}

	t.Logf("scanner_identity=%x generated_blob_identity=%x source_sha256=%s bytes=%d generated_digest=%s checked_in_digest=%s locked_c_digest=%s generated_error=%t checked_in_error=%t compact_routed=%d compact_fallback=%d compact_reason=%q", identity.Scanner, identity.Grammar, sourceHash, len(source), genInspection.SHA256, blobInspection.SHA256, cDigest, genTree.RootNode().HasError(), blobTree.RootNode().HasError(), routed, fallback, sqlC26ahCompactFallback)
	t.Logf("generated_vs_locked_c=%s generated_vs_checked_in=%s", genVsCErrs[0], genVsBlobErrs[0])
}
