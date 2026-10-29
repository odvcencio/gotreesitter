//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestSwiftEOFRecoveryPrefixBoundAndKnownCGap(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_FloatingPointToString.swift"))
	if err != nil {
		t.Fatal(err)
	}
	if len(source) < 16384 {
		t.Fatalf("Swift witness has %d bytes, want at least 16384", len(source))
	}
	goLang := grammars.SwiftLanguage()
	shortTree, err := gotreesitter.NewParser(goLang).Parse(source[:8192])
	if err != nil {
		t.Fatal(err)
	}
	defer shortTree.Release()
	shortInspection, err := benchfixtures.InspectGoTree(shortTree.RootNode(), goLang)
	if err != nil {
		t.Fatal(err)
	}
	const wantShortDigest = "ddc0706b4482b12ec869f44055311a9319d81eb8cb888107e37a3ba4fe0b33de"
	if shortInspection.SHA256 != wantShortDigest || shortTree.ParseRuntime().CRecoverEOFFallbacks == 0 {
		t.Fatalf("Swift 8192-byte prefix changed: digest=%s fallbacks=%d", shortInspection.SHA256, shortTree.ParseRuntime().CRecoverEOFFallbacks)
	}
	source = source[:16384]
	goTree, err := gotreesitter.NewParser(goLang).Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer goTree.Release()
	goInspection, err := benchfixtures.InspectGoTree(goTree.RootNode(), goLang)
	if err != nil {
		t.Fatal(err)
	}
	const wantGoDigest = "afad13bec5eba6bc7712d5ea94f8fc82b9d92fefc6321c53a38f8c43b64ab29a"
	if goInspection.SHA256 != wantGoDigest {
		t.Fatalf("Swift prefix Go digest = %s, want %s", goInspection.SHA256, wantGoDigest)
	}
	runtime := goTree.ParseRuntime()
	if runtime.StopReason != gotreesitter.ParseStopAccepted || runtime.Truncated || runtime.NodesAllocated > 40000 || runtime.CRecoverEOFFallbacks == 0 {
		t.Fatalf("Swift prefix recovery budget: stop=%s truncated=%t nodes=%d fallbacks=%d", runtime.StopReason, runtime.Truncated, runtime.NodesAllocated, runtime.CRecoverEOFFallbacks)
	}
	cLang, err := ParityCLanguage("swift")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked C returned no Swift prefix tree")
	}
	defer cTree.Close()
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	const wantCDigest = "810fcd21eeb7b2c4afccf3e96de9c074d18b870e9d3ae3b90ad01165f532838f"
	if cDigest != wantCDigest {
		t.Fatalf("Swift prefix locked C digest = %s, want %s", cDigest, wantCDigest)
	}
	if goTree.RootNode().Type(goLang) != "source_file" || cTree.RootNode().Kind() != "ERROR" || !goTree.RootNode().HasError() || !cTree.RootNode().HasError() {
		t.Fatalf("Swift prefix known C gap changed: Go=%s/%t C=%s/%t", goTree.RootNode().Type(goLang), goTree.RootNode().HasError(), cTree.RootNode().Kind(), cTree.RootNode().HasError())
	}
	t.Logf("Swift prefix: Go=%s C=%s nodes=%d EOF fallbacks=%d", goInspection.SHA256, cDigest, runtime.NodesAllocated, runtime.CRecoverEOFFallbacks)
}
