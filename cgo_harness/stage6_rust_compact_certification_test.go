//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	"bytes"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestStage6RustCompactCertification proves clean, scanner, recovery, and
// incremental Rust shapes against the locked C oracle.
func TestStage6RustCompactCertification(t *testing.T) {
	cases := []struct {
		name           string
		source         []byte
		wantRoute      bool
		wantReasonPart string
	}{
		{name: "smoke", source: []byte(grammars.ParseSmokeSample("rust")), wantRoute: true},
		{name: "trait_impl", source: []byte("trait T { fn f(&self); }\nstruct S;\nimpl T for S { fn f(&self) {} }\n"), wantRoute: true},
		{name: "match", source: []byte("fn f(x: Option<i32>) -> i32 { match x { Some(v) => v, None => 0 } }\n"), wantRoute: true},
		{name: "raw_string", source: []byte("fn main() { let raw = r###\"hello\"###; let after = 3; }\n"), wantRoute: true},
		{name: "recovery", source: []byte("fn f( {\n let x = 1;\n}\n"), wantReasonPart: "recovery"},
	}
	language := grammars.RustLanguage()
	if reusable, ok := language.ExternalScanner.(gotreesitter.IncrementalReuseExternalScanner); !ok || !reusable.SupportsIncrementalReuse() {
		t.Fatal("Rust scanner does not advertise incremental reuse")
	}
	if checkpointed, ok := language.ExternalScanner.(gotreesitter.CheckpointedExternalScanner); !ok || !checkpointed.UsesExternalScannerCheckpoints() {
		t.Fatal("Rust scanner does not advertise scanner checkpoints")
	}
	cLanguage, err := ParityCLanguage("rust")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			defer cParser.Close()
			cTree := cParser.Parse(tc.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parser returned no tree")
			}
			defer cTree.Close()

			parser := gotreesitter.NewParser(language)
			parser.SetAdmissionCandidateRoute(true)
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			goTree, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			routedDelta := routedAfter - routedBefore
			fallbackDelta := fallbackAfter - fallbackBefore
			reason := gotreesitter.AdmissionCandidateLastFallbackReason()
			t.Logf("route=%d/%d reason=%q", routedDelta, fallbackDelta, reason)

			if tc.wantRoute {
				if routedDelta != 1 || fallbackDelta != 0 {
					t.Fatalf("clean case did not route through compact admission: %d/%d", routedDelta, fallbackDelta)
				}
				assertLockedCTreeExact(t, "Rust compact "+tc.name, goTree, language, cTree)
			} else {
				if routedDelta != 0 || fallbackDelta != 1 {
					t.Fatalf("recovery case did not fall back once: %d/%d", routedDelta, fallbackDelta)
				}
				if tc.wantReasonPart != "" && !strings.Contains(reason, tc.wantReasonPart) {
					t.Fatalf("fallback reason=%q, want substring %q", reason, tc.wantReasonPart)
				}
				if goTree.RootNode().HasError() != cTree.RootNode().HasError() {
					t.Fatalf("Rust %s root error differs: Rust=%v C=%v", tc.name, goTree.RootNode().HasError(), cTree.RootNode().HasError())
				}
				if diff := FirstDivergenceDumpV1(goTree.RootNode(), language, cTree.RootNode()); diff != nil {
					t.Fatalf("Rust %s error tree diverges from locked C: %+v", tc.name, diff)
				}
			}
		})
	}

	for _, tc := range []struct {
		name string
		src  []byte
		old  []byte
		new  []byte
	}{
		{name: "smoke", src: []byte(grammars.ParseSmokeSample("rust")), old: []byte("1"), new: []byte("2")},
		{name: "raw_string_after", src: []byte("fn main() { let raw = r###\"hello\"###; let after = 3; }\n"), old: []byte("3"), new: []byte("4")},
		{name: "raw_string_content", src: []byte("fn main() { let raw = r###\"hello\"###; let after = 3; }\n"), old: []byte("hello"), new: []byte("hullo")},
	} {
		t.Run("incremental_"+tc.name, func(t *testing.T) {
			oldTreeParser := gotreesitter.NewParser(language)
			oldTree, err := oldTreeParser.Parse(tc.src)
			if err != nil {
				t.Fatal(err)
			}
			defer oldTree.Release()
			offset := bytes.Index(tc.src, tc.old)
			if offset < 0 || len(tc.old) != len(tc.new) {
				t.Fatalf("invalid edit witness old=%q", tc.old)
			}
			edited := append([]byte(nil), tc.src...)
			copy(edited[offset:offset+len(tc.new)], tc.new)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + len(tc.old)),
				NewEndByte:  uint32(offset + len(tc.new)),
				StartPoint:  pointAtOffset(tc.src, offset),
				OldEndPoint: pointAtOffset(tc.src, offset+len(tc.old)),
				NewEndPoint: pointAtOffset(edited, offset+len(tc.new)),
			})
			incremental, profile, err := oldTreeParser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			t.Logf("profile=%+v", profile)
			if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
				t.Fatalf("Rust incremental certification did not reuse the clean prefix: %+v", profile)
			}

			cParser := sitter.NewParser()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			defer cParser.Close()
			cTree := cParser.Parse(edited, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parser returned no edited tree")
			}
			defer cTree.Close()
			assertLockedCTreeExact(t, "Rust compact incremental "+tc.name, incremental, language, cTree)
		})
	}
}
