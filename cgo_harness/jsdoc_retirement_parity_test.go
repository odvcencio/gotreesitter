//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	jsdocProducerTrigger       = "/**\n * @param {string} name\n * @returns {number}\n */\n"
	jsdocProducerControl       = "/**\n * @param {string} name\n */\n"
	jsdocProducerTriggerSHA256 = "8a1683a43035994f3abf03f2f9556b96514a745018c5373ff77d3127fb27d201"
	jsdocProducerControlSHA256 = "0f4dbe6ca5d62b8c033c09ac26689c787a66298540c46b3af7a9760a7240b5ce"
)

func TestJsdocLexerSkipProvenanceLockedCParity(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	entry, ok := parityEntriesByName["jsdoc"]
	if !ok {
		t.Fatal("missing JSDoc grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("jsdoc")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		source []byte
		sha256 string
	}{
		{name: "multi_tag_trigger", source: []byte(jsdocProducerTrigger), sha256: jsdocProducerTriggerSHA256},
		{name: "single_tag_control", source: []byte(jsdocProducerControl), sha256: jsdocProducerControlSHA256},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(test.source)); got != test.sha256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.sha256)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(raw.Release)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(test.source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			candidateParser := gotreesitter.NewParser(language)
			candidateParser.SetAdmissionCandidateRoute(true)
			candidate, err := candidateParser.Parse(test.source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			t.Cleanup(candidate.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf("candidate route counters routed=%d/%d fallback=%d/%d", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
			}

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			t.Cleanup(cTree.Close)

			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatalf("inspect C deep tree: %v", err)
			}
			rawRuntime := raw.ParseRuntime()
			productionRuntime := production.ParseRuntime()
			candidateRuntime := candidate.ParseRuntime()
			if rawRuntime.NormalizationNodesRewritten != 0 || productionRuntime.NormalizationNodesRewritten != 0 || candidateRuntime.NormalizationNodesRewritten != 0 {
				t.Fatalf("JSDoc normalization rewrote nodes: raw=%d production=%d candidate=%d", rawRuntime.NormalizationNodesRewritten, productionRuntime.NormalizationNodesRewritten, candidateRuntime.NormalizationNodesRewritten)
			}
			t.Logf("witness=%s bytes=%d source_sha256=%s raw_rewrites=%d production_rewrites=%d candidate_rewrites=%d c_digest=%s", test.name, len(test.source), test.sha256, rawRuntime.NormalizationNodesRewritten, productionRuntime.NormalizationNodesRewritten, candidateRuntime.NormalizationNodesRewritten, cDigest)

			assertJsdocLockedCTreeExact(t, "raw", raw, language, cTree, cDigest)
			assertJsdocLockedCTreeExact(t, "production", production, language, cTree, cDigest)
			assertJsdocLockedCTreeExact(t, "candidate", candidate, language, cTree, cDigest)
		})
	}
}

