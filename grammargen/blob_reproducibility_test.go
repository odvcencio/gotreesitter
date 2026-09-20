package grammargen

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// blobReproducibilityCase names one grammargen-owned blob under
// grammars/grammar_blobs/ and the exact recipe that must reproduce it.
// grammars/registry_builtin_gen.go marks a grammar's GrammarSource as
// GrammarSourceGrammargenBlob when cmd/grammargen, not cmd/ts2go, owns the
// shipped artifact. As of 2026-09-20 that is go, regex, swift, and yaml.
type blobReproducibilityCase struct {
	name     string
	blobPath string
	lrSplit  bool
	// jsonPath, when set, builds the grammar by importing a resolved
	// tree-sitter grammar.json from this path (relative to this package)
	// instead of calling a cmd/grammargen builtin grammar function.
	jsonPath   string
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
		// regex.bin imports tree-sitter-regex's own resolved grammar.json
		// (ImportGrammarJSON), not a cmd/grammargen builtin grammar
		// function; see TestRegexImportCharacterClassRangeParity for the
		// same import path. testdata/regex_upstream_grammar.json is a
		// pinned copy of that file so this test does not need network
		// access or an offline clone of grammars/languages.lock's "regex"
		// entry to run.
		//
		// Pinned source: github.com/tree-sitter/tree-sitter-regex, commit
		// b2ac15e27fce703d2f37a79ccd94a5c0cbe9720b (tag 0.25.0, matches
		// grammars/languages.lock), file src/grammar.json,
		// sha256=a2e6cef007b68b11ea646d866e58747686b167c24789c1092958e9c77b27c6f6,
		// MIT license (see that repo's LICENSE).
		//
		// No extra Grammar fields (BinaryRepeatMode, EnableLRSplitting) are
		// set: confirmed 2026-09-20 by rebuilding this exact grammar.json
		// with the historical generator from the commit that shipped this
		// blob (1f8b714d0), which reproduces it byte-for-byte with plain
		// ImportGrammarJSON + Generate. regex.bin was regenerated on
		// 2026-09-20 with today's generator using that same plain recipe;
		// the only change versus the June blob is table size, from state
		// minimization passes (grammargen/dfa_minimize.go,
		// grammargen/lr_state_minimize.go) added after the June bake.
		name:     "regex",
		blobPath: "../grammars/grammar_blobs/regex.bin",
		jsonPath: "testdata/regex_upstream_grammar.json",
	},
	{
		name:     "swift",
		blobPath: "../grammars/grammar_blobs/swift.bin",
		lrSplit:  false,
	},
	{
		name:     "yaml",
		blobPath: "../grammars/grammar_blobs/yaml.bin",
		lrSplit:  false,
	},
}

// TestGrammargenOwnedBlobsAreReproducible regenerates every grammargen-owned
// blob (grammars/registry_builtin_gen.go: GrammarSource ==
// GrammarSourceGrammargenBlob) with its documented recipe and compares the
// decoded *gotreesitter.Language against the shipped file. This is the guard
// against the class of drift found in the go.bin audit before v0.53.0
// ("grammargen go.bin not rebuildable"): the checked-in blob and the
// generator silently diverging because a later generator change was never
// re-baked into the shipped artifact. A case without a skip reason must
// reproduce every table field. Add a skip reason only when reproducing a
// blob needs a resource this test cannot obtain on its own (for example
// network access), and state exactly what is missing.
//
// This compares decoded structs, not raw bytes. encoding/gob assigns each
// concrete struct type's wire type ID from a process-global, monotonically
// increasing counter the first time that type crosses any Encoder in the
// process, so the *byte encoding* of an unchanged Language can still differ
// depending on what else this test binary gob-encoded first (confirmed
// 2026-09-20: running TestEncodeLanguageBlobDeterministicWithLargeStateGotos
// before this test changes go.bin's regenerated SHA-256, but every decoded
// field, including ParseTable, SmallParseTable, ParseActions, and LexStates,
// stays identical). A raw byte comparison would make this test's pass/fail
// depend on unrelated test execution order; TestYAMLOwnedGrammarGeneratesCompactBlob
// had that exposure until 2026-09-20, when it moved to the same decoded
// comparison this test uses.
func TestGrammargenOwnedBlobsAreReproducible(t *testing.T) {
	for _, tc := range blobReproducibilityCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipReason != "" {
				t.Skip(tc.skipReason)
			}
			grammar, regenerateHint, err := buildReproducibilityGrammar(tc)
			if err != nil {
				t.Fatalf("build %s grammar: %v", tc.name, err)
			}
			blob, err := Generate(grammar)
			if err != nil {
				t.Fatalf("generate %s: %v", tc.name, err)
			}
			got, err := decodeLanguageBlob(blob)
			if err != nil {
				t.Fatalf("decode regenerated %s blob: %v", tc.name, err)
			}
			shippedBytes, err := os.ReadFile(filepath.Join(filepath.FromSlash(tc.blobPath)))
			if err != nil {
				t.Fatalf("read shipped %s blob: %v", tc.name, err)
			}
			want, err := decodeLanguageBlob(shippedBytes)
			if err != nil {
				t.Fatalf("decode shipped %s blob: %v", tc.name, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s.bin is not reproducible: decoded Language differs from the shipped blob (regenerated sha256=%x, shipped sha256=%x; a differing sha256 alone is not conclusive, see the gob type-ID note above)\nregenerate with:\n  %s",
					tc.name, sha256.Sum256(blob), sha256.Sum256(shippedBytes), regenerateHint)
			}
		})
	}
}

// buildReproducibilityGrammar constructs the *Grammar a case's recipe
// describes, plus the shell command that reproduces its regenerate step, for
// use in a failure message.
func buildReproducibilityGrammar(tc blobReproducibilityCase) (*Grammar, string, error) {
	if tc.jsonPath != "" {
		source, err := os.ReadFile(filepath.FromSlash(tc.jsonPath))
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", tc.jsonPath, err)
		}
		grammar, err := ImportGrammarJSON(source)
		if err != nil {
			return nil, "", fmt.Errorf("import %s: %w", tc.jsonPath, err)
		}
		grammar.EnableLRSplitting = tc.lrSplit
		return grammar, fmt.Sprintf("go run ./cmd/grammargen -json grammargen/%s -bin %s", tc.jsonPath, tc.blobPath), nil
	}
	fn, ok := builtinGrammarByName(tc.name)
	if !ok {
		return nil, "", fmt.Errorf("grammar %q has no cmd/grammargen builtin entry and no jsonPath; add one or add a skip reason", tc.name)
	}
	grammar := fn()
	grammar.EnableLRSplitting = tc.lrSplit
	lrSplitFlag := ""
	if tc.lrSplit {
		lrSplitFlag = "-lr-split "
	}
	return grammar, fmt.Sprintf("go run ./cmd/grammargen %s-bin %s %s", lrSplitFlag, tc.blobPath, tc.name), nil
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
	case "yaml":
		return YAMLGrammar, true
	default:
		return nil, false
	}
}
