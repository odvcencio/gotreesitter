//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type goNextWitness struct {
	name           string
	file           string
	source         []byte
	sourceSHA      string
	rawDigest      string
	routeDigest    string
	cDigest        string
	compactMode    string
	forestAccepted bool
	production     goNextPassSet
	compact        goNextPassSet
	forest         goNextPassSet
	incremental    goNextPassSet
	rawDiff        *goNextDivergenceExpectation
	routeDiff      *goNextDivergenceExpectation
}

type goNextPassExpectation struct {
	recorded  bool
	checked   uint64
	run       uint64
	rewritten uint64
}

type goNextPassSet struct {
	source, compat, newMake goNextPassExpectation
}

type goNextDivergenceExpectation struct {
	path, category, goValue, cValue string
}

// TestGoNextLiveArmProbe records all live Go dispatcher subpasses on every
// required route. It keeps the three producer gaps visible before retirement.
func TestGoNextLiveArmProbe(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.GoLanguage()
	if language == nil {
		t.Fatal("Go language is unavailable")
	}
	if got := goNextHashFile(t, "../grammars/languages.lock"); got != "9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb" {
		t.Fatalf("grammar lock SHA-256=%s", got)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(grammars.BlobByName("go"))); got != "9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08" {
		t.Fatalf("Go grammar blob SHA-256=%s", got)
	}
	cLanguage, err := COracleLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("go")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("grammar=go grammar_lock_sha256=9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb blob_sha256=9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08 c_contract=%s c_runtime=%s c_runtime_commit=%s c_grammar_artifact_sha256=%s", identity.Contract, identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarArtifactSHA256)

	witnesses := []goNextWitness{
		{
			name:        "tracked-go-sample",
			file:        "../cgo_harness/corpus_structural/go_sample.go",
			sourceSHA:   "262c7b8359a364b56db8549f65f4f2e01ed43de096e607dd05f50b94bd45efb0",
			rawDigest:   "833b5685c629a0224ed220aceb98679ee7a81197c7c9d0e3d0516dfed1b9cdf9",
			routeDigest: "833b5685c629a0224ed220aceb98679ee7a81197c7c9d0e3d0516dfed1b9cdf9",
			cDigest:     "833b5685c629a0224ed220aceb98679ee7a81197c7c9d0e3d0516dfed1b9cdf9",
			compactMode: "accepted", forestAccepted: true,
			production: goNextPassSetAll(0), compact: goNextNoPass(), forest: goNextPassSetAll(0), incremental: goNextPassSetAll(0),
		},
		{
			name:        "clean-semicolon-sibling",
			source:      []byte("package p\n\nconst (\n\tA = 1\n\tB = 2\n)\n\nfunc F() int {\n\tswitch {\n\tcase true:\n\t\treturn 1\n\tdefault:\n\t\treturn 0\n\t}\n}\n"),
			sourceSHA:   "60105006a5b8ff9547a9e9ed9f68562eb4e63652ad54166e3c2e6d1ab15f62d5",
			rawDigest:   "e0570a37201e4cf73162dd11b4508aaa8c169706af8f5cd87462b5cd12e4207f",
			routeDigest: "e0570a37201e4cf73162dd11b4508aaa8c169706af8f5cd87462b5cd12e4207f",
			cDigest:     "e0570a37201e4cf73162dd11b4508aaa8c169706af8f5cd87462b5cd12e4207f",
			compactMode: "accepted", forestAccepted: true,
			production: goNextPassSetAll(0), compact: goNextNoPass(), forest: goNextPassSetAll(0), incremental: goNextPassSetAll(0),
		},
		{
			name:        "malformed-recovery",
			source:      []byte("package p\n\nfunc F() {\n\tif ready {\n\t\treturn\n"),
			sourceSHA:   "91a32447779c323e63d839081cf35f7dccec60106b0fef36e3f7331797e2f56a",
			rawDigest:   "9c04d41835d5bff3fcd1ca7db2f1279508819fcca287942813236091b0c3c088",
			routeDigest: "9c04d41835d5bff3fcd1ca7db2f1279508819fcca287942813236091b0c3c088",
			cDigest:     "9c04d41835d5bff3fcd1ca7db2f1279508819fcca287942813236091b0c3c088",
			compactMode: "fallback", forestAccepted: false,
			production: goNextPassSetAll(0), compact: goNextPassSetAll(0), forest: goNextNoPass(), incremental: goNextPassSetAll(0),
		},
		{
			name:        "no-op-control",
			source:      []byte("package p\n\nvar _ = 1\n"),
			sourceSHA:   "0681dbe5037a6d1246c250b33718e27526dbdd755b5c53029cbd22039294bf95",
			rawDigest:   "2b5a222aef26944bb46ea6bdc1bbc17da9af330252dc3317610965e7feb514e5",
			routeDigest: "2b5a222aef26944bb46ea6bdc1bbc17da9af330252dc3317610965e7feb514e5",
			cDigest:     "2b5a222aef26944bb46ea6bdc1bbc17da9af330252dc3317610965e7feb514e5",
			compactMode: "accepted", forestAccepted: true,
			production: goNextPassSetAll(0), compact: goNextNoPass(), forest: goNextPassSetAll(0), incremental: goNextPassSetAll(0),
		},
		{
			name:        "new-make-types",
			source:      []byte("package p\n\nfunc F() {\n\t_ = new(pkg.Type)\n\t_ = new(*T)\n\t_ = make(pkg.Type, 0)\n}\n"),
			sourceSHA:   "9fc0b1f058672dbf42a643eab2c986e03f71dfc306a3ff0f2835cb9f89575710",
			rawDigest:   "322f6d6db609c49fb8b09a52d2d9e9ae0853c56d2349bc89a9af1df8c6ca4374",
			routeDigest: "329fac609c64a2e338b8e0532b63dd6a38e3f1b6da311dab68e0eb3576a664a6",
			cDigest:     "329fac609c64a2e338b8e0532b63dd6a38e3f1b6da311dab68e0eb3576a664a6",
			compactMode: "fallback", forestAccepted: true,
			production:  goNextPassSet{source: goNextPass(0), compat: goNextPass(0), newMake: goNextPass(9)},
			compact:     goNextPassSet{source: goNextPass(0), compat: goNextPass(0), newMake: goNextPass(9)},
			forest:      goNextPassSet{source: goNextPass(0), compat: goNextPass(0), newMake: goNextPass(9)},
			incremental: goNextPassSet{source: goNextPass(0), compat: goNextPass(0), newMake: goNextPass(9)},
			rawDiff:     goNextDivergence("/source_file/function_declaration[1]/block[3]/statement_list[1]/assignment_statement[0]/expression_list[2]/call_expression[0]/argument_list[1]/selector_expression[1]", "type", "selector_expression", "qualified_type"),
		},
		{
			name:        "recovery-control",
			source:      []byte("package p\n\nfunc F() {\n\tvar value = []int{1, 2, 3}\n\t_ = value[1]\n}\n"),
			sourceSHA:   "4b7b7498c02ed4e5c6f7f005b350c7e36b7ab33b02701fb10a4d2d76aef056aa",
			rawDigest:   "3c74ff188c1085bb06dd82824347a8e4ff63760b5bfd51dff01663625f53927d",
			routeDigest: "3c74ff188c1085bb06dd82824347a8e4ff63760b5bfd51dff01663625f53927d",
			cDigest:     "3c74ff188c1085bb06dd82824347a8e4ff63760b5bfd51dff01663625f53927d",
			compactMode: "accepted", forestAccepted: true,
			production: goNextPassSetAll(0), compact: goNextNoPass(), forest: goNextPassSetAll(0), incremental: goNextPassSetAll(0),
		},
	}
	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source
			if source == nil {
				var err error
				source, err = os.ReadFile(witness.file)
				if err != nil {
					t.Fatal(err)
				}
			}
			goNextRunRoutes(t, language, cLanguage, witness, source)
		})
	}
	goNextRunIncludedRanges(t, language, cLanguage)
}

