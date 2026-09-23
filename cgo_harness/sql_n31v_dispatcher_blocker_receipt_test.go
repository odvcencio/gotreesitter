//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	gots "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	sqlN31vSourceSHA        = "c2826e3d8fc7ec0a99c4e2ecc37514a3d6bd0a4aa60b4ff65dc2382629d1b11e"
	sqlN31vRawDigest        = "8c4095130f0da24ad8d3ce0dd9c56becfe3e70e4eaed118ef132933b2f492848"
	sqlN31vNormalizedDigest = "9f256e76a2192f6e3f6d98bf57d773eb02d362ca525efe0caec163567a272bd1"
	sqlN31vCArtifactSHA     = "f13ad13cdc0f748a362e50f92e06f685736905db0bbbdbd2b3dffd0307232ec2"
	sqlN31vCompactFallback  = "compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token"
	// sqlN31vIncrementalReusedSubtrees and sqlN31vIncrementalReusedBytes pin
	// the byte-reuse bookkeeping for the unrouted (default-route) incremental
	// parse below (incParser sets no admission-route override). Before the
	// compact route's default flipped to off (admission_switch.go), that
	// call went through the compact-route-aware incremental path and reused
	// nothing for this witness. With production as the default, it goes
	// through production's own incremental engine instead, which reuses one
	// 6-byte subtree here that the compact path did not. The resulting tree
	// is unchanged by that reuse: digest, HasError, the C-oracle divergence
	// check, and the dispatch receipt below are still pinned to their exact
	// prior values and still match the locked C oracle, so this is a real,
	// verified efficiency gain from the route change, not a correctness
	// regression -- update these two constants, not the unrelated shape
	// assertions, if this ever needs to move again.
	sqlN31vIncrementalReusedSubtrees = 1
	sqlN31vIncrementalReusedBytes    = 6
)

