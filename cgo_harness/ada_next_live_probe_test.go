//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestAdaNextLiveArmProbe records every route before a possible retirement.
// It does not assert parity. The receipt exposes each real rewrite and
// divergence, including malformed recovery and forest behavior.
func TestAdaNextLiveArmProbe(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	language := grammars.AdaLanguage()
	cLanguage, err := COracleLanguage("ada")
	if err != nil {
		t.Fatal(err)
	}

	witnesses := []struct {
		name      string
		source    []byte
		malformed bool
	}{
		{
			name: "positional-object-decl-access",
			source: []byte("procedure P is\n" +
				"   X : Rec (Pkg.Obj'Access);\n" +
				"begin\n" +
				"   null;\n" +
				"end P;\n"),
		},
		{
			name: "positional-subtype-decl-access",
			source: []byte("procedure P is\n" +
				"   subtype S is Rec (Pkg.Obj'Access);\n" +
				"begin\n" +
				"   null;\n" +
				"end P;\n"),
		},
		{
			name: "positional-record-component-access",
			source: []byte("procedure P is\n" +
				"   type Holder is record\n" +
				"      Field : Rec (Pkg.Obj'Access);\n" +
				"   end record;\n" +
				"begin\n" +
				"   null;\n" +
				"end P;\n"),
		},
		{
			name: "positional-nested-selector-access",
			source: []byte("procedure P is\n" +
				"   X : Rec (Pkg.Sub.Obj'Access);\n" +
				"begin\n" +
				"   null;\n" +
				"end P;\n"),
		},
		{
			name: "positional-allocator-access",
			source: []byte("procedure P is\n" +
				"begin\n" +
				"   A := new T (Pkg.Obj'Access);\n" +
				"end P;\n"),
		},
		{
			name: "positional-object-decl-size",
			source: []byte("procedure P is\n" +
				"   X : Rec (Obj'Size);\n" +
				"begin\n" +
				"   null;\n" +
				"end P;\n"),
		},
		{
			name: "positional-array-aggregate",
			source: []byte("package P is\n" +
				"   type A is array (1 .. 3) of Boolean;\n" +
				"   V : constant A := (1, 2, 3);\n" +
				"end P;\n"),
		},
		{
			name: "named-association-control",
			source: []byte("procedure P is\n" +
				"begin\n" +
				"   A := new T (F => Pkg.Obj'Access);\n" +
				"end P;\n"),
		},
		{
			name: "array-others-control",
			source: []byte("procedure P is\n" +
				"begin\n" +
				"   A := (others => 0);\n" +
				"end P;\n"),
		},
		{
			name: "malformed-truncated-association",
			source: []byte("procedure P is\n" +
				"begin\n" +
				"   A := (Pkg.Obj'Access;\n"),
			malformed: true,
		},
		{
			name: "malformed-truncated-array-aggregate",
			source: []byte("package P is\n" +
				"   type A is array (1 .. 3) of Boolean;\n" +
				"   V : constant A := (1, 2,\n"),
			malformed: true,
		},
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			target := append([]byte(nil), witness.source...)
			if len(target) == 0 || target[len(target)-1] != '\n' {
				target = append(target, '\n')
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(target, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parse returned no root")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(target)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(raw.Release)
			t.Logf("route=raw %s", adaProbeReceipt(raw, language, cTree, cDigest))

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(target)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(production.Release)
			t.Logf("route=production %s", adaProbeReceipt(production, language, cTree, cDigest))

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(target)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(compact.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			compactRoute := "accepted"
			if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
				compactRoute = "fallback:" + gotreesitter.AdmissionCandidateLastFallbackReason()
			} else if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf("compact counters before=(%d,%d) after=(%d,%d)", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			}
			t.Logf("route=compact mode=%s %s", compactRoute, adaProbeReceipt(compact, language, cTree, cDigest))

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(target)
			if forestOK && forest != nil {
				t.Cleanup(forest.Release)
				t.Logf("route=forest mode=accepted %s", adaProbeReceipt(forest, language, cTree, cDigest))
			} else {
				t.Logf("route=forest mode=declined")
			}

			incrementalParser := gotreesitter.NewParser(language)
			incrementalParser.SetAdmissionCandidateRoute(false)
			base := bytes.TrimSuffix(target, []byte{'\n'})
			oldTree, err := incrementalParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(oldTree.Release)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(len(base)),
				OldEndByte:  uint32(len(base)),
				NewEndByte:  uint32(len(target)),
				StartPoint:  adaProbePointAtByte(base, len(base)),
				OldEndPoint: adaProbePointAtByte(base, len(base)),
				NewEndPoint: adaProbePointAtByte(target, len(target)),
			})
			incremental, profile, err := incrementalParser.ParseIncrementalProfiled(target, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(incremental.Release)
			t.Logf("route=incremental reuse=%t unsupported=%t reason=%s reused_subtrees=%d reused_bytes=%d %s", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes, adaProbeReceipt(incremental, language, cTree, cDigest))
			t.Logf("witness=%s malformed=%t bytes=%d source_sha256=%x c_digest=%s", witness.name, witness.malformed, len(target), sha256.Sum256(target), cDigest)
		})
	}
}

func adaProbeReceipt(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest string) string {
	if tree == nil || tree.RootNode() == nil {
		return "tree=nil"
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		return fmt.Sprintf("inspect_error=%v", err)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	return fmt.Sprintf("error_root=%t digest=%s rewrites=%d passes=%s c_digest=%s exact=%t divergence=%+v", tree.RootNode().HasError(), inspection.SHA256, tree.ParseRuntime().NormalizationNodesRewritten, adaProbePasses(tree), cDigest, diff == nil && inspection.SHA256 == cDigest, diff)
}

func adaProbePasses(tree *gotreesitter.Tree) string {
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

func adaProbePointAtByte(source []byte, offset int) gotreesitter.Point {
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