func goNextRunRoutes(t *testing.T, language *gotreesitter.Language, cLanguage *sitter.Language, witness goNextWitness, source []byte) {
	t.Helper()
	name := witness.name
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != witness.sourceSHA {
		t.Fatalf("source SHA-256=%s, want %s", got, witness.sourceSHA)
	}
	cTree := goNextCTree(t, cLanguage, source)
	defer cTree.Close()
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	if cDigest != witness.cDigest {
		t.Fatalf("locked-C digest=%s, want %s", cDigest, witness.cDigest)
	}
	t.Logf("witness=%s bytes=%d source_sha256=%x c_digest=%s c_error_root=%t", name, len(source), sha256.Sum256(source), cDigest, cTree.RootNode().HasError())

	rawParser := gotreesitter.NewParser(language)
	rawParser.SetAdmissionCandidateRoute(false)
	raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Release()
	goNextCheckRoute(t, "raw", raw, language, cTree, cDigest, witness.rawDigest, witness.rawDiff, goNextNoPass())

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer production.Release()
	goNextCheckRoute(t, "production", production, language, cTree, cDigest, witness.routeDigest, witness.routeDiff, witness.production)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(language)
	compactParser.SetAdmissionCandidateRoute(true)
	compact, err := compactParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer compact.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	compactMode := goNextCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter)
	if compactMode != witness.compactMode {
		t.Fatalf("compact mode=%s, want %s", compactMode, witness.compactMode)
	}
	t.Logf("witness=%s route=compact mode=%s routed=%d/%d->%d/%d fallback_reason=%q", name, compactMode, routedBefore, fallbackBefore, routedAfter, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason())
	goNextCheckRoute(t, "compact", compact, language, cTree, cDigest, witness.routeDigest, witness.routeDiff, witness.compact)

	forestParser := gotreesitter.NewParser(language)
	forest, forestOK := forestParser.ParseForestExperimental(source)
	if forestOK && forest != nil {
		if !witness.forestAccepted {
			t.Fatalf("forest accepted, want decline")
		}
		defer forest.Release()
		goNextCheckRoute(t, "forest", forest, language, cTree, cDigest, witness.routeDigest, witness.routeDiff, witness.forest)
	} else {
		if witness.forestAccepted {
			t.Fatalf("forest declined, want acceptance")
		}
		if forest != nil {
			forest.Release()
			t.Fatal("forest declined with a non-nil tree")
		}
		t.Logf("witness=%s route=forest mode=declined", name)
	}

	incrementalParser := gotreesitter.NewParser(language)
	incrementalParser.SetAdmissionCandidateRoute(false)
	base := bytes.TrimSuffix(source, []byte{'\n'})
	oldTree, err := incrementalParser.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(len(base)),
		OldEndByte:  uint32(len(base)),
		NewEndByte:  uint32(len(source)),
		StartPoint:  goNextPointAtByte(base),
		OldEndPoint: goNextPointAtByte(base),
		NewEndPoint: goNextPointAtByte(source),
	})
	incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer incremental.Release()
	goNextCheckRoute(t, "incremental", incremental, language, cTree, cDigest, witness.routeDigest, witness.routeDiff, witness.incremental)
	t.Logf("witness=%s incremental reuse old_tree=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d", name, profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes)
	if !profile.OldTreeReuseRoute || profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("incremental reuse changed: old_tree=%t unsupported=%t reason=%q subtrees=%d bytes=%d", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes)
	}
}

