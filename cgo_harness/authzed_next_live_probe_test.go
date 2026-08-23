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

// TestAuthzedNextLiveArmLockedCRoutes records the Authzed arm before a
// retirement decision. The custom lexer limits the raw and forest receipts.
func TestAuthzedNextLiveArmLockedCRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")

	language := grammars.AuthzedLanguage()
	cLanguage, err := COracleLanguage("authzed")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []struct {
		name string
		path string
		src  []byte
	}{
		{name: "a0-small-localimport-with-quotes", path: filepath.Join("..", "testdata", "dispatcher_census_a0", "authzed", "small__localimport_with_quotes_in_quotes.zed")},
		{name: "a0-small-doccomments", path: filepath.Join("..", "testdata", "dispatcher_census_a0", "authzed", "small__doccomments.zed")},
		{name: "a0-large-superlarge", path: filepath.Join("..", "testdata", "dispatcher_census_a0", "authzed", "large__superlarge.zed")},
		{
			name: "clean-basic-definition",
			src: []byte("definition user {}\n\n" +
				"definition document {\n" +
				"  relation viewer: user\n" +
				"  permission view = viewer\n" +
				"}\n"),
		},
		{
			name: "clean-single-quoted-caveat",
			src: []byte("definition another {}\n\n" +
				"caveat somecaveat(somecondition uint, somebool bool, somestring string) {\n" +
				"  somecondition == 42 && somebool && somestring == 'hello'\n" +
				"}\n\n" +
				"definition user {}\n"),
		},
		{
			name: "clean-associativity-incremental",
			src: []byte("definition resource {\n" +
				"  permission union = a + b + c\n" +
				"  permission exclusion = a - b - c\n" +
				"  permission intersection = a & b & c\n" +
				"}\n"),
		},
		{
			name: "control-plain-definition",
			src:  []byte("definition user {}\n"),
		},
		{
			name: "malformed-unsupported-use",
			src: []byte("use import\n\n" +
				"import \"subjects.zed\"\n\n" +
				"definition resource {\n" +
				"  relation viewer: user\n" +
				"  permission view = viewer\n" +
				"}\n"),
		},
		{
			name: "malformed-permission-method",
			src: []byte("definition resource {\n" +
				"  permission view = a.foo(bar)\n" +
				"}\n"),
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.src
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(witness.path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if len(source) == 0 {
				t.Fatal("empty Authzed witness")
			}

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, rawErr := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if rawErr != nil {
				t.Logf("route=raw status=unavailable reason=%q backend=custom-authzed-token-source", rawErr)
			} else {
				t.Cleanup(raw.Release)
				t.Logf("route=raw status=available %s", authzedProbeReceipt(raw, language, cTree, cDigest))
			}

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.ParseWithTokenSource(source, grammars.NewAuthzedTokenSourceOrEOF(source, language))
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)
			t.Logf("route=production %s", authzedProbeReceipt(production, language, cTree, cDigest))

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.ParseWithTokenSource(source, grammars.NewAuthzedTokenSourceOrEOF(source, language))
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			t.Logf("route=compact admission_requested=true candidate_declined=true counters=%d/%d->%d/%d %s", routedBefore, fallbackBefore, routedAfter, fallbackAfter, authzedProbeReceipt(compact, language, cTree, cDigest))

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			if forestOK && forest != nil {
				t.Cleanup(forest.Release)
				t.Logf("route=forest status=accepted %s", authzedProbeReceipt(forest, language, cTree, cDigest))
			} else {
				offset, symbol, reason, states := forestParser.ForestDeclineInfo()
				t.Logf("route=forest status=declined offset=%d symbol=%d reason=%q states=%v backend=custom-authzed-token-source", offset, symbol, reason, states)
			}

			incrementalSource := append(append([]byte(nil), source...), '\n')
			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			oldTree, err := incrementalParser.ParseWithTokenSource(source, grammars.NewAuthzedTokenSourceOrEOF(source, language))
			if err != nil {
				t.Fatalf("incremental base parse: %v", err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(source)),
				OldEndByte:  uint32(len(source)),
				NewEndByte:  uint32(len(incrementalSource)),
				StartPoint:  authzedProbePointAtByte(source, len(source)),
				OldEndPoint: authzedProbePointAtByte(source, len(source)),
				NewEndPoint: authzedProbePointAtByte(incrementalSource, len(incrementalSource)),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalWithTokenSourceProfiled(incrementalSource, oldTree, grammars.NewAuthzedTokenSourceOrEOF(incrementalSource, language))
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			t.Cleanup(incremental.Release)
			incrementalCParser := sitter.NewParser()
			t.Cleanup(incrementalCParser.Close)
			if err := incrementalCParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			incrementalCTree := incrementalCParser.Parse(incrementalSource, nil)
			if incrementalCTree == nil || incrementalCTree.RootNode() == nil {
				t.Fatal("locked C parser returned no incremental tree")
			}
			t.Cleanup(incrementalCTree.Close)
			incrementalCDigest, err := COracleDeepDigest(incrementalCTree)
			if err != nil {
				t.Fatal(err)
			}
			freshParser := gotreesitter.NewParser(language)
			fresh, err := freshParser.ParseWithTokenSource(incrementalSource, grammars.NewAuthzedTokenSourceOrEOF(incrementalSource, language))
			if err != nil {
				t.Fatalf("incremental fresh comparison parse: %v", err)
			}
			t.Cleanup(fresh.Release)
			incrementalInspection, err := benchfixtures.InspectGoTree(incremental.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			freshInspection, err := benchfixtures.InspectGoTree(fresh.RootNode(), language)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("route=incremental reuse=%t unsupported=%t reason=%q reused_subtrees=%d reused_bytes=%d source_sha256=%x incremental_fresh_equal=%t fresh_digest=%s %s", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes, sha256.Sum256(incrementalSource), incrementalInspection.SHA256 == freshInspection.SHA256, freshInspection.SHA256, authzedProbeReceipt(incremental, language, incrementalCTree, incrementalCDigest))

			t.Logf("witness=%s bytes=%d source_sha256=%x c_digest=%s c_error=%t", witness.name, len(source), sha256.Sum256(source), cDigest, cTree.RootNode().HasError())
		})
	}
}

