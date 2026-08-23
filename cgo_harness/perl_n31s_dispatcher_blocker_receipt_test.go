//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
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
	perlN31sBaseCommit         = "af9ded2b77b7828b12b1d2da7c9fff8dd5ca053b"
	perlN31sGrammarLockSHA     = "9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb"
	perlN31sGrammarBlobSHA     = "22388f06c2c54bb4748fd5f5f682ed25eecff8115a7e8e6a98f94f9c94bb9820"
	perlN31sA0ManifestSHA      = "215df59aa56d28caa403f799733ef915db1c4ac07eb2bc96a9402f80cf67f80a"
	perlN31sTrackedManifestSHA = "be584a0a4a26f0ca5268a7845cf3f04247e6b57259b9c7057e8eb2c9af26f839"
	perlN31sCorpusSidecarSHA   = "2b2209597d1701ccc813bd35d1685b5b13730e6ebd285e66485ce812e35877cf"
	perlN31sCorpusLockSHA      = "41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea"
	perlN31sCArtifactSHA       = "3d8bb427c9043d5e4846f5cd83313afecf6b6e27be8fb82a7cd58f3b8f52ab87"
	perlN31sGrammarRepo        = "https://github.com/tree-sitter-perl/tree-sitter-perl"
	perlN31sGrammarCommit      = "ad74e6db234c35d537de9358799a8e0cc4f5dee0"
)

