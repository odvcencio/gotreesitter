//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	haskellSmallSourceSHA256 = "c60b55de99836dacb00a0f2808835895132996a95d52304cefab548b8cbdef65"
	haskellSmallDeepSHA256   = "6e3806c54c39af93701157f87ac8ee9ef947108d66b7d6069d6908a4d5c71e9f"
)

const haskellSmallSource = `module Main (main) where

import Data.Aeson.Encode.Pretty (encodePretty)
import Data.ByteString.Lazy.Char8 qualified as LBS
import Hasura.Server.MetadataOpenAPI (metadataOpenAPI)
import Prelude

main :: IO ()
main = LBS.putStrLn $ encodePretty metadataOpenAPI
`

func TestHaskellSmallAdmissionLockedCParity(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	source := []byte(haskellSmallSource)
	if len(source) != 260 {
		t.Fatalf("source bytes=%d, want 260", len(source))
	}
	sum := sha256.Sum256(source)
	if got := hex.EncodeToString(sum[:]); got != haskellSmallSourceSHA256 {
		t.Fatalf("source SHA-256=%s, want %s", got, haskellSmallSourceSHA256)
	}

	language := grammars.HaskellLanguage()
	if language == nil {
		t.Fatal("Haskell Go language is nil")
	}
	cLanguage, err := COracleLanguage("haskell")
	if err != nil {
		t.Fatalf("load locked Haskell language: %v", err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set locked Haskell language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked Haskell parser returned no tree")
	}
	t.Cleanup(cTree.Close)
	requireHaskellSmallCRoot(t, cTree.RootNode(), len(source))

	productionParser := gotreesitter.NewParser(language)
	productionParser.SetAdmissionCandidateRoute(false)
	productionTree, err := productionParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	t.Cleanup(func() { productionTree.Release() })
	requireHaskellSmallRoot(t, "production", productionTree.RootNode(), len(source))
	assertLockedCTreeExact(t, "Haskell small production", productionTree, language, cTree)

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	candidateParser := gotreesitter.NewParser(language)
	candidateParser.SetAdmissionCandidateRoute(true)
	candidateTree, err := candidateParser.Parse(source)
	if err != nil {
		t.Fatalf("compact candidate parse: %v", err)
	}
	t.Cleanup(func() { candidateTree.Release() })
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter-routedBefore != 1 || fallbackAfter-fallbackBefore != 0 {
		t.Fatalf("compact route delta=%d/%d reason=%q, want 1/0", routedAfter-routedBefore, fallbackAfter-fallbackBefore, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	requireHaskellSmallRoot(t, "compact candidate", candidateTree.RootNode(), len(source))
	assertLockedCTreeExact(t, "Haskell small compact candidate", candidateTree, language, cTree)

	productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect production deep tree: %v", err)
	}
	candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect compact deep tree: %v", err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect locked C deep tree: %v", err)
	}
	if productionInspection.SHA256 != haskellSmallDeepSHA256 || candidateInspection.SHA256 != haskellSmallDeepSHA256 || cDigest != haskellSmallDeepSHA256 {
		t.Fatalf("Haskell small deep digest production=%s compact=%s locked_c=%s, want %s", productionInspection.SHA256, candidateInspection.SHA256, cDigest, haskellSmallDeepSHA256)
	}
	t.Logf("Haskell small exact locked-C parity deep_digest=%s route_delta=%d/%d", cDigest, routedAfter-routedBefore, fallbackAfter-fallbackBefore)
}

func requireHaskellSmallRoot(t *testing.T, label string, root *gotreesitter.Node, sourceLen int) {
	t.Helper()
	if root == nil {
		t.Fatalf("%s root is nil", label)
	}
	if root.HasError() {
		t.Fatalf("%s root has an error", label)
	}
	if root.StartByte() != 0 || root.EndByte() != uint32(sourceLen) {
		t.Fatalf("%s root span=%d..%d, want 0..%d", label, root.StartByte(), root.EndByte(), sourceLen)
	}
}

func requireHaskellSmallCRoot(t *testing.T, root *sitter.Node, sourceLen int) {
	t.Helper()
	if root == nil {
		t.Fatal("locked C root is nil")
	}
	if root.HasError() {
		t.Fatal("locked C root has an error")
	}
	if root.StartByte() != 0 || root.EndByte() != uint(sourceLen) {
		t.Fatalf("locked C root span=%d..%d, want 0..%d", root.StartByte(), root.EndByte(), sourceLen)
	}
}
