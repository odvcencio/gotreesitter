package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExternalName(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "string", raw: `"_automatic_semicolon"`, want: "_automatic_semicolon"},
		{name: "name object", raw: `{"type":"SYMBOL","name":"safe_nav"}`, want: "safe_nav"},
		{name: "value object", raw: `{"type":"STRING","value":"else"}`, want: "else"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := externalName(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("externalName: %v", err)
			}
			if got != tc.want {
				t.Fatalf("externalName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShouldCheck(t *testing.T) {
	cases := []struct {
		name string
		in   updateResult
		want bool
	}{
		{name: "applied", in: updateResult{RepoURL: "https://example.test/repo", NewRef: "abc", Applied: true}, want: true},
		{name: "available", in: updateResult{RepoURL: "https://example.test/repo", NewRef: "abc", Status: updateStatusAvailable}, want: true},
		{name: "unchanged", in: updateResult{RepoURL: "https://example.test/repo", NewRef: "abc"}, want: false},
		{name: "missing ref", in: updateResult{RepoURL: "https://example.test/repo", Applied: true}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldCheck(tc.in); got != tc.want {
				t.Fatalf("shouldCheck = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestWriteBlockedList verifies the guard emits exactly the blocked names,
// one per line, in report order — this file becomes grammar_updater's
// -exclude-list, so it must never include a name the guard cleared.
func TestWriteBlockedList(t *testing.T) {
	report := &guardReport{
		Results: []guardResult{
			{Name: "go", Blocked: false},
			{Name: "kotlin", Blocked: true, Reasons: []string{"src/scanner.c changed"}},
			{Name: "rust", Blocked: false},
			{Name: "swift", Blocked: true, Reasons: []string{"external token list changed"}},
		},
	}

	path := filepath.Join(t.TempDir(), "held_back.txt")
	if err := writeBlockedList(path, report); err != nil {
		t.Fatalf("writeBlockedList: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read blocked list: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{"kotlin", "swift"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("blocked list = %v, want %v", got, want)
	}
}

// TestWriteBlockedListEmpty verifies an all-clear guard run writes an empty
// file rather than omitting it, so the workflow's exclude-list step is
// unconditional.
func TestWriteBlockedListEmpty(t *testing.T) {
	report := &guardReport{Results: []guardResult{{Name: "go", Blocked: false}}}

	path := filepath.Join(t.TempDir(), "held_back.txt")
	if err := writeBlockedList(path, report); err != nil {
		t.Fatalf("writeBlockedList: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read blocked list: %v", err)
	}
	if strings.TrimSpace(string(data)) != "" {
		t.Fatalf("expected empty blocked list, got %q", string(data))
	}
}

func TestSafeDirName(t *testing.T) {
	got := []string{
		safeDirName("Swift"),
		safeDirName("c-sharp"),
		safeDirName(""),
	}
	want := []string{"swift", "c_sharp", "grammar"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("safeDirName values = %v, want %v", got, want)
	}
}

// TestApplyGrammarDiffScannerFacingChanges is the regression test for the
// 2026-09-20 incident: the guard cleared c_sharp, cmake, and yaml as
// scanner-safe even though src/scanner.c changed upstream for all three
// (and c_sharp also gained a new external token, "_lambda_paren_open").
// The root cause was that checkUpdate only inspected a grammar's
// scanner-facing files when a hand-curated ExternalScannerSpec had been
// registered for it (grammars/runtime/*_scanner.go); c_sharp, cmake, and
// yaml never got one, so the old guard returned "not blocked" without ever
// fetching or diffing anything. applyGrammarDiff instead always diffs the
// old ref against the new ref directly, so it does not depend on that
// registration existing.
//
// Fixtures live under testdata/ as small synthetic file trees (not vendored
// upstream sources) that reproduce each pair's shape: which files changed,
// and whether the externals array changed.
func TestApplyGrammarDiffScannerFacingChanges(t *testing.T) {
	cases := []struct {
		name             string
		fixture          string
		wantBlocked      bool
		wantReasonSubs   []string
		wantNoReasonSubs []string
	}{
		{
			// tree-sitter/tree-sitter-c-sharp 88366631d598 -> 9150f7d56bb4.
			name:           "c_sharp 2026-09-20: scanner.c changed and a new external token was added",
			fixture:        "c_sharp_20260920",
			wantBlocked:    true,
			wantReasonSubs: []string{"src/scanner.c changed", "external token list changed"},
		},
		{
			// uyha/tree-sitter-cmake c7b2a71e7f8e -> 58993af75218.
			name:             "cmake 2026-09-20: scanner.c changed, externals unchanged",
			fixture:          "cmake_20260920",
			wantBlocked:      true,
			wantReasonSubs:   []string{"src/scanner.c changed"},
			wantNoReasonSubs: []string{"external token list changed"},
		},
		{
			// tree-sitter-grammars/tree-sitter-yaml 4463985dfccc -> a1c4812a73ec.
			name:             "yaml 2026-09-20: scanner.c changed, externals unchanged",
			fixture:          "yaml_20260920",
			wantBlocked:      true,
			wantReasonSubs:   []string{"src/scanner.c changed"},
			wantNoReasonSubs: []string{"external token list changed"},
		},
		{
			// A true no-change pair, for example tree-sitter/tree-sitter-c
			// ae19b676 -> b780e47f: only the generated parser.c metadata
			// changed upstream. The guard never reads parser.c, so this must
			// clear.
			name:        "c true no-change pair: only parser.c metadata changed",
			fixture:     "c_no_change",
			wantBlocked: false,
		},
		{
			name:           "grammar gains an external scanner between refs",
			fixture:        "scanner_added",
			wantBlocked:    true,
			wantReasonSubs: []string{"src/scanner.c added", "external token list changed"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldDir := filepath.Join("testdata", tc.fixture, "old")
			newDir := filepath.Join("testdata", tc.fixture, "new")
			result := &guardResult{Name: tc.fixture}
			applyGrammarDiff(result, oldDir, newDir, "src")

			if result.Blocked != tc.wantBlocked {
				t.Fatalf("Blocked = %v, want %v (reasons: %v)", result.Blocked, tc.wantBlocked, result.Reasons)
			}
			for _, sub := range tc.wantReasonSubs {
				if !reasonsContain(result.Reasons, sub) {
					t.Fatalf("reasons %v missing expected substring %q", result.Reasons, sub)
				}
			}
			for _, sub := range tc.wantNoReasonSubs {
				if reasonsContain(result.Reasons, sub) {
					t.Fatalf("reasons %v unexpectedly contain substring %q", result.Reasons, sub)
				}
			}
		})
	}
}

// TestApplyRecoveryGateDiff is task #71 item 2's fixture: a routine grammar
// blob regeneration must not silently flip a language's C-recovery
// cost-competition default. The doxygen case reproduces pine's 2026-09-21
// diagnosis directly with real blobs: the shipped blob has zero
// ExternalLexStates rows (gate off), and the candidate — regenerated from
// the same locked parser.c with `go run ./cmd/ts2go` — restores the 8 rows
// the grammar's external scanner declares, which flips the gate on with no
// recovery-board evidence. The json case is the no-change control: an
// identical shipped and candidate blob must never block.
func TestApplyRecoveryGateDiff(t *testing.T) {
	cases := []struct {
		name           string
		fixture        string
		grammar        string
		wantBlocked    bool
		wantReasonSubs []string
	}{
		{
			name:        "doxygen: candidate blob restores the locked parser.c's 8 ExternalLexStates rows",
			fixture:     "recovery_gate_doxygen",
			grammar:     "doxygen",
			wantBlocked: true,
			wantReasonSubs: []string{
				"recovery gate would change",
				"external_lex_state_rows 0->8",
			},
		},
		{
			name:        "json: shipped and candidate blobs are identical",
			fixture:     "recovery_gate_no_change",
			grammar:     "json",
			wantBlocked: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shippedPath := filepath.Join("testdata", tc.fixture, "shipped", tc.grammar+".bin")
			candidatePath := filepath.Join("testdata", tc.fixture, "candidate", tc.grammar+".bin")
			result := &guardResult{Name: tc.grammar}
			applyRecoveryGateDiff(result, shippedPath, candidatePath)

			if result.Blocked != tc.wantBlocked {
				t.Fatalf("Blocked = %v, want %v (reasons: %v)", result.Blocked, tc.wantBlocked, result.Reasons)
			}
			for _, sub := range tc.wantReasonSubs {
				if !reasonsContain(result.Reasons, sub) {
					t.Fatalf("reasons %v missing expected substring %q", result.Reasons, sub)
				}
			}
		})
	}
}

// TestApplyRecoveryGateDiffNoCandidateIsNoOp covers the normal case: a plain
// grammars/languages.lock ref bump never regenerates a blob, so
// checkUpdate's candidateBlobDir is empty and applyRecoveryGateDiff's
// candidate path never exists. It must never block on a missing candidate.
func TestApplyRecoveryGateDiffNoCandidateIsNoOp(t *testing.T) {
	result := &guardResult{Name: "doxygen"}
	applyRecoveryGateDiff(
		result,
		filepath.Join("testdata", "recovery_gate_doxygen", "shipped", "doxygen.bin"),
		filepath.Join("testdata", "recovery_gate_doxygen", "candidate", "does_not_exist.bin"),
	)
	if result.Blocked {
		t.Fatalf("Blocked = true with no candidate blob available; want false (reasons: %v)", result.Reasons)
	}
}

// TestApplyGrammarDiffFollowsScannerIncludes covers a shim scanner.c whose
// entire body is a quoted #include reaching outside the grammar's own
// subdir, the tree-sitter-ocaml shape: grammars/ocaml/src/scanner.c never
// changes, only the shared common/scanner.h it #includes does. Before
// applyGrammarDiff followed #include chains, hashing scanner.c alone saw no
// change here and cleared the grammar, even though the scanner's real
// behavior moved.
func TestApplyGrammarDiffFollowsScannerIncludes(t *testing.T) {
	oldDir := filepath.Join("testdata", "ocaml_include_shim_20260920", "old")
	newDir := filepath.Join("testdata", "ocaml_include_shim_20260920", "new")
	result := &guardResult{Name: "ocaml_include_shim_20260920"}
	applyGrammarDiff(result, oldDir, newDir, "grammars/ocaml/src")

	if !result.Blocked {
		t.Fatalf("Blocked = false, want true (the included common/scanner.h changed); reasons: %v", result.Reasons)
	}
	if reasonsContain(result.Reasons, "grammars/ocaml/src/scanner.c changed") {
		t.Fatalf("reasons %v claim scanner.c itself changed; the fixture's scanner.c shim is byte-identical on both refs", result.Reasons)
	}
	if !reasonsContain(result.Reasons, "common/scanner.h changed") {
		t.Fatalf("reasons %v missing the included common/scanner.h change", result.Reasons)
	}
	if reasonsContain(result.Reasons, "external token list changed") {
		t.Fatalf("reasons %v unexpectedly report an externals change; the fixture keeps grammar.json identical on both refs so the include chain is the only signal", result.Reasons)
	}

	var sawIncluded bool
	for _, fr := range result.SourceFiles {
		if fr.Path == "common/scanner.h" {
			sawIncluded = true
			if !fr.Changed {
				t.Fatalf("common/scanner.h source-file entry: Changed = false, want true: %+v", fr)
			}
		}
	}
	if !sawIncluded {
		t.Fatalf("result.SourceFiles %+v does not report common/scanner.h at all", result.SourceFiles)
	}
}

// TestResolveIncludedFiles exercises the #include-chain walk directly: it
// must follow a quoted include out of the grammar's own subdir, refuse to
// follow one that would resolve outside the checkout root, and ignore an
// angle-bracket include entirely.
func TestResolveIncludedFiles(t *testing.T) {
	root := filepath.Join("testdata", "ocaml_include_shim_20260920", "old")
	got := resolveIncludedFiles(root, "grammars/ocaml/src/scanner.c")
	want := []string{"grammars/ocaml/src/scanner.c", "common/scanner.h"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolveIncludedFiles = %v, want %v", got, want)
	}
}

func TestPathWithinRoot(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"common/scanner.h", true},
		{"scanner.h", true},
		{"..", false},
		{"../secrets", false},
		{"../../etc/passwd", false},
	}
	for _, tc := range cases {
		if got := pathWithinRoot(tc.rel); got != tc.want {
			t.Errorf("pathWithinRoot(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

func TestQuotedIncludePaths(t *testing.T) {
	src := []byte(`#include "../../../common/scanner.h"
#include <stdbool.h>
  #include   "local.h"
// #include "commented_out.h"
`)
	got := quotedIncludePaths(src)
	want := []string{"../../../common/scanner.h", "local.h"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quotedIncludePaths = %v, want %v", got, want)
	}
}

func reasonsContain(reasons []string, substr string) bool {
	for _, r := range reasons {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}

// TestDiffSourceFile covers the three-way outcome diffSourceFile must
// distinguish: absent on both refs (not scanner-facing), present on both
// with the same content (unchanged), and a presence or content mismatch
// (scanner-facing change).
func TestDiffSourceFile(t *testing.T) {
	oldDir := filepath.Join("testdata", "scanner_added", "old")
	newDir := filepath.Join("testdata", "scanner_added", "new")

	if fr, changed := diffSourceFile(oldDir, newDir, filepath.Join("src", "scanner.cc")); fr != nil || changed {
		t.Fatalf("scanner.cc absent on both refs: got fr=%+v changed=%v, want nil/false", fr, changed)
	}

	fr, changed := diffSourceFile(oldDir, newDir, filepath.Join("src", "scanner.c"))
	if fr == nil {
		t.Fatal("scanner.c present on new ref only: got nil result")
	}
	if !changed {
		t.Fatal("scanner.c added between refs: want changed=true")
	}
	if !fr.MissingOld || fr.MissingNew {
		t.Fatalf("scanner.c presence flags = missingOld:%v missingNew:%v, want missingOld:true missingNew:false", fr.MissingOld, fr.MissingNew)
	}

	sameDirFr, sameDirChanged := diffSourceFile(newDir, newDir, filepath.Join("src", "scanner.c"))
	if sameDirFr == nil || sameDirChanged {
		t.Fatalf("identical file on both sides: got fr=%+v changed=%v, want unchanged", sameDirFr, sameDirChanged)
	}
}
