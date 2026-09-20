package grammargen

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

// blobReproducibilityCase names one grammargen-owned blob under
// grammars/grammar_blobs/ and the exact recipe that must reproduce it.
// grammars/registry_builtin_gen.go marks a grammar's GrammarSource as
// GrammarSourceGrammargenBlob when cmd/grammargen, not cmd/ts2go, owns the
// shipped artifact. As of 2026-09-20 that is go, regex, and swift; regex and
// swift are excluded below, each with a documented reason.
type blobReproducibilityCase struct {
	name       string
	blobPath   string
	lrSplit    bool
	skipReason string
}

var blobReproducibilityCases = []blobReproducibilityCase{
	{
		name:     "go",
		blobPath: "../grammars/grammar_blobs/go.bin",
		lrSplit:  false,
		// -lr-split is not safe for Go: it interacts with the external
		// `_automatic_semicolon` ASI scanner symbol and produces a table
		// that misparses ordinary single-line if-statements such as
		// `if err != nil { foo() }` (confirmed 2026-09-20; see
		// grammargen/README.md "Go's blob is generated without -lr-split").
	},
	{
		name:       "regex",
		skipReason: "regex.bin is imported from tree-sitter-regex's own grammar.json (ImportGrammarJSON), not a cmd/grammargen builtin grammar function; reproducing it needs an offline clone of the upstream repo (see grammars/languages.lock), which this fast unit test does not perform. Regenerate manually with the tree-sitter-regex checkout used by TestRegexImportCharacterClassRangeParity and cmd/grammargen -json.",
	},
	{
		name:       "swift",
		skipReason: "swift.bin is being regenerated on cypress/swift-scanner-port-20260919; do not touch swift files on other branches while that work is in flight.",
	},
}

// TestGrammargenOwnedBlobsAreReproducible regenerates every grammargen-owned
// blob (grammars/registry_builtin_gen.go: GrammarSource ==
// GrammarSourceGrammargenBlob) with its documented recipe and compares the
// SHA-256 against the shipped file. This is the guard against the class of
// drift found in the go.bin audit before v0.53.0 ("grammargen go.bin not
// rebuildable"): the checked-in blob and the generator silently diverging
// because a later generator change was never re-baked into the shipped
// artifact. A grammar listed above without a skip reason must reproduce
// byte-for-byte; keep the excluded list short and each reason current.
func TestGrammargenOwnedBlobsAreReproducible(t *testing.T) {
	for _, tc := range blobReproducibilityCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipReason != "" {
				t.Skip(tc.skipReason)
			}
			fn, ok := builtinGrammarByName(tc.name)
			if !ok {
				t.Fatalf("grammar %q has no cmd/grammargen builtin entry; add one or add a skip reason", tc.name)
			}
			grammar := fn()
			grammar.EnableLRSplitting = tc.lrSplit
			blob, err := Generate(grammar)
			if err != nil {
				t.Fatalf("generate %s: %v", tc.name, err)
			}
			shipped, err := os.ReadFile(filepath.Join(filepath.FromSlash(tc.blobPath)))
			if err != nil {
				t.Fatalf("read shipped %s blob: %v", tc.name, err)
			}
			if !bytes.Equal(blob, shipped) {
				gotSum := sha256.Sum256(blob)
				wantSum := sha256.Sum256(shipped)
				lrSplitFlag := ""
				if tc.lrSplit {
					lrSplitFlag = "-lr-split "
				}
				t.Fatalf("%s.bin is not reproducible: regenerated sha256=%x shipped sha256=%x\nregenerate with:\n  go run ./cmd/grammargen %s-bin %s %s",
					tc.name, gotSum, wantSum, lrSplitFlag, tc.blobPath, tc.name)
			}
		})
	}
}

// builtinGrammarByName exposes the subset of cmd/grammargen's builtin
// grammar table this package can construct directly, without importing
// package main. Keep it in sync with the grammars this test covers, not
// with cmd/grammargen's full list.
func builtinGrammarByName(name string) (func() *Grammar, bool) {
	switch name {
	case "go":
		return GoGrammar, true
	case "swift":
		return SwiftGrammar, true
	default:
		return nil, false
	}
}
