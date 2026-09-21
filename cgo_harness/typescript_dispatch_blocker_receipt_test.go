//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
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

type typescriptDispatchWitness struct {
	name                     string
	source                   func(*testing.T) []byte
	wantSourceSHA            string
	wantDigest               string
	wantProductionVisited    uint64
	wantProductionRewritten  uint64
	wantIncrementalVisited   uint64
	wantIncrementalRewritten uint64
	wantCompactMode          string
	wantReusedSubtrees       uint64
	wantReusedBytes          uint64
}

// TestTypeScriptDispatchBlockerRoutes records all required routes for the
// live TypeScript dispatcher arm and its focused selection witnesses.
func TestTypeScriptDispatchBlockerRoutes(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.TypescriptLanguage()
	if language.ExternalScanner == nil {
		t.Fatal("TypeScript language has no registered external scanner")
	}
	reusable, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner)
	if !ok || !reusable.SupportsIncrementalReuse() {
		t.Fatalf("TypeScript external scanner does not support incremental reuse: %T", language.ExternalScanner)
	}
	t.Logf("scanner=present type=%T supports_incremental_reuse=true", language.ExternalScanner)

	cLanguage, err := COracleLanguage("typescript")
	if err != nil {
		t.Fatal(err)
	}
	witnesses := []typescriptDispatchWitness{
		{
			name: "tracked-1462-15", source: func(t *testing.T) []byte {
				source, err := os.ReadFile(filepath.Join("..", "cgo_harness", "corpus_structural", "typescript_sample.ts"))
				if err != nil {
					t.Fatal(err)
				}
				return source
			},
			wantSourceSHA:            "40b4a7a06fde353d8c2b726acb16f59aab44d49d1b6257c37345c2a1f56b9fb7",
			wantDigest:               "0c29d566e57e5bdee435a7c8f17578bc2b0e5ff53c8dfea720655fec2b9f7f39",
			wantProductionVisited:    1462,
			wantProductionRewritten:  15,
			wantIncrementalVisited:   1462,
			wantIncrementalRewritten: 10,
			wantCompactMode:          "fallback",
			wantReusedSubtrees:       406,
			wantReusedBytes:          2289,
		},
		{
			name: "positive-simple", source: func(*testing.T) []byte {
				return []byte("const value: number = 1;\n")
			},
			wantSourceSHA:          "5967d633a6670814c4b5e0a8c889eb5c0e51155258d35d68a476eb1717e6e2ee",
			wantDigest:             "1e38064181b465fdf83382149c49a085ccad8cc2a7fcefad67b187c6d87ee619",
			wantProductionVisited:  12,
			wantIncrementalVisited: 12,
			wantCompactMode:        "accepted",
			wantReusedSubtrees:     5,
			wantReusedBytes:        18,
		},
		{
			name: "typed-arrow-return", source: func(*testing.T) []byte {
				return []byte("const f = (a: A): B => a;\n")
			},
			wantSourceSHA:          "8ede7d478c3201e1cbd1ba129ec7b844e71f174094be19ecaf27a2344a6d67f2",
			wantDigest:             "6c5d7858e8ca512ff1f3082e2f4be701ce95c4741ea01ddb41e1f2d681e83d00",
			wantProductionVisited:  21,
			wantIncrementalVisited: 21,
			wantCompactMode:        "fallback",
			wantReusedSubtrees:     6,
			wantReusedBytes:        10,
		},
		{
			name: "generic-arrow-comma", source: func(*testing.T) []byte {
				return []byte("const f = <T,>(a: T): T => a;\n")
			},
			wantSourceSHA:          "b0e90a18d9bdf1da875885b3ac4c0d80b0214b316f2d5ff2170023f91e62d849",
			wantDigest:             "61aa2071bbcdba4a421c3ebff9a2fdfa3bb282c799c85deab98c26b7a2a6adc0",
			wantProductionVisited:  27,
			wantIncrementalVisited: 27,
			wantCompactMode:        "fallback",
			wantReusedSubtrees:     4,
			wantReusedBytes:        8,
		},
		{
			name: "generic-call-simple", source: func(*testing.T) []byte {
				return []byte("foo<number>(1);\n")
			},
			wantSourceSHA:          "399364dd8ba692c16b2387a2954c653a75b2e52f0af88e5810bdc4d9555f1fb5",
			wantDigest:             "5fd25b615488dd97468bc371bfb05af01b4fa13d17559c52c35246d27174739b",
			wantProductionVisited:  14,
			wantIncrementalVisited: 14,
			wantCompactMode:        "fallback",
			wantReusedSubtrees:     2,
			wantReusedBytes:        9,
		},
		{
			name: "generic-call-selection-gap", source: func(*testing.T) []byte {
				return []byte("token() === SyntaxKind.TrueKeyword || token() === SyntaxKind.FalseKeyword ? parseTokenNode<BooleanLiteral>() : parseLiteralLikeNode(token()) as LiteralExpression\n")
			},
			wantSourceSHA:          "7ffd2531b611cb09cd9d47e2be4b024d8b4c5f7b98a14e9a9fa73ed882f246bb",
			wantDigest:             "d0ba1c98d9058b3aff2795d1cf5b019a57306a6b247724f92dc961a62b802f02",
			wantProductionVisited:  51,
			wantIncrementalVisited: 51,
			wantCompactMode:        "accepted",
			wantReusedSubtrees:     9,
			wantReusedBytes:        52,
		},
		{
			name: "issue-544-structural-control", source: func(t *testing.T) []byte {
				source, err := os.ReadFile(filepath.Join("..", "grammars", "testdata", "typescript_issue_544.ts"))
				if err != nil {
					t.Fatal(err)
				}
				return source
			},
			wantSourceSHA:            "fe0ffa1df2c94d1f0ccde7d1aad3a50b90469fcfe1e98ad5812eba13c22809a4",
			wantDigest:               "6049f72952e720eb432bad16a60ca80f541f16785f17d2ea143b5e4ac3422103",
			wantProductionVisited:    832,
			wantProductionRewritten:  2,
			wantIncrementalVisited:   832,
			wantIncrementalRewritten: 2,
			wantCompactMode:          "fallback",
			wantReusedSubtrees:       11,
			wantReusedBytes:          64,
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			source := witness.source(t)
			sourceSHA := fmt.Sprintf("%x", sha256.Sum256(source))
			if sourceSHA != witness.wantSourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", sourceSHA, witness.wantSourceSHA)
			}

			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked-C parser returned no root")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != witness.wantDigest {
				t.Fatalf("locked-C digest=%s, want %s", cDigest, witness.wantDigest)
			}

			goLang := language
			rawParser := gotreesitter.NewParser(goLang)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			typescriptDispatchAssertExact(t, "raw", raw, goLang, cTree, witness.wantDigest)

			productionParser := gotreesitter.NewParser(goLang)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			typescriptDispatchAssertExact(t, "production", production, goLang, cTree, witness.wantDigest)
			typescriptDispatchCheckPass(t, "production", production, witness.wantProductionVisited, witness.wantProductionRewritten)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(goLang)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactMode := typescriptDispatchCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			if compactMode != witness.wantCompactMode {
				t.Fatalf("compact mode=%s, want %s", compactMode, witness.wantCompactMode)
			}
			if compactMode == "fallback" && !strings.Contains(gotreesitter.AdmissionCandidateLastFallbackReason(), "scheduler-frontier-shape") {
				t.Fatalf("compact fallback reason=%q, want scheduler-frontier-shape", gotreesitter.AdmissionCandidateLastFallbackReason())
			}
			typescriptDispatchAssertExact(t, "compact", compact, goLang, cTree, witness.wantDigest)
			typescriptDispatchCheckPass(t, "compact", compact, witness.wantProductionVisited, witness.wantProductionRewritten)

			forestParser := gotreesitter.NewParser(goLang)
			forest, forestOK := forestParser.ParseForestExperimental(source)
			if !forestOK || forest == nil {
				t.Fatal("forest route declined for an accessible TypeScript witness")
			}
			t.Cleanup(forest.Release)
			typescriptDispatchAssertExact(t, "forest", forest, goLang, cTree, witness.wantDigest)

			incrementalParser := gotreesitter.NewParser(goLang)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(source, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(source)),
				StartPoint:  typescriptDispatchPoint(base),
				OldEndPoint: typescriptDispatchPoint(base),
				NewEndPoint: typescriptDispatchPoint(source),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(source, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			typescriptDispatchAssertExact(t, "incremental", incremental, goLang, cTree, witness.wantDigest)
			typescriptDispatchCheckPass(t, "incremental", incremental, witness.wantIncrementalVisited, witness.wantIncrementalRewritten)
			if !profile.OldTreeReuseRoute || profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" {
				t.Fatalf("incremental reuse route changed: old_tree=%t unsupported=%t reason=%q", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
			}
			if profile.ReusedSubtrees != witness.wantReusedSubtrees || profile.ReusedBytes != witness.wantReusedBytes {
				t.Fatalf("incremental reuse=%d subtrees/%d bytes, want %d/%d", profile.ReusedSubtrees, profile.ReusedBytes, witness.wantReusedSubtrees, witness.wantReusedBytes)
			}
			t.Logf("witness=%s bytes=%d source_sha256=%s digest=%s compact=%s incremental_reuse=true reused_subtrees=%d reused_bytes=%d", witness.name, len(source), sourceSHA, witness.wantDigest, compactMode, profile.ReusedSubtrees, profile.ReusedBytes)
		})
	}
}