func TestJsdocLexerSkippedPrefixMutationsLockedCParity(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	entry, ok := parityEntriesByName["jsdoc"]
	if !ok {
		t.Fatal("missing JSDoc grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("jsdoc")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		source []byte
	}{
		{name: "space_only_prefix", source: []byte("/**\n * @param {string} name\n   @returns {number}\n */\n")},
		{name: "double_decoration", source: []byte("/**\n * @param {string} name\n ** @returns {number}\n */\n")},
		{name: "content_before_tag", source: []byte("/**\n * @param {string} name\n * x @returns {number}\n */\n")},
		{name: "unterminated_type", source: []byte("/**\n * @param {string name\n * @returns {number}\n */\n")},
		{name: "missing_comment_close", source: []byte("/**\n * @param {string} name\n * @returns {number}\n")},
		{name: "crlf_prefix", source: []byte("/**\r\n * @param {string} name\r\n * @returns {number}\r\n */\r\n")},
		{name: "tabbed_prefix", source: []byte("/**\n * @param {string} name\n\t* @returns {number}\n */\n")},
		{name: "empty_decorated_line", source: []byte("/**\n * @param {string} name\n *\n * @returns {number}\n */\n")},
	}
	direct, fallback := 0, 0
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(test.source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			candidateParser := gotreesitter.NewParser(language)
			candidateParser.SetAdmissionCandidateRoute(true)
			candidate, err := candidateParser.Parse(test.source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			t.Cleanup(candidate.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			switch {
			case routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore:
				direct++
				t.Log("candidate route=direct")
			case routedAfter == routedBefore && fallbackAfter == fallbackBefore+1:
				fallback++
				t.Logf("candidate route=fallback reason=%s", gotreesitter.AdmissionCandidateLastFallbackReason())
			default:
				t.Fatalf("candidate route counters routed=%d/%d fallback=%d/%d", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
			}

			assertJsdocLockedCTreeExact(t, "mutation_production", production, language, cTree, cDigest)
			assertJsdocLockedCTreeExact(t, "mutation_candidate", candidate, language, cTree, cDigest)
		})
	}
	if direct == 0 || fallback == 0 {
		t.Fatalf("mutation routes direct=%d fallback=%d, want both exercised", direct, fallback)
	}
}

func TestErlangLeadingSkippedPrefixLockedCParity(t *testing.T) {
	entry, ok := parityEntriesByName["erlang"]
	if !ok {
		t.Fatal("missing Erlang grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("erlang")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		source []byte
	}{
		{name: "soh_digit", source: []byte("\x010")},
		{name: "du_digit", source: []byte("\x100")},
	} {
		t.Run(test.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}

			rawParser := gotreesitter.NewParser(language)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(test.source)
			if err != nil {
				t.Fatalf("raw parse: %v", err)
			}
			t.Cleanup(raw.Release)
			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(test.source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)
			candidateParser := gotreesitter.NewParser(language)
			candidateParser.SetAdmissionCandidateRoute(true)
			candidate, err := candidateParser.Parse(test.source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			t.Cleanup(candidate.Release)

			for _, route := range []struct {
				name string
				tree *gotreesitter.Tree
			}{
				{name: "raw", tree: raw}, {name: "production", tree: production}, {name: "candidate", tree: candidate},
			} {
				root := route.tree.RootNode()
				if root.HasError() {
					t.Errorf("%s HasError=true, want false", route.name)
				}
				if root.StartByte() != 1 || root.EndByte() != 2 {
					t.Errorf("%s root span=%d..%d, want 1..2", route.name, root.StartByte(), root.EndByte())
				}
				assertJsdocLockedCTreeExact(t, "erlang_"+route.name, route.tree, language, cTree, cDigest)
			}
			if cRoot := cTree.RootNode(); cRoot.HasError() || cRoot.StartByte() != 1 || cRoot.EndByte() != 2 {
				t.Fatalf("C root has_error=%t span=%d..%d, want false and 1..2", cRoot.HasError(), cRoot.StartByte(), cRoot.EndByte())
			}
		})
	}
}

func TestErlangAdmissionFuzzSeedsLockedCParity(t *testing.T) {
	runErlangAdmissionFuzzSeedsLockedCParity(t, false)
}

func runErlangAdmissionFuzzSeedsLockedCParity(t *testing.T, requireNative bool) {
	t.Helper()
	entry, ok := parityEntriesByName["erlang"]
	if !ok {
		t.Fatal("missing Erlang grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("erlang")
	if err != nil {
		t.Fatal(err)
	}
	for _, newline := range []bool{false, true} {
		t.Run(fmt.Sprintf("newline_%t", newline), func(t *testing.T) {
			source := []byte("000\"0A!A \"A\"=0:A0!)A\"0%0000")
			if newline {
				source = append(source, '\n')
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("C tree: %s", cTree.RootNode().ToSexp())
			for _, route := range []struct {
				name    string
				compact bool
			}{{"production", false}, {"compact", true}} {
				t.Run(route.name, func(t *testing.T) {
					parser := gotreesitter.NewParser(language)
					parser.SetAdmissionCandidateRoute(route.compact)
					routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
					tree, err := parser.Parse(source)
					if tree != nil {
						t.Cleanup(tree.Release)
					}
					if err != nil || tree == nil || tree.RootNode() == nil {
						t.Fatalf("parse returned no usable tree: %v", err)
					}
					routed, fallback := gotreesitter.AdmissionCandidateCounters()
					t.Logf("routed=%d fallback=%d tree=%s", routed-routedBefore, fallback-fallbackBefore, tree.RootNode().SExpr(language))
					assertJsdocLockedCTreeExact(t, "erlang_fuzz_"+route.name, tree, language, cTree, cDigest)
					if requireNative && route.compact && (routed-routedBefore != 1 || fallback != fallbackBefore) {
						t.Fatalf("compact parse must run natively: routed=%d fallback=%d", routed-routedBefore, fallback-fallbackBefore)
					}
				})
			}
		})
	}
}

func TestBashSkippedEscapeLockedCParity(t *testing.T) {
	entry, ok := parityEntriesByName["bash"]
	if !ok {
		t.Fatal("missing Bash grammar entry")
	}
	language := entry.Language()
	cLanguage, err := COracleLanguage("bash")
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("e \\ cho hi")
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C oracle returned a nil tree")
	}
	t.Cleanup(cTree.Close)
	cRoot := cTree.RootNode()
	if cRoot.HasError() {
		t.Fatalf("C root HasError=true, want false")
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}

	rawParser := gotreesitter.NewParser(language)
	raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("raw parse: %v", err)
	}
	t.Cleanup(raw.Release)
	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	t.Cleanup(production.Release)
	candidateParser := gotreesitter.NewParser(language)
	candidateParser.SetAdmissionCandidateRoute(true)
	candidate, err := candidateParser.Parse(source)
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	t.Cleanup(candidate.Release)

	for _, route := range []struct {
		name string
		tree *gotreesitter.Tree
	}{
		{name: "raw", tree: raw},
		{name: "production", tree: production},
		{name: "candidate", tree: candidate},
	} {
		if route.tree.RootNode().HasError() {
			t.Errorf("%s HasError=true, want false", route.name)
		}
		assertJsdocLockedCTreeExact(t, "bash_skipped_escape_"+route.name, route.tree, language, cTree, cDigest)
	}
}

func assertJsdocLockedCTreeExact(t *testing.T, label string, goTree *gotreesitter.Tree, goLang *gotreesitter.Language, cTree *sitter.Tree, wantDigest string) {
	t.Helper()
	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff != nil {
		t.Errorf("%s tree diverges from the locked C oracle: %+v", label, diff)
		return
	}
	if diff := firstLockedCTreeFlagDivergence(goRoot, goLang, cRoot, "/"+goRoot.Type(goLang)); diff != nil {
		t.Errorf("%s tree has a missing or error flag divergence: %v", label, diff)
		return
	}
	inspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
	if err != nil {
		t.Errorf("inspect %s Go deep tree: %v", label, err)
		return
	}
	if inspection.SHA256 != wantDigest {
		t.Errorf("%s deep digest Go=%s C=%s", label, inspection.SHA256, wantDigest)
		return
	}
	t.Logf("%s route matches locked C exactly: symbols, fields, spans, points, extras, missing/error flags, deep digest=%s", label, inspection.SHA256)
}
