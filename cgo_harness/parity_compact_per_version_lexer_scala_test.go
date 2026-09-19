//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	compactPerVersionLexerScalaSource         = "(y)->"
	compactPerVersionLexerScalaSourceSHA256   = "59b8f8c8f7f235973d07ac90f3089939e92da41ee23dcb496bc5c880f9577068"
	compactPerVersionLexerScalaCDigest        = "81dc569b1ad3d567ed158aea75fd30a388ec1f4d3e1cbdd08c29a6535bd456d3"
	compactPerVersionLexerScalaTree           = "(compilation_unit (postfix_expression (parenthesized_expression (identifier)) (operator_identifier)))"
	compactPerVersionLexerScalaGrammarCommit  = "97aead18d97708190a51d4f551ea9b05b60641c9"
	compactPerVersionLexerScalaGrammarRepo    = "https://github.com/tree-sitter/tree-sitter-scala"
	compactPerVersionLexerScalaArtifactSHA256 = "c981583f2f5fa3acc4973b79ea7c43caa46e861179fe1323f12608bff5a21459"
	compactPerVersionLexerScalaRuntimeVersion = "0.25.1"
	compactPerVersionLexerScalaRuntimeCommit  = "f5afe475deb7c0bae6407fb776c76824f717bb61"
)

// TestCompactPerVersionLexerScalaCOracle is the stage-one oracle receipt.
// Exhaustive enumeration found no shorter accepted witness over the tested
// punctuation alphabet. The owned requests consume widths two and one.
func TestCompactPerVersionLexerScalaCOracle(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	source := []byte(compactPerVersionLexerScalaSource)
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != compactPerVersionLexerScalaSourceSHA256 {
		t.Fatalf("stage-one source SHA-256=%s, want %s", got, compactPerVersionLexerScalaSourceSHA256)
	}

	identity, err := COracleIdentity("scala")
	if err != nil {
		t.Fatalf("load locked Scala C identity: %v", err)
	}
	assertCompactPerVersionLexerScalaIdentity(t, identity)

	cLanguage, err := ParityCLanguage("scala")
	if err != nil {
		t.Fatalf("load locked Scala C parser: %v", err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set locked Scala C language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked C parser returned no root")
	}
	t.Cleanup(cTree.Close)
	cRoot := cTree.RootNode()
	assertCompactPerVersionLexerScalaCRoot(t, cRoot, len(source))
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect locked Scala C tree: %v", err)
	}
	if cDigest != compactPerVersionLexerScalaCDigest {
		t.Fatalf("locked Scala C digest=%s, want %s", cDigest, compactPerVersionLexerScalaCDigest)
	}
	if got := formatCNodeSExpr(cRoot); got != compactPerVersionLexerScalaTree {
		t.Fatalf("locked Scala C tree=%q, want %q", got, compactPerVersionLexerScalaTree)
	}

	entry := grammars.DetectLanguageByName("scala")
	if entry == nil || entry.Language() == nil {
		t.Fatal("Scala Go grammar is unavailable")
	}
	goLanguage := entry.Language()
	gotreesitter.EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { gotreesitter.EnableRecoveryRuntimeTelemetry(false) })
	beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	goParser := gotreesitter.NewParser(goLanguage)
	goParser.SetAdmissionCandidateRoute(true)
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("compact candidate parse: %v", err)
	}
	if goTree == nil || goTree.RootNode() == nil {
		t.Fatal("compact candidate returned no root")
	}
	t.Cleanup(goTree.Release)
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	if routed-beforeRouted != 1 || fallback-beforeFallback != 0 {
		t.Fatalf("compact candidate counters=%d/%d, want 1/0; reason=%q",
			routed-beforeRouted, fallback-beforeFallback, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	if goTree.ParseStoppedEarly() {
		t.Fatal("compact Scala parse stopped early")
	}
	goRoot := goTree.RootNode()
	if goRoot.Type(goLanguage) != "compilation_unit" || goRoot.StartByte() != 0 || goRoot.EndByte() != uint32(len(source)) || goRoot.HasError() {
		t.Fatalf("compact Scala root type/range/error=%q/%d..%d/%t, want compilation_unit/0..%d/false",
			goRoot.Type(goLanguage), goRoot.StartByte(), goRoot.EndByte(), goRoot.HasError(), len(source))
	}
	if got := goRoot.SExpr(goLanguage); got != compactPerVersionLexerScalaTree {
		t.Fatalf("compact Scala tree=%q, want %q", got, compactPerVersionLexerScalaTree)
	}
	if diff := FirstDivergenceDumpV1(goRoot, goLanguage, cRoot); diff != nil {
		t.Fatalf("compact Scala tree diverges from locked C: %+v", *diff)
	}
	inspection, err := benchfixtures.InspectGoTree(goRoot, goLanguage)
	if err != nil {
		t.Fatalf("inspect compact Scala tree: %v", err)
	}
	if inspection.SHA256 != cDigest {
		t.Fatalf("compact Scala digest=%s, want locked C %s", inspection.SHA256, cDigest)
	}
}

func assertCompactPerVersionLexerScalaCRoot(t *testing.T, root *sitter.Node, sourceLength int) {
	t.Helper()
	if root.Kind() != "compilation_unit" || root.StartByte() != 0 || root.EndByte() != uint(sourceLength) || root.HasError() {
		t.Fatalf("locked Scala C root type/range/error=%q/%d..%d/%t, want compilation_unit/0..%d/false",
			root.Kind(), root.StartByte(), root.EndByte(), root.HasError(), sourceLength)
	}
}

func assertCompactPerVersionLexerScalaIdentity(t *testing.T, identity COracleBuildIdentity) {
	t.Helper()
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"contract", identity.Contract, COracleContractVersion},
		{"transport", identity.Transport, "cgo_parity_binding"},
		{"binding module", identity.BindingModule, COracleBindingModule},
		{"binding version", identity.BindingVersion, COracleBindingVersion},
		{"binding commit", identity.BindingCommit, COracleBindingCommit},
		{"runtime version", identity.RuntimeVersion, compactPerVersionLexerScalaRuntimeVersion},
		{"runtime commit", identity.RuntimeCommit, compactPerVersionLexerScalaRuntimeCommit},
		{"runtime linkage", identity.RuntimeLinkage, "static_cgo_test_binary"},
		{"language", identity.Language, "scala"},
		{"grammar repository", identity.GrammarRepo, compactPerVersionLexerScalaGrammarRepo},
		{"grammar commit", identity.GrammarCommit, compactPerVersionLexerScalaGrammarCommit},
		{"grammar linkage", identity.GrammarLinkage, "shared_dlopen"},
		{"grammar compile flags", identity.GrammarCompileFlags, COracleGrammarCFlags},
		{"grammar artifact SHA-256", identity.GrammarArtifactSHA256, compactPerVersionLexerScalaArtifactSHA256},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Fatalf("locked Scala C %s=%q, want %q", check.name, check.got, check.want)
		}
	}
}
