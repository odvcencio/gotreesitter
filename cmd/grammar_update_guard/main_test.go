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