// TestTypeScriptMergeCapOneTypedArrowReceipt preserves the controlled
// diagnostic that exposes the unresolved parser selection boundary.
//
// Re-pinned 2026-09-21 (task #88 audit). The pin went stale for the same
// reason as the HLSL and Authzed witnesses: commit a122eac7d stopped
// excluding missing and error leaves from inherited field assignment
// (parser_reduce.go). Its parent, 8e9a083f5, still matched the old pin.
// The locked C digest and the first divergence are unchanged.
func TestTypeScriptMergeCapOneTypedArrowReceipt(t *testing.T) {
	const source = "const f = (a: A): B => a;\n"
	wantGoDigest := "1d0f23a994999bc5cd051081e7d3bdd5fc58f28b37e801d2e320ec29bf4249c1"
	wantCDigest := "6c5d7858e8ca512ff1f3082e2f4be701ce95c4741ea01ddb41e1f2d681e83d00"
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "1")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)

	goLang := grammars.TypescriptLanguage()
	cLanguage, err := COracleLanguage("typescript")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse([]byte(source), nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked-C parser returned no root")
	}
	t.Cleanup(cTree.Close)
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	if cDigest != wantCDigest {
		t.Fatalf("locked-C digest=%s, want %s", cDigest, wantCDigest)
	}

	parser := gotreesitter.NewParser(goLang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.ParseNoResultCompatibilityBenchmarkOnly([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), goLang)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != wantGoDigest {
		t.Fatalf("cap-one Go digest=%s, want %s", inspection.SHA256, wantGoDigest)
	}
	if !tree.RootNode().HasError() {
		t.Fatal("cap-one typed-arrow tree has no root error")
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), goLang, cTree.RootNode())
	wantDiff := &DumpV1Divergence{Path: "/program", Category: "error", GoValue: "true", CValue: "false"}
	if diff == nil || *diff != *wantDiff {
		t.Fatalf("cap-one first divergence=%+v, want %+v", diff, wantDiff)
	}
	t.Logf("cap=1 source_sha256=%x go_digest=%s c_digest=%s first_divergence=%+v", sha256.Sum256([]byte(source)), inspection.SHA256, cDigest, diff)
}