func goNextRunIncludedRanges(t *testing.T, language *gotreesitter.Language, cLanguage *sitter.Language) {
	t.Helper()
	source, err := os.ReadFile("../testdata/included_ranges/go_two_fences.go")
	if err != nil {
		t.Fatal(err)
	}
	spans := [][2]int{{26, 150}, {203, 276}}
	goRanges := make([]gotreesitter.Range, 0, len(spans))
	cRanges := make([]sitter.Range, 0, len(spans))
	for _, span := range spans {
		sr, sc := goNextPointAt(source, span[0])
		er, ec := goNextPointAt(source, span[1])
		goRanges = append(goRanges, gotreesitter.Range{StartByte: uint32(span[0]), EndByte: uint32(span[1]), StartPoint: gotreesitter.Point{Row: sr, Column: sc}, EndPoint: gotreesitter.Point{Row: er, Column: ec}})
		cRanges = append(cRanges, sitter.Range{StartByte: uint(span[0]), EndByte: uint(span[1]), StartPoint: sitter.Point{Row: uint(sr), Column: uint(sc)}, EndPoint: sitter.Point{Row: uint(er), Column: uint(ec)}})
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	if err := cParser.SetIncludedRanges(cRanges); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("included-ranges C parser returned no tree")
	}
	t.Cleanup(cTree.Close)
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	parser := gotreesitter.NewParser(language)
	parser.SetIncludedRanges(goRanges)
	goTree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(goTree.Release)
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != "978465a10f7d814c2183eed2e4ceedfd1dd467efd780210ae39be8e170f3b01b" {
		t.Fatalf("included-ranges source SHA-256=%s", got)
	}
	if cDigest != "9c8e5bb506bb345a577beb351f7b9230cca5e2e02cc4fd619e21f607657f290f" {
		t.Fatalf("included-ranges locked-C digest=%s", cDigest)
	}
	goNextCheckRoute(t, "included-ranges", goTree, language, cTree, cDigest, "8231fb8782e5699708e183619b68e1d030ab09e60de99ac52e3c092ffc56d59c", goNextDivergence("/source_file", "range", "0:0-22:0 @0..276", "0:26-22:0 @26..276"), goNextPassSet{source: goNextPass(0), compat: goNextPass(25), newMake: goNextPass(0)})
	if got := goTree.RootNode().Type(language); got != "source_file" {
		t.Fatalf("included-ranges Go root=%q", got)
	}
	if got := cTree.RootNode().Kind(); got != "source_file" {
		t.Fatalf("included-ranges C root=%q", got)
	}
	if got := goTree.RootNode().ChildCount(); got != 10 {
		t.Fatalf("included-ranges Go children=%d", got)
	}
	if got := cTree.RootNode().ChildCount(); got != 7 {
		t.Fatalf("included-ranges C children=%d", got)
	}
	t.Logf("included-ranges source_sha256=%x c_root=%s/%d..%d/%d go_root=%s/%d..%d/%d", sha256.Sum256(source), cTree.RootNode().Kind(), cTree.RootNode().StartByte(), cTree.RootNode().EndByte(), cTree.RootNode().ChildCount(), goTree.RootNode().Type(language), goTree.RootNode().StartByte(), goTree.RootNode().EndByte(), goTree.RootNode().ChildCount())
}

func goNextCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C parser returned no tree")
	}
	return tree
}

func goNextCheckRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest, wantDigest string, wantDiff *goNextDivergenceExpectation, wantPass goNextPassSet) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s route returned no tree", route)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	t.Logf("route=%s error_root=%t digest=%s c_digest=%s exact=%t native_authoritative=%t first_divergence=%+v passes=%s", route, tree.RootNode().HasError(), inspection.SHA256, cDigest, inspection.SHA256 == cDigest && diff == nil, tree.ParseRuntime().NativeRecoveredStructureAuthoritative, diff, goNextPassSummary(tree))
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	goNextCheckDivergence(t, route, diff, wantDiff)
	if tree.ParseRuntime().NativeRecoveredStructureAuthoritative {
		t.Fatalf("%s native_authoritative=true", route)
	}
	goNextCheckPass(t, route, tree, wantPass)
}

func goNextCheckDivergence(t *testing.T, route string, got *DumpV1Divergence, want *goNextDivergenceExpectation) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s divergence=%+v, want nil", route, got)
		}
		return
	}
	if got == nil || got.Path != want.path || got.Category != want.category || got.GoValue != want.goValue || got.CValue != want.cValue {
		t.Fatalf("%s divergence=%+v, want %+v", route, got, want)
	}
}

func goNextCheckPass(t *testing.T, route string, tree *gotreesitter.Tree, want goNextPassSet) {
	t.Helper()
	var found map[string]gotreesitter.NormalizationPassRuntime
	if runtime := tree.ParseRuntime(); runtime.NormalizationPasses != nil {
		found = make(map[string]gotreesitter.NormalizationPassRuntime)
		for _, pass := range *runtime.NormalizationPasses {
			if strings.HasPrefix(pass.Name, "dispatch.go.") {
				found[pass.Name] = pass
			}
		}
	}
	expected := map[string]goNextPassExpectation{
		"dispatch.go.source-file-root": want.source,
		"dispatch.go.compat-walk":      want.compat,
		"dispatch.go.new-make-type":    want.newMake,
	}
	recorded := want.source.recorded || want.compat.recorded || want.newMake.recorded
	if recorded && len(found) != len(expected) {
		t.Fatalf("%s recorded %d Go dispatcher passes, want %d", route, len(found), len(expected))
	}
	if !recorded && len(found) != 0 {
		t.Fatalf("%s recorded unexpected Go dispatcher passes: %v", route, found)
	}
	for _, name := range []string{"dispatch.go.source-file-root", "dispatch.go.compat-walk", "dispatch.go.new-make-type"} {
		pass, ok := found[name]
		passWant := expected[name]
		if !passWant.recorded {
			if ok {
				t.Fatalf("%s recorded %s, want none", route, name)
			}
			continue
		}
		if !ok {
			t.Fatalf("%s missing %s", route, name)
		}
		if pass.Checked != passWant.checked || pass.Run != passWant.run || pass.NodesRewritten != passWant.rewritten {
			t.Fatalf("%s %s checked/run/rewritten=%d/%d/%d, want %d/%d/%d; visited=%d is diagnostic", route, name, pass.Checked, pass.Run, pass.NodesRewritten, passWant.checked, passWant.run, passWant.rewritten, pass.NodesVisited)
		}
	}
}

