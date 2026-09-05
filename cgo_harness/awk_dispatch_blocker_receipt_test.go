//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	awkRecoveredSourceSHA = "f3dd8c811b2ad06c865fb1ad59ac0098fef57bdbb89377ac96ddb4e845f6bfba"
	awkRecoveredRawDigest = "6d53efe8af8b1e47aaf1defa8d2a727a6bcd43c7a9fc37516c6bb2b45ad0db56"
	awkRecoveredGoDigest  = "cead9d68f270583fa37ed19b470ca4482ce315b41a30528b7432e95a07fefee8"
	awkRecoveredCDigest   = "bb33c51db03cf6f16c5b206ce6d47d8369e4904f86466fecd9440311e5995925"
	awkCleanSourceSHA     = "99d1043aabedfc2a53a4d50d35fd0e5f257beb49612617c0855c37ab4baa6ec1"
	awkCleanDigest        = "6cd4e8645947bff0604ea5131f9b2188322a021b84db5f3f7c729a76b330d5d2"
)

type awkDispatchRouteWitness struct {
	name       string
	source     []byte
	sourceSHA  string
	rawDigest  string
	goDigest   string
	cDigest    string
	goChildren int
	cChildren  int
}

// TestAWKDispatchBlockerRoutes records the live AWK arm on focused routes.
func TestAWKDispatchBlockerRoutes(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	recovered := awkRecoveredFixture(t)
	witnesses := []awkDispatchRouteWitness{
		{
			name: "clean", source: []byte("BEGIN { print 1 }\n"),
			sourceSHA: awkCleanSourceSHA, rawDigest: awkCleanDigest, goDigest: awkCleanDigest, cDigest: awkCleanDigest,
		},
		{
			name: "recovery", source: recovered,
			sourceSHA: awkRecoveredSourceSHA, rawDigest: awkRecoveredRawDigest, goDigest: awkRecoveredGoDigest, cDigest: awkRecoveredCDigest,
			goChildren: 408, cChildren: 338,
		},
	}

	language := grammars.AwkLanguage()
	if language.ExternalScanner == nil {
		t.Fatal("AWK language has no registered external scanner")
	}
	scanner, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner)
	if !ok || !scanner.SupportsIncrementalReuse() {
		t.Fatal("AWK scanner does not support incremental reuse")
	}
	stateless, ok := language.ExternalScanner.(gotreesitter.StatelessExternalScanner)
	preserving, preservingOK := language.ExternalScanner.(gotreesitter.FailurePreservingExternalScanner)
	if !ok || !stateless.ExternalScannerIsStateless() || !preservingOK || !preserving.PreservesStateOnScanFailure() {
		t.Fatal("AWK scanner lost stateless or failure-preserving proof")
	}
	cLanguage, err := COracleLanguage("awk")
	if err != nil {
		t.Fatal(err)
	}

	for _, witness := range witnesses {
		witness := witness
		t.Run(witness.name, func(t *testing.T) {
			cTree := awkCTree(t, cLanguage, witness.source)
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(witness.source)); got != witness.sourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", got, witness.sourceSHA)
			}
			if cDigest != witness.cDigest {
				t.Fatalf("locked-C digest=%s, want %s", cDigest, witness.cDigest)
			}

			rawParser := gotreesitter.NewParser(language)
			rawParser.SetAdmissionCandidateRoute(false)
			raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(witness.source)
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Release()
			awkCheckRoute(t, "raw", raw, language, cTree, cDigest, witness.rawDigest)

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(witness.source)
			if err != nil {
				t.Fatal(err)
			}
			defer production.Release()
			awkCheckRoute(t, "production", production, language, cTree, cDigest, witness.goDigest)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			compactParser := gotreesitter.NewParser(language)
			compactParser.SetAdmissionCandidateRoute(true)
			compact, err := compactParser.Parse(witness.source)
			if err != nil {
				t.Fatal(err)
			}
			defer compact.Release()
			awkCheckRoute(t, "compact", compact, language, cTree, cDigest, witness.goDigest)
			if witness.name == "recovery" {
				routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
				if routedAfter-routedBefore != 0 || fallbackAfter-fallbackBefore != 1 {
					t.Fatalf("recovery compact counter delta=%d/%d, want routed=0 fallback=1", routedAfter-routedBefore, fallbackAfter-fallbackBefore)
				}
			}

			forestParser := gotreesitter.NewParser(language)
			forest, forestOK := forestParser.ParseForestExperimental(witness.source)
			if witness.name == "clean" {
				if !forestOK || forest == nil {
					t.Fatal("clean forest route declined")
				}
				defer forest.Release()
				awkCheckRoute(t, "forest", forest, language, cTree, cDigest, witness.goDigest)
			} else if forestOK || forest != nil {
				if forest != nil {
					forest.Release()
				}
				t.Fatal("recovery forest route accepted")
			}

			if witness.name == "clean" {
				awkCheckCleanIncremental(t, language, witness.source, cLanguage)
			}
			if witness.goChildren > 0 {
				if got := int(production.RootNode().ChildCount()); got != witness.goChildren {
					t.Fatalf("production root children=%d, want %d", got, witness.goChildren)
				}
				if got := int(cTree.RootNode().ChildCount()); got != witness.cChildren {
					t.Fatalf("locked-C root children=%d, want %d", got, witness.cChildren)
				}
				if child := production.RootNode().Child(0); child == nil || child.StartByte() != 0 || child.EndByte() != 11 {
					t.Fatalf("production first rule span changed")
				}
				if child := cTree.RootNode().Child(0); child == nil || child.StartByte() != 0 || child.EndByte() != 47 {
					t.Fatalf("locked-C first rule span changed")
				}
			}
		})
	}
}

