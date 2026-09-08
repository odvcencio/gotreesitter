//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestParitySwiftCleanRecoveryProbeControls(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "for-range",
			source: "func f(n: Int) -> Int {\n" +
				"  var total = 0\n" +
				"  for i in 0..<n { total += i }\n" +
				"  return total\n" +
				"}\n",
		},
		{
			name: "for-closed-range",
			source: "func f(n: Int) -> Int {\n" +
				"  var total = 0\n" +
				"  for i in 0...n { total += i }\n" +
				"  return total\n" +
				"}\n",
		},
		{
			name:   "native-ternary",
			source: "func f(x: Int) -> Int { return x > 0 ? 1 : 2 }\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runParityCase(t, parityCase{name: "swift", source: test.source}, test.name, []byte(test.source))
		})
	}
}

// swiftForInRangeLoopCountSource mirrors
// grammars.swiftForInRangeLoopCountSource: a single function containing count
// copies of the #123 for…in trailing-closure-ambiguity trigger
// (`for i in 0..<10 { }`). See that function's doc comment for why the loop
// count matters: it is the removed terminal-diagnostic-count pre-gate's own
// trigger variable.
func swiftForInRangeLoopCountSource(count int) string {
	var b strings.Builder
	b.WriteString("func manyLoops() {\n")
	for i := 0; i < count; i++ {
		b.WriteString("    for i in 0..<10 { }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// TestParitySwiftForInRangeLoopCountAgreesWithCOracle is the C-parity
// counterpart to grammars.TestSwiftForInRangeLoopCountParityWitness: for 9,
// 10, and 20 copies of the same for…in-range loop, gotreesitter's Go output
// must agree, node for node, with the locked C tree-sitter-swift oracle. The
// removed terminal-diagnostic-count pre-gates (mechanism 2) made Go return an
// ERROR tree at 10 and 20 loops while the C oracle stayed clean — this test
// is the direct proof that the fixed accept gate (swiftRecoveryProbeMatches
// LegacyRoute, parser_result_swift.go) no longer diverges from C at any of
// these counts.
func TestParitySwiftForInRangeLoopCountAgreesWithCOracle(t *testing.T) {
	for _, count := range []int{9, 10, 20} {
		source := swiftForInRangeLoopCountSource(count)
		t.Run(fmt.Sprintf("loops=%d", count), func(t *testing.T) {
			runParityCase(t, parityCase{name: "swift", source: source}, fmt.Sprintf("loops=%d", count), []byte(source))
		})
	}
}

func TestSwiftUnsafeWitnessRemainsKnownCStructuralMismatch(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_FloatingPointToString.swift"))
	if err != nil {
		t.Fatalf("read Swift unsafe witness: %v", err)
	}
	goTree, goLang, err := parseWithGo(parityCase{name: "swift"}, source, nil)
	if err != nil {
		t.Fatalf("parse Swift witness with Go: %v", err)
	}
	defer releaseGoTree(goTree)

	cLang, err := ParityCLanguage("swift")
	if err != nil {
		t.Fatalf("load locked Swift C parser: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set locked Swift C parser language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked Swift C parser returned no tree")
	}
	defer cTree.Close()

	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	if !goRoot.HasError() || !cRoot.HasError() {
		t.Fatalf("Swift unsafe witness HasError() Go=%v C=%v, want true/true", goRoot.HasError(), cRoot.HasError())
	}
	goInspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
	if err != nil {
		t.Fatalf("inspect Go Swift witness: %v", err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect C Swift witness: %v", err)
	}
	// Pin both sides, not just the C oracle's: this witness's whole point is a
	// known, tracked structural mismatch (#576, the `unsafe` expression-prefix
	// keyword), and an unpinned Go digest could silently drift to a different
	// (still-mismatching) shape without this test noticing — exactly the kind
	// of silent change the initial-only recovery probe (parser_result_swift.go)
	// must never cause. This Go digest matches
	// grammars.TestSwiftUnsafeWitnessKeepsCurrentGoTreeAcrossRecoveryProbe's
	// pinned digest for the same file.
	const wantGoDigest = "00085f672ac6595ed7182fe74369701e8547cfad2a8a5cb0326c9caf5e98e39d"
	if goInspection.SHA256 != wantGoDigest {
		t.Fatalf("Go Swift witness digest = %s, want %s", goInspection.SHA256, wantGoDigest)
	}
	const wantCDigest = "ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d"
	if cDigest != wantCDigest {
		t.Fatalf("locked C Swift witness digest = %s, want %s", cDigest, wantCDigest)
	}
	if goInspection.SHA256 == cDigest {
		t.Fatal("Swift unsafe witness reached C structural parity; ratchet the known-mismatch test")
	}
	if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff == nil {
		t.Fatal("Swift unsafe witness digest differs without a structural divergence")
	}
	t.Logf("Swift unsafe witness known C mismatch: Go=%s C=%s", goInspection.SHA256, cDigest)
}

func TestSwiftUnsafeMinimalWitnessMatchesLockedC(t *testing.T) {
	const sourceText = "let x = unsafe bar()"
	const wantSourceSHA256 = "b511d81ace2a89b05e8e5e0ca6730c10f2ac9295111dae013097c7c6be8861fe"
	const swiftGoBlobSHA256 = "be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582"
	const wantGoDigest = "c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68"
	const wantCDigest = "c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68"
	const wantSwiftGrammarCommit = "41d6e5fe811ec94229ee71771174a8cce558dfee"
	const wantCRuntimeVersion = "0.25.1"
	const wantCRuntimeCommit = "f5afe475deb7c0bae6407fb776c76824f717bb61"
	const wantCGrammarRepo = "https://github.com/alex-pinkus/tree-sitter-swift"
	const wantCArtifactSHA256 = "2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c"
	source := []byte(sourceText)
	sourceDigest := sha256.Sum256(source)
	if got := fmt.Sprintf("%x", sourceDigest); got != wantSourceSHA256 {
		t.Fatalf("Swift unsafe minimal source SHA-256 = %s, want %s", got, wantSourceSHA256)
	}
	goBlob := grammars.BlobByName("swift")
	if len(goBlob) == 0 {
		t.Fatal("Swift Go grammar blob is empty")
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(goBlob)); got != swiftGoBlobSHA256 {
		t.Fatalf("Swift Go grammar blob SHA-256 = %s, want %s", got, swiftGoBlobSHA256)
	}

	goTree, goLang, err := parseWithGo(parityCase{name: "swift"}, source, nil)
	if err != nil {
		t.Fatalf("parse Swift unsafe minimal witness with Go: %v", err)
	}
	defer releaseGoTree(goTree)
	goRoot := goTree.RootNode()
	if goRoot == nil {
		t.Fatal("Go Swift unsafe minimal witness returned no tree")
	}

	identity, err := COracleIdentity("swift")
	if err != nil {
		t.Fatalf("load Swift C oracle identity: %v", err)
	}
	if identity.GrammarCommit != wantSwiftGrammarCommit {
		t.Fatalf("Swift C grammar commit = %s, want %s", identity.GrammarCommit, wantSwiftGrammarCommit)
	}
	if identity.RuntimeVersion != wantCRuntimeVersion {
		t.Fatalf("Swift C runtime version = %s, want %s", identity.RuntimeVersion, wantCRuntimeVersion)
	}
	if identity.RuntimeCommit != wantCRuntimeCommit {
		t.Fatalf("Swift C runtime commit = %s, want %s", identity.RuntimeCommit, wantCRuntimeCommit)
	}
	if identity.GrammarRepo != wantCGrammarRepo {
		t.Fatalf("Swift C grammar repository = %s, want %s", identity.GrammarRepo, wantCGrammarRepo)
	}
	if identity.GrammarArtifactSHA256 != wantCArtifactSHA256 {
		t.Fatalf("Swift C grammar artifact SHA-256 = %s, want %s", identity.GrammarArtifactSHA256, wantCArtifactSHA256)
	}
	cLang, err := ParityCLanguage("swift")
	if err != nil {
		t.Fatalf("load locked Swift C parser: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set locked Swift C parser language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked Swift C parser returned no tree")
	}
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if !goRoot.HasError() || !cRoot.HasError() {
		t.Fatalf("Swift unsafe minimal witness HasError() Go=%v C=%v, want true/true", goRoot.HasError(), cRoot.HasError())
	}

	goInspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
	if err != nil {
		t.Fatalf("inspect Go Swift unsafe minimal witness: %v", err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect C Swift unsafe minimal witness: %v", err)
	}
	if goInspection.SHA256 != wantGoDigest {
		t.Fatalf("Go Swift unsafe minimal witness digest = %s, want %s", goInspection.SHA256, wantGoDigest)
	}
	if cDigest != wantCDigest {
		t.Fatalf("locked C Swift unsafe minimal witness digest = %s, want %s", cDigest, wantCDigest)
	}
	if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff != nil {
		t.Fatalf("Swift unsafe minimal witness still diverges from locked C: %+v", *diff)
	}
	t.Logf("SWIFT_576_MINIMAL_C_WITNESS version=1 disposition=locked_C_parity issue=#576 source_sha256=%s source_bytes=%d grammar=swift grammar_lock_commit=%s c_runtime=%s@%s c_grammar_repo=%s c_grammar_commit=%s c_grammar_artifact_sha256=%s go_blob_sha256=%s go_deep_sha256=%s c_deep_sha256=%s", wantSourceSHA256, len(source), identity.GrammarCommit, identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarRepo, identity.GrammarCommit, identity.GrammarArtifactSHA256, swiftGoBlobSHA256, goInspection.SHA256, cDigest)
}

func TestSwiftUnsafeMinimalWitnessMatchesLockedCAfterPooledRecoveryControl(t *testing.T) {
	const wantMinimalDigest = "c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68"
	control := []byte("func f(n: Int) -> Int {\n" +
		"  var total = 0\n" +
		"  for i in 0..<n { total += i }\n" +
		"  return total\n" +
		"}\n")
	minimal := []byte("let x = unsafe bar()")

	goLang := grammars.SwiftLanguage()
	pool := gotreesitter.NewParserPool(goLang)
	cLang, err := ParityCLanguage("swift")
	if err != nil {
		t.Fatalf("load locked Swift C parser: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set locked Swift C parser language: %v", err)
	}

	assertPooledParity := func(label string, source []byte) string {
		t.Helper()
		goTree, err := pool.Parse(source)
		if err != nil {
			t.Fatalf("%s pooled Go parse: %v", label, err)
		}
		if goTree == nil || goTree.RootNode() == nil {
			t.Fatalf("%s pooled Go parser returned no tree", label)
		}
		defer goTree.Release()

		cTree := cParser.Parse(source, nil)
		if cTree == nil || cTree.RootNode() == nil {
			t.Fatalf("%s locked C parser returned no tree", label)
		}
		defer cTree.Close()
		if diff := FirstDivergenceDumpV1(goTree.RootNode(), goLang, cTree.RootNode()); diff != nil {
			t.Fatalf("%s pooled Go tree diverges from locked C: %+v", label, *diff)
		}
		inspection, err := benchfixtures.InspectGoTree(goTree.RootNode(), goLang)
		if err != nil {
			t.Fatalf("inspect %s pooled Go tree: %v", label, err)
		}
		return inspection.SHA256
	}

	assertPooledParity("for-range control", control)
	if got := assertPooledParity("unsafe minimal witness", minimal); got != wantMinimalDigest {
		t.Fatalf("pooled Swift unsafe minimal digest = %s, want %s", got, wantMinimalDigest)
	}
}

func TestSwiftUnsafeLargeWitnessKeepsDigestAfterPooledRecoveryControl(t *testing.T) {
	const wantGoDigest = "00085f672ac6595ed7182fe74369701e8547cfad2a8a5cb0326c9caf5e98e39d"
	const wantCDigest = "ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d"
	control := []byte("func f(n: Int) -> Int {\n" +
		"  var total = 0\n" +
		"  for i in 0..<n { total += i }\n" +
		"  return total\n" +
		"}\n")
	large, err := os.ReadFile(filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_FloatingPointToString.swift"))
	if err != nil {
		t.Fatalf("read Swift unsafe witness: %v", err)
	}

	goLang := grammars.SwiftLanguage()
	pool := gotreesitter.NewParserPool(goLang)
	cLang, err := ParityCLanguage("swift")
	if err != nil {
		t.Fatalf("load locked Swift C parser: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set locked Swift C parser language: %v", err)
	}

	controlGo, err := pool.Parse(control)
	if err != nil {
		t.Fatalf("parse for-range control with pooled Go parser: %v", err)
	}
	controlC := cParser.Parse(control, nil)
	if controlC == nil || controlC.RootNode() == nil {
		controlGo.Release()
		t.Fatal("locked C parser returned no for-range control tree")
	}
	if diff := FirstDivergenceDumpV1(controlGo.RootNode(), goLang, controlC.RootNode()); diff != nil {
		controlC.Close()
		controlGo.Release()
		t.Fatalf("for-range control diverges from locked C: %+v", *diff)
	}
	controlC.Close()
	controlGo.Release()

	largeGo, err := pool.Parse(large)
	if err != nil {
		t.Fatalf("parse Swift unsafe witness with pooled Go parser: %v", err)
	}
	defer largeGo.Release()
	largeC := cParser.Parse(large, nil)
	if largeC == nil || largeC.RootNode() == nil {
		t.Fatal("locked C parser returned no Swift unsafe witness tree")
	}
	defer largeC.Close()
	inspection, err := benchfixtures.InspectGoTree(largeGo.RootNode(), goLang)
	if err != nil {
		t.Fatalf("inspect pooled Swift unsafe witness: %v", err)
	}
	if inspection.SHA256 != wantGoDigest {
		t.Fatalf("pooled Swift unsafe witness digest = %s, want %s", inspection.SHA256, wantGoDigest)
	}
	cDigest, err := COracleDeepDigest(largeC)
	if err != nil {
		t.Fatalf("inspect locked C Swift unsafe witness: %v", err)
	}
	if cDigest != wantCDigest {
		t.Fatalf("locked C Swift unsafe witness digest = %s, want %s", cDigest, wantCDigest)
	}
	if diff := FirstDivergenceDumpV1(largeGo.RootNode(), goLang, largeC.RootNode()); diff == nil {
		t.Fatal("Swift unsafe witness reached C structural parity; ratchet the known-mismatch test")
	}
}