func goNextPassSummary(tree *gotreesitter.Tree) string {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return "none"
	}
	parts := make([]string, 0, len(*tree.ParseRuntime().NormalizationPasses))
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if strings.HasPrefix(pass.Name, "dispatch.go.") {
			parts = append(parts, fmt.Sprintf("%s=%d/%d/%d/%d", pass.Name, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten))
		}
	}
	return strings.Join(parts, ",")
}

func goNextCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter uint64) string {
	switch {
	case routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore:
		return "accepted"
	case routedAfter == routedBefore && fallbackAfter == fallbackBefore+1:
		return "fallback"
	default:
		return fmt.Sprintf("invalid=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
	}
}

func goNextPass(rewritten uint64) goNextPassExpectation {
	return goNextPassExpectation{recorded: true, checked: 1, run: 1, rewritten: rewritten}
}

func goNextPassSetAll(rewritten uint64) goNextPassSet {
	pass := goNextPass(rewritten)
	return goNextPassSet{source: pass, compat: pass, newMake: pass}
}

func goNextNoPass() goNextPassSet { return goNextPassSet{} }

func goNextDivergence(path, category, goValue, cValue string) *goNextDivergenceExpectation {
	return &goNextDivergenceExpectation{path: path, category: category, goValue: goValue, cValue: cValue}
}

func goNextHashFile(t *testing.T, path string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(source))
}

func goNextPointAt(source []byte, off int) (uint32, uint32) {
	var row, column uint32
	for _, value := range source[:off] {
		if value == '\n' {
			row++
			column = 0
		} else {
			column++
		}
	}
	return row, column
}

func goNextPointAtByte(source []byte) gotreesitter.Point {
	row, column := goNextPointAt(source, len(source))
	return gotreesitter.Point{Row: row, Column: column}
}

// TestGoNextLiveArmReceiptDocument guards the durable Go blocker receipt.
func TestGoNextLiveArmReceiptDocument(t *testing.T) {
	doc, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	changelog, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(doc)), " ")
	for _, marker := range []string{
		"## 2026-08-24 Go dispatcher blocker receipt",
		"Status: `KEEP LIVE / NO-GO`. Keep `dispatch.go` live.",
		"The evidence base is `929609ccde78b0c9f4e57cf2225e0ae1204149cb`. The publication base is `2c533f5c19f5f7ab9b586cd8454f0cdc4ece014b`.",
		"The A0 (authenticated dispatcher census) manifest has 14 languages, 42 files, and 14 receipts. It has no Go entry.",
		"The tracked census has seven fixtures. Its Go fixture is",
		"262c7b8359a364b56db8549f65f4f2e01ed43de096e607dd05f50b94bd45efb0",
		"The focused receipt covers six tracked or pinned witnesses.",
		"Visited counts remain diagnostic.",
		"The compact guard accepts only `routed+1/fallback` unchanged, or routed unchanged and `fallback+1`.",
		"Every Go tree reports `NativeRecoveredStructureAuthoritative=false`.",
		"Go uses range `0..276` with 10 children. C uses range `26..276` with seven children.",
		"The route receipt is under `/tmp/gts-n31j-go-rebased-artifacts/20260823T112605Z-final-rebased-ratchet`.",
		"aeb53730c138395ef4618d7c1cfee805c720908dacc1cf8cd63248c72ae2d1d8",
		"The document guard receipt is under `/tmp/gts-n31j-go-rebased-artifacts/20260823T112634Z-final-rebased-document`.",
		"15084a11ddaf6f0628919623a371ed1b39a3156ac817e4e7f008a2cf7ed2eaf2",
		"No safe shared producer invariant was identified.",
		"Keep `dispatch.go` live until a producer emits exact C output for every authenticated witness and route.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("Go blocker receipt lacks marker %q", marker)
		}
	}
	for _, marker := range []string{
		"Recorded the N31j Go dispatcher blocker at evidence base",
		"929609ccde78b0c9f4e57cf2225e0ae1204149cb",
		"2c533f5c19f5f7ab9b586cd8454f0cdc4ece014b",
		"Keep `dispatch.go` live.",
		"Ship no parser, registry, or production change.",
	} {
		if !strings.Contains(string(changelog), marker) {
			t.Fatalf("Go changelog lacks marker %q", marker)
		}
	}
}