func TestPerlN31sDispatcherBlockerReceipt(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	sources := []struct {
		name string
		src  []byte
	}{
		{name: "push_two_args", src: []byte("push @found, $_;\n")},
		{name: "push_three_args", src: []byte("push @found, $a, $b;\n")},
	}
	goLang := grammars.PerlLanguage()
	if goLang == nil || goLang.Name != "perl" {
		t.Fatalf("language=%v, want perl", goLang)
	}
	if goLang.ExternalScanner == nil || fmt.Sprintf("%T", goLang.ExternalScanner) != "grammars.PerlExternalScanner" {
		t.Fatalf("scanner=%T, want grammars.PerlExternalScanner", goLang.ExternalScanner)
	}
	if _, ok := goLang.ExternalScanner.(gots.IncrementalReuseExternalScanner); ok {
		t.Fatal("Perl scanner unexpectedly advertises incremental reuse")
	}
	for path, want := range map[string]string{
		"../grammars/languages.lock":                        perlN31sGrammarLockSHA,
		"../grammars/grammar_blobs/perl.bin":                perlN31sGrammarBlobSHA,
		"../testdata/dispatcher_census_a0_manifest_v1.json": perlN31sA0ManifestSHA,
		"../testdata/dispatcher_census_tracked_v1.json":     perlN31sTrackedManifestSHA,
	} {
		if got := perlN31sHashFile(t, path); got != want {
			t.Fatalf("%s SHA-256=%s, want %s", path, got, want)
		}
	}
	if got := perlN31sHashFile(t, "perf_scan/corpus_sources.lock.sha256"); got != perlN31sCorpusSidecarSHA {
		t.Fatalf("corpus sidecar SHA-256=%s, want %s", got, perlN31sCorpusSidecarSHA)
	}
	sidecar, err := os.ReadFile("perf_scan/corpus_sources.lock.sha256")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(sidecar)) != perlN31sCorpusLockSHA+"  corpus_sources.lock" {
		t.Fatalf("corpus sidecar=%q", string(sidecar))
	}
	for _, absent := range []string{"../corpus_sources.lock", "perf_scan/corpus_sources.lock"} {
		if _, err := os.Stat(absent); !os.IsNotExist(err) {
			t.Fatalf("unauthenticated corpus lock %s is present or readable: %v", absent, err)
		}
	}
	cLang, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity("perl")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Contract != COracleContractVersion || identity.Transport != "cgo_parity_binding" || identity.BindingModule != COracleBindingModule || identity.BindingVersion != COracleBindingVersion || identity.BindingCommit != COracleBindingCommit || identity.RuntimeVersion != COracleRuntimeVersion || identity.RuntimeCommit != COracleRuntimeCommit || identity.RuntimeLinkage != "static_cgo_test_binary" || identity.Language != "perl" || identity.GrammarRepo != perlN31sGrammarRepo || identity.GrammarCommit != perlN31sGrammarCommit || identity.GrammarLinkage != "shared_dlopen" || identity.GrammarCompileFlags != COracleGrammarCFlags || identity.CompilerPath != "/usr/bin/cc" || identity.CompilerVersion != "cc (Debian 12.2.0-14+deb12u1) 12.2.0" || identity.GrammarArtifactPath == "" || identity.GrammarArtifactSHA256 != perlN31sCArtifactSHA {
		t.Fatalf("locked-C identity changed: %+v", identity)
	}
	t.Logf("base=%s locked_c_identity=%+v scanner=%T scanner_incremental_reuse=false included_ranges=not_applicable", perlN31sBaseCommit, identity, goLang.ExternalScanner)
	for _, tc := range sources {
		t.Run(tc.name, func(t *testing.T) {
			want := perlN31sExpected(tc.name)
			if got := fmt.Sprintf("%x", sha256.Sum256(tc.src)); got != want.sourceSHA {
				t.Fatalf("source SHA-256=%s, want %s", got, want.sourceSHA)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(tc.src, nil)
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			if cDigest != want.cDigest {
				t.Fatalf("locked-C digest=%s, want %s", cDigest, want.cDigest)
			}
			parse := func(route string, candidate bool, fn func(*gots.Parser) (*gots.Tree, error)) {
				p := gots.NewParser(goLang)
				p.SetAdmissionCandidateRoute(candidate)
				tree, err := fn(p)
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), goLang)
				if err != nil {
					t.Fatal(err)
				}
				diff := FirstDivergenceDumpV1(tree.RootNode(), goLang, cTree.RootNode())
				routeWant := want.routes[route]
				if inspection.SHA256 != routeWant.digest || tree.RootNode().HasError() != routeWant.error || perlN31sDispatch(tree) != routeWant.dispatch || (diff == nil) != (routeWant.diff == nil) || (diff != nil && *diff != *routeWant.diff) {
					t.Fatalf("%s result digest=%s error=%t divergence=%+v dispatch=%s, want digest=%s error=%t divergence=%+v dispatch=%s", route, inspection.SHA256, tree.RootNode().HasError(), diff, perlN31sDispatch(tree), routeWant.digest, routeWant.error, routeWant.diff, routeWant.dispatch)
				}
				t.Logf("source_sha256=%x route=%s digest=%s c_digest=%s error=%t c_error=%t divergence=%+v dispatch=%s rewrites=%d", sha256.Sum256(tc.src), route, inspection.SHA256, cDigest, tree.RootNode().HasError(), cTree.RootNode().HasError(), diff, perlN31sDispatch(tree), tree.ParseRuntime().NormalizationNodesRewritten)
			}
			parse("raw", false, func(p *gots.Parser) (*gots.Tree, error) { return p.ParseNoResultCompatibilityBenchmarkOnly(tc.src) })
			parse("production", false, func(p *gots.Parser) (*gots.Tree, error) { return p.Parse(tc.src) })
			beforeRouted, beforeFallback := gots.AdmissionCandidateCounters()
			parse("compact", true, func(p *gots.Parser) (*gots.Tree, error) { return p.Parse(tc.src) })
			afterRouted, afterFallback := gots.AdmissionCandidateCounters()
			if afterRouted-beforeRouted != want.compactRouted || afterFallback-beforeFallback != want.compactFallback || gots.AdmissionCandidateLastFallbackReason() != want.compactReason {
				t.Fatalf("compact counters=%d/%d reason=%q, want %d/%d reason=%q", afterRouted-beforeRouted, afterFallback-beforeFallback, gots.AdmissionCandidateLastFallbackReason(), want.compactRouted, want.compactFallback, want.compactReason)
			}
			t.Logf("compact routed=%d fallback=%d reason=%q", afterRouted-beforeRouted, afterFallback-beforeFallback, gots.AdmissionCandidateLastFallbackReason())
			forestParser := gots.NewParser(goLang)
			forest, ok := forestParser.ParseForestExperimental(tc.src)
			if ok != want.forestOK {
				t.Fatalf("forest accepted=%t, want %t", ok, want.forestOK)
			}
			if ok {
				defer forest.Release()
				inspection, err := benchfixtures.InspectGoTree(forest.RootNode(), goLang)
				if err != nil {
					t.Fatal(err)
				}
				diff := FirstDivergenceDumpV1(forest.RootNode(), goLang, cTree.RootNode())
				if inspection.SHA256 != want.forestDigest || forest.RootNode().HasError() != want.forestError || perlN31sDispatch(forest) != want.forestDispatch || diff != nil {
					t.Fatalf("forest digest=%s error=%t divergence=%+v dispatch=%s", inspection.SHA256, forest.RootNode().HasError(), diff, perlN31sDispatch(forest))
				}
				t.Logf("forest accepted=true digest=%s error=%t divergence=%+v dispatch=%s rewrites=%d", inspection.SHA256, forest.RootNode().HasError(), FirstDivergenceDumpV1(forest.RootNode(), goLang, cTree.RootNode()), perlN31sDispatch(forest), forest.ParseRuntime().NormalizationNodesRewritten)
			} else {
				t.Logf("forest accepted=false")
			}
			base := bytes.TrimSuffix(tc.src, []byte{'\n'})
			incParser := gots.NewParser(goLang)
			oldTree, err := incParser.Parse(base)
			if err != nil {
				t.Fatal(err)
			}
			defer oldTree.Release()
			oldTree.Edit(gots.InputEdit{StartByte: uint32(len(base)), OldEndByte: uint32(len(base)), NewEndByte: uint32(len(tc.src)), StartPoint: perlN31sPoint(base), OldEndPoint: perlN31sPoint(base), NewEndPoint: perlN31sPoint(tc.src)})
			inc, profile, err := incParser.ParseIncrementalProfiled(tc.src, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			defer inc.Release()
			inspection, err := benchfixtures.InspectGoTree(inc.RootNode(), goLang)
			if err != nil {
				t.Fatal(err)
			}
			diff := FirstDivergenceDumpV1(inc.RootNode(), goLang, cTree.RootNode())
			if inspection.SHA256 != want.incrementalDigest || inc.RootNode().HasError() != want.incrementalError || perlN31sDispatch(inc) != want.incrementalDispatch || diff != nil {
				t.Fatalf("incremental digest=%s error=%t divergence=%+v dispatch=%s", inspection.SHA256, inc.RootNode().HasError(), diff, perlN31sDispatch(inc))
			}
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("incremental reuse profile=%+v, want unsupported external scanner and zero reuse", profile)
			}
			t.Logf("incremental digest=%s error=%t divergence=%+v dispatch=%s rewrites=%d profile=%+v", inspection.SHA256, inc.RootNode().HasError(), FirstDivergenceDumpV1(inc.RootNode(), goLang, cTree.RootNode()), perlN31sDispatch(inc), inc.ParseRuntime().NormalizationNodesRewritten, profile)
		})
	}
}