// TestAuthzedNextLiveArmReceiptDocument guards the durable blocker markers.
func TestAuthzedNextLiveArmReceiptDocument(t *testing.T) {
	raw, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	document := strings.Join(strings.Fields(string(raw)), " ")
	for _, marker := range []string{
		"The Authzed arm remains live.",
		"The A0 receipt records three files, three checked, three run, 8,326 visited nodes, 18 rewrites, one error root, and zero parse errors.",
		"The raw route uses the built-in deterministic finite automaton (DFA) path.",
		"The real corpus is unavailable because `cgo_harness/corpus_real` is absent.",
		"Require the scheduler owner to emit every locked-C tree without rewrites.",
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("Authzed blocker receipt lacks marker %q", marker)
		}
	}
}

func authzedProbeReceipt(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) string {
	if tree == nil || tree.RootNode() == nil {
		return "tree=nil"
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		return fmt.Sprintf("inspect_error=%v", err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	runtime := tree.ParseRuntime()
	return fmt.Sprintf("error_root=%t digest=%s dispatch_rewrites=%d runtime_rewrites=%d passes=%s c_digest=%s exact=%t divergence=%+v", tree.RootNode().HasError(), inspection.SHA256, authzedProbeDispatchRewrites(tree), runtime.NormalizationNodesRewritten, authzedProbePasses(tree), cDigest, diff == nil && inspection.SHA256 == cDigest, diff)
}

func authzedProbeDispatchRewrites(tree *gotreesitter.Tree) uint64 {
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return 0
	}
	var total uint64
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name == "dispatch.authzed" {
			total += pass.NodesRewritten
		}
	}
	return total
}

func authzedProbePasses(tree *gotreesitter.Tree) string {
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return "none"
	}
	parts := make([]string, 0, len(*runtime.NormalizationPasses))
	for _, pass := range *runtime.NormalizationPasses {
		parts = append(parts, fmt.Sprintf("%s:%d", pass.Name, pass.NodesRewritten))
	}
	return fmt.Sprintf("%v", parts)
}

func authzedProbePointAtByte(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	for index, value := range source {
		if index >= offset {
			break
		}
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