func TestSQLN31vDispatcherBlockerRoutes(t *testing.T) {
	gots.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	source := []byte("SELECT a, b,\n")
	base := []byte("SELECT a, b")
	goLang := grammars.SqlLanguage()
	if goLang == nil || goLang.Name != "sql" {
		t.Fatalf("language=%v, want sql", goLang)
	}
	cLang, err := COracleLanguage("sql")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("sql")
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := goLang.ExternalScanner.(gots.ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("SQL scanner has no checkpoint identity provider")
	}
	scannerIdentity, ok := provider.CheckpointIdentity()
	if !ok {
		t.Fatal("SQL scanner did not provide checkpoint identity")
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != sqlN31vSourceSHA {
		t.Fatalf("source SHA-256=%s, want %s", got, sqlN31vSourceSHA)
	}
	if got, want := fmt.Sprintf("%x", scannerIdentity.Scanner), "7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d"; got != want {
		t.Fatalf("SQL scanner identity=%s, want %s", got, want)
	}
	if got, want := fmt.Sprintf("%x", scannerIdentity.Grammar), "e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268"; got != want {
		t.Fatalf("SQL grammar identity=%s, want %s", got, want)
	}
	if identity.GrammarArtifactSHA256 != sqlN31vCArtifactSHA {
		t.Fatalf("locked-C SQL artifact=%s, want %s", identity.GrammarArtifactSHA256, sqlN31vCArtifactSHA)
	}
	t.Logf("grammar_lock_sha256=%s blob_sha256=e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268 scanner_identity=%x grammar_identity=%x c_contract=%s c_runtime=%s@%s c_grammar=%s@%s c_artifact_sha256=%s", currentGrammarLockSHA256(t), scannerIdentity.Scanner, scannerIdentity.Grammar, identity.Contract, identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarRepo, identity.GrammarCommit, identity.GrammarArtifactSHA256)
	cTree := sqlN31vCTree(t, cLang, source)
	defer cTree.Close()
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	if cDigest != sqlN31vNormalizedDigest || !cTree.RootNode().HasError() || cTree.RootNode().ChildCount() != 1 {
		t.Fatalf("locked-C receipt changed: digest=%s error=%t children=%d", cDigest, cTree.RootNode().HasError(), cTree.RootNode().ChildCount())
	}
	t.Logf("source_sha256=%x bytes=%d c_digest=%s c_error=%t c_children=%d", sha256.Sum256(source), len(source), cDigest, cTree.RootNode().HasError(), cTree.RootNode().ChildCount())

	parse := func(route string, candidate bool, fn func(*gots.Parser) (*gots.Tree, error)) {
		parser := gots.NewParser(goLang)
		parser.SetAdmissionCandidateRoute(candidate)
		tree, err := fn(parser)
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Release()
		inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), goLang)
		if err != nil {
			t.Fatal(err)
		}
		diff := FirstDivergenceDumpV1(tree.RootNode(), goLang, cTree.RootNode())
		wantDigest := sqlN31vNormalizedDigest
		wantDiff := (*DumpV1Divergence)(nil)
		wantDispatch := "1/1/10/7"
		if route == "raw" {
			wantDigest = sqlN31vRawDigest
			wantDiff = &DumpV1Divergence{Path: "/source_file", Category: "shape", GoValue: "children=2", CValue: "children=1"}
			wantDispatch = "none"
		}
		if inspection.SHA256 != wantDigest || tree.RootNode().HasError() != true || (diff == nil) != (wantDiff == nil) || (diff != nil && *diff != *wantDiff) || sqlN31vDispatch(tree) != wantDispatch {
			t.Fatalf("%s receipt changed: digest=%s error=%t divergence=%+v dispatch=%s", route, inspection.SHA256, tree.RootNode().HasError(), diff, sqlN31vDispatch(tree))
		}
		t.Logf("route=%s digest=%s c_digest=%s error=%t c_error=%t children=%d divergence=%+v dispatch=%s rewrites=%d runtime=%s", route, inspection.SHA256, cDigest, tree.RootNode().HasError(), cTree.RootNode().HasError(), tree.RootNode().ChildCount(), diff, sqlN31vDispatch(tree), tree.ParseRuntime().NormalizationNodesRewritten, tree.ParseRuntime().Summary())
	}
	parse("raw", false, func(p *gots.Parser) (*gots.Tree, error) {
		return p.ParseNoResultCompatibilityBenchmarkOnly(source)
	})
	parse("production", false, func(p *gots.Parser) (*gots.Tree, error) {
		return p.Parse(source)
	})
	beforeRouted, beforeFallback := gots.AdmissionCandidateCounters()
	parse("compact", true, func(p *gots.Parser) (*gots.Tree, error) {
		return p.Parse(source)
	})
	afterRouted, afterFallback := gots.AdmissionCandidateCounters()
	if afterRouted-beforeRouted != 0 || afterFallback-beforeFallback != 1 || gots.AdmissionCandidateLastFallbackReason() != sqlN31vCompactFallback {
		t.Fatalf("compact receipt changed: routed=%d fallback=%d reason=%q", afterRouted-beforeRouted, afterFallback-beforeFallback, gots.AdmissionCandidateLastFallbackReason())
	}
	t.Logf("compact routed_delta=%d fallback_delta=%d reason=%q", afterRouted-beforeRouted, afterFallback-beforeFallback, gots.AdmissionCandidateLastFallbackReason())

	forestParser := gots.NewParser(goLang)
	forest, forestOK := forestParser.ParseForestExperimental(source)
	if forestOK && forest != nil {
		forest.Release()
		t.Fatal("forest route accepted a recovery witness")
	} else {
		t.Logf("route=forest accepted=false")
	}

	incParser := gots.NewParser(goLang)
	oldTree, err := incParser.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	oldTree.Edit(gots.InputEdit{StartByte: uint32(len(base)), OldEndByte: uint32(len(base)), NewEndByte: uint32(len(source)), StartPoint: sqlN31vPoint(base), OldEndPoint: sqlN31vPoint(base), NewEndPoint: sqlN31vPoint(source)})
	incremental, profile, err := incParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer incremental.Release()
	inspection, err := benchfixtures.InspectGoTree(incremental.RootNode(), goLang)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != sqlN31vNormalizedDigest || !incremental.RootNode().HasError() || FirstDivergenceDumpV1(incremental.RootNode(), goLang, cTree.RootNode()) != nil || sqlN31vDispatch(incremental) != "1/1/10/7" || profile.ReusedSubtrees != sqlN31vIncrementalReusedSubtrees || profile.ReusedBytes != sqlN31vIncrementalReusedBytes {
		t.Fatalf("incremental receipt changed: digest=%s error=%t divergence=%+v dispatch=%s profile=%+v", inspection.SHA256, incremental.RootNode().HasError(), FirstDivergenceDumpV1(incremental.RootNode(), goLang, cTree.RootNode()), sqlN31vDispatch(incremental), profile)
	}
	t.Logf("route=incremental digest=%s c_digest=%s error=%t divergence=%+v dispatch=%s rewrites=%d profile=%+v runtime=%s", inspection.SHA256, cDigest, incremental.RootNode().HasError(), FirstDivergenceDumpV1(incremental.RootNode(), goLang, cTree.RootNode()), sqlN31vDispatch(incremental), incremental.ParseRuntime().NormalizationNodesRewritten, profile, incremental.ParseRuntime().Summary())
}

func TestSQLN31vDispatcherBlockerReceiptDocument(t *testing.T) {
	doc, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	changelog, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"## 2026-08-24 SQL dispatcher blocker receipt",
		"Status: `KEEP LIVE / NO-GO`. Keep `dispatch.sql` live.",
		"ac90e46ace3c4ac6fb6bbc9f0897e449c949cfad",
		sqlN31vSourceSHA,
		sqlN31vRawDigest,
		sqlN31vNormalizedDigest,
		"The compact route falls back.",
		"The forest route declines.",
		"The container inspection record contains `GOMAXPROCS=1`.",
		"No parser or registry change ships.",
	} {
		if !strings.Contains(string(doc), marker) {
			t.Fatalf("retirement document lacks marker %q", marker)
		}
	}
	for _, marker := range []string{
		"Recorded the SQL dispatcher blocker",
		"Keep `dispatch.sql` live",
		"No parser or registry change ships.",
	} {
		if !strings.Contains(string(changelog), marker) {
			t.Fatalf("changelog lacks marker %q", marker)
		}
	}
}

func sqlN31vDispatch(tree *gots.Tree) string {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return "none"
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == "dispatch.sql" {
			return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
	return "none"
}

func sqlN31vPoint(source []byte) gots.Point {
	var point gots.Point
	for _, b := range source {
		if b == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

func sqlN31vCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C parser returned no root")
	}
	return tree
}
