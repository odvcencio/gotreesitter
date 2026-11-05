//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// The original frozen issue-490 construction joins declarations with }func.
// Preserve those malformed bytes beside the valid newline-separated witness;
// both must match the complete locked-C tree on each fresh Go route.
func TestGoIssue490FragmentBoundaryLockedC(t *testing.T) {
	fragment, err := os.ReadFile("../testdata/issue490_go_fragment.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(fragment) != 8328 || fmt.Sprintf("%x", sha256.Sum256(fragment)) != "02082426492c552453551eacc0b7435e6203995dc0a94df50e56dc0d2ce92aa6" {
		t.Fatal("frozen issue-490 fragment identity changed")
	}
	build := func(separate bool) []byte {
		source := append([]byte("package p\n"), fragment...)
		for index := 1; index < 8; index++ {
			if separate {
				source = append(source, '\n')
			}
			source = append(source, fragment...)
		}
		return source
	}
	language := grammars.GoLanguage()
	if language == nil {
		t.Fatal("Go grammar unavailable")
	}
	cLanguage := loadCanonicalGoCLanguage(t)
	for _, witness := range []struct {
		name         string
		source       []byte
		bytes        int
		sha256       string
		wantHasError bool
	}{
		{"original_joined", build(false), 66634, "033c1b7b8aa11f8bf5ab508c2619fa05a56549ba2529d2f7279c12685d5ea3fd", true},
		{"newline_separated", build(true), 66641, "3831fedd55f01e312aa7133d92419dad9002bb7fe27d51c62545ada15031730b", false},
	} {
		t.Run(witness.name, func(t *testing.T) {
			if len(witness.source) != witness.bytes || fmt.Sprintf("%x", sha256.Sum256(witness.source)) != witness.sha256 {
				t.Fatalf("issue-490 witness changed: bytes=%d SHA-256=%x", len(witness.source), sha256.Sum256(witness.source))
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(witness.source, nil)
			if ct == nil || ct.RootNode() == nil {
				t.Fatal("locked C returned no tree")
			}
			defer ct.Close()
			if got := ct.RootNode().HasError(); got != witness.wantHasError {
				t.Fatalf("locked C HasError=%t, want %t", got, witness.wantHasError)
			}
			if got := ct.RootNode().EndByte(); got != uint(witness.bytes) {
				t.Fatalf("locked C root ends at %d, want %d", got, witness.bytes)
			}
			for _, route := range []struct {
				name      string
				candidate bool
			}{{"production", false}, {"candidate", true}} {
				t.Run(route.name, func(t *testing.T) {
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(route.candidate)
					gt, err := parser.Parse(witness.source)
					if err != nil {
						t.Fatal(err)
					}
					if gt == nil || gt.RootNode() == nil {
						t.Fatal("fresh Go returned no tree")
					}
					defer gt.Release()
					if gt.ParseStoppedEarly() {
						t.Fatalf("fresh Go stopped early: %s", gt.ParseRuntime().Summary())
					}
					if got := gt.RootNode().HasError(); got != witness.wantHasError {
						t.Fatalf("fresh Go HasError=%t, locked C=%t", got, witness.wantHasError)
					}
					assertLockedCTreeExactWithErrors(t, "issue-490 "+witness.name+" "+route.name, gt, language, ct)
					t.Logf("source_bytes=%d source_sha256=%s C_HasError=%t candidate=%t", witness.bytes, witness.sha256, witness.wantHasError, route.candidate)
				})
			}
		})
	}
}