func TestAWKDispatchBlockerReceiptDocument(t *testing.T) {
	doc, err := os.ReadFile("../docs/root-normalization-retirement.md")
	if err != nil {
		t.Fatal(err)
	}
	changelog, err := os.ReadFile("../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"## 2026-08-24 AWK dispatcher blocker receipt",
		"Status: `KEEP LIVE / NO-GO`. Keep `dispatch.awk` live.",
		"61a7c75e225e3035390be32d635545e40d8c5faf",
		"5739fd79bcfc75ba7526773d0cf634521f8aca3c",
		"f3dd8c811b2ad06c865fb1ad59ac0098fef57bdbb89377ac96ddb4e845f6bfba",
		"6d53efe8af8b1e47aaf1defa8d2a727a6bcd43c7a9fc37516c6bb2b45ad0db56",
		"cead9d68f270583fa37ed19b470ca4482ce315b41a30528b7432e95a07fefee8",
		"bb33c51db03cf6f16c5b206ce6d47d8369e4904f86466fecd9440311e5995925",
		"Both witnesses cover raw,",
		"Only the clean witness has an incremental receipt.",
		"Recovery incremental telemetry remains absent.",
		"recovery incremental telemetry is recorded",
		"The original",
		"generated probe sources are absent",
		"20260823T061200Z",
		"20260823T054932Z-awk-c-routes",
		"20260823T055314Z-awk-normalizer-order",
	} {
		if !strings.Contains(string(doc), marker) {
			t.Fatalf("retirement document lacks marker %q", marker)
		}
	}
	for _, marker := range []string{
		"Recorded the AWK dispatcher blocker",
		"Keep `dispatch.awk` live",
		"Ship no parser or",
		"registry change.",
		"Rechecked the AWK dispatcher blocker at current main commit",
		"83e0cfbc30ad82e2f327d58e35eea9f438a0ffda",
		"The one-language Docker guard",
	} {
		if !strings.Contains(string(changelog), marker) {
			t.Fatalf("changelog lacks marker %q", marker)
		}
	}
}

func awkRecoveredFixture(t *testing.T) []byte {
	t.Helper()
	encoded, err := os.ReadFile("../testdata/awk_recovered_rule_split/T.gawk.b64")
	if err != nil {
		t.Fatal(err)
	}
	source, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 7392 || fmt.Sprintf("%x", sha256.Sum256(source)) != awkRecoveredSourceSHA {
		t.Fatal("tracked T.gawk fixture identity changed")
	}
	return source
}

func awkCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
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

func awkCheckRoute(t *testing.T, route string, tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree, cDigest, wantDigest string) {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != wantDigest {
		t.Fatalf("%s digest=%s, want %s", route, inspection.SHA256, wantDigest)
	}
	diff := FirstDivergenceDumpV1(tree.RootNode(), language, cTree.RootNode())
	exact := diff == nil && inspection.SHA256 == cDigest
	t.Logf("route=%s bytes=%d digest=%s c_digest=%s exact=%t error_root=%t rewrites=%d divergence=%+v", route, len(tree.Source()), inspection.SHA256, cDigest, exact, tree.RootNode().HasError(), tree.ParseRuntime().NormalizationNodesRewritten, diff)
}

func awkCheckCleanIncremental(t *testing.T, language *gotreesitter.Language, source []byte, cLanguage *sitter.Language) {
	t.Helper()
	edited := bytes.Replace(source, []byte("print 1"), []byte("print 2"), 1)
	if len(edited) != len(source) {
		t.Fatal("clean edit changed source length")
	}
	parser := gotreesitter.NewParser(language)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	offset := bytes.Index(source, []byte("1"))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1),
		StartPoint:  gotreesitter.Point{Row: 0, Column: uint32(offset)},
		OldEndPoint: gotreesitter.Point{Row: 0, Column: uint32(offset + 1)},
		NewEndPoint: gotreesitter.Point{Row: 0, Column: uint32(offset + 1)},
	})
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer incremental.Release()
	if profile.ReparseNanos <= 0 {
		t.Fatalf("clean incremental profile=%+v", profile)
	}
	cTree := awkCTree(t, cLanguage, edited)
	defer cTree.Close()
	awkCheckRoute(t, "incremental", incremental, language, cTree, awkCleanDigest, awkCleanDigest)
}