// TestTypeScriptDispatchBlockerReceiptDocument guards the durable receipt.
func TestTypeScriptDispatchBlockerReceiptDocument(t *testing.T) {
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
		"## 2026-08-24 TypeScript dispatcher blocker receipt",
		"Status: `KEEP LIVE / NO-GO`. Keep `dispatch.typescript` live.",
		"Base commit: `731f8a9d9440a006b2cc6b56ef5b31c0ff3b5ce7`.",
		"cgo_harness/typescript_dispatch_blocker_receipt_test.go",
		"40b4a7a06fde353d8c2b726acb16f59aab44d49d1b6257c37345c2a1f56b9fb7",
		"1,462 visited nodes, 15 rewritten nodes",
		"The A0 (initial dispatcher census) manifest has 14 languages and 42 files.",
		"It excludes TypeScript and TSX.",
		"The full authenticated TypeScript and TSX corpus is unavailable.",
		"supports_incremental_reuse=true",
		"0c29d566e57e5bdee435a7c8f17578bc2b0e5ff53c8dfea720655fec2b9f7f39",
		"GOT_GLR_MAX_MERGE_PER_KEY=1",
		"The existing ternary generic-call selection test remains skipped.",
		"webworker.generated.d.ts",
		"was not rerun locally",
		"Keep `dispatch.typescript` live until",
		`## 2026-08-24 TypeScript current-main follow-up`,
		`ab84e809297e945a6debd52ec3e211956b497893`,
		`20260824T102012Z-typescript-current-main-ab84`,
		`5ed6103ec1d94a21570c02dd5e27067024383b61b59040c3a670a40109a40085`,
		`2f53f7d3d13f66545d55f7ac6f19d2398c0403bb663757704a15650b4fc84015`,
		`75c98fc9f46ab991c18d9660dd9dfb08d5cfd52b25f1d379e948392f87c56a0e`,
		`46d8d4f7a0056db32e874500ae5b19170237e1628a63a9e3a401e0ee426d6126`,
		`bf8c490b0bbeb6d4150abce2edc193552e44b093893665dde69bd39e9e940e85`,
		`All shipping witnesses retain their locked-C deep digests.`,
		`The tracked witness retains 15 production rewrites and 10 incremental rewrites.`,
		`406 subtrees, 2,289 bytes`,
		`GOT_GLR_MAX_MERGE_PER_KEY=1`,
	} {
		marker = strings.Join(strings.Fields(marker), " ")
		if !strings.Contains(document, marker) {
			t.Fatalf("retirement document lacks marker %q", marker)
		}
	}
	for _, marker := range []string{
		"Record the N31b TypeScript dispatcher blocker",
		"731f8a9d9440a006b2cc6b56ef5b31c0ff3b5ce7",
		"Keep `dispatch.typescript` live.",
		"No production code changed.",
	} {
		if !strings.Contains(string(changelog), marker) {
			t.Fatalf("changelog lacks marker %q", marker)
		}
	}
}

func typescriptDispatchAssertExact(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, wantDigest string) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no tree", route)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("%s returned a root error: %s", route, tree.RootNode().SExpr(language))
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("%s inspect: %v", route, err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	if diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode()); diff != nil {
		t.Fatalf("%s first locked-C divergence=%+v", route, diff)
	}
}

func typescriptDispatchCheckPass(t *testing.T, route string, tree *gotreesitter.Tree, wantVisited, wantRewritten uint64) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		t.Fatalf("%s did not record normalization passes", route)
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name != "dispatch.typescript" {
			continue
		}
		if pass.NodesVisited != wantVisited || pass.NodesRewritten != wantRewritten {
			t.Fatalf("%s dispatch pass=%d/%d, want %d/%d", route, pass.NodesVisited, pass.NodesRewritten, wantVisited, wantRewritten)
		}
		return
	}
	t.Fatalf("%s did not record dispatch.typescript", route)
}

func typescriptDispatchCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter uint64) string {
	if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
		return "accepted"
	}
	if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
		return "fallback"
	}
	return fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
}

func typescriptDispatchPoint(source []byte) gotreesitter.Point {
	var point gotreesitter.Point
	for _, value := range source {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