func perlN31sDispatch(tree *gots.Tree) string {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return "none"
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == "dispatch.perl" {
			return fmt.Sprintf("%d/%d/%d/%d", pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten)
		}
	}
	return "none"
}

func perlN31sPoint(source []byte) gots.Point {
	var p gots.Point
	for _, b := range source {
		if b == '\n' {
			p.Row++
			p.Column = 0
		} else {
			p.Column++
		}
	}
	return p
}

type perlN31sRoute struct {
	digest, dispatch string
	error            bool
	diff             *DumpV1Divergence
}

type perlN31sExpectation struct {
	sourceSHA, cDigest, incrementalDigest string
	routes                                map[string]perlN31sRoute
	compactRouted, compactFallback        uint64
	compactReason                         string
	forestOK, forestError                 bool
	forestDigest, forestDispatch          string
	incrementalError                      bool
	incrementalDispatch                   string
}

func perlN31sExpected(name string) perlN31sExpectation {
	baseDiff := &DumpV1Divergence{
		Path:     "/source_file/expression_statement[0]/list_expression[0]",
		Category: "type",
		GoValue:  "list_expression",
		CValue:   "ambiguous_function_call_expression",
	}
	if name == "push_two_args" {
		const digest = "27dac6760d613fe9d554c1f4a73465d5ea5d339098540bd7bce136eead0d3916"
		return perlN31sExpectation{
			sourceSHA: "08ac06c62278aa8bb26361629ac930bbbfbe5031da04a54ee3aeec4875ce0b3b",
			cDigest:   digest, incrementalDigest: digest,
			routes: map[string]perlN31sRoute{
				"raw":        {digest: "f084c77bb5f2c5824dbdf978f68b11f4069b85d9a2dd0a7d11c40ce5e9d2a4d9", diff: baseDiff, dispatch: "none"},
				"production": {digest: digest, dispatch: "1/1/13/4"},
				"compact":    {digest: digest, dispatch: "none"},
			},
			compactRouted: 1, compactFallback: 0,
			forestOK: true, forestDigest: digest, forestDispatch: "1/1/13/0",
			incrementalDispatch: "1/1/13/4",
		}
	}
	const digest = "a18c7dba86442049b19f644c9db50b5d090340065698deb180e7b2088dff408e"
	return perlN31sExpectation{
		sourceSHA: "7be8389f1e6981c2e1e6324357df96ffb34063e9ea6811b8c332143e76015cd1",
		cDigest:   digest, incrementalDigest: digest,
		routes: map[string]perlN31sRoute{
			"raw":        {digest: "6dfa1321087fae3d85fa419198d72c54cf3ee91258e49b1213c64ebf2192e20d", diff: baseDiff, dispatch: "none"},
			"production": {digest: digest, dispatch: "1/1/17/4"},
			"compact":    {digest: digest, dispatch: "none"},
		},
		compactRouted: 1, compactFallback: 0,
		forestOK: true, forestDigest: digest, forestDispatch: "1/1/17/0",
		incrementalDispatch: "1/1/17/4",
	}
}

func perlN31sHashFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(b) == 0 {
		t.Fatalf("evidence file %s is empty", path)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
