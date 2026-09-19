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
