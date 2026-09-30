//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// The trailing space in issue #1340 is significant: C's error-mode external
// scanner consumes it as padding before a zero-width line-ending token. That
// lookahead permits a hidden missing integer terminal inside the table's pair.
func TestTOMLUnfinishedPairLockedC(t *testing.T) {
	lang := grammars.TomlLanguage()
	cLang, err := ParityCLanguage("toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"[session]\nvalue = 0\nhalf = ",
		"[s]\na=", "[s]\na= ", "a=", "a= ",
		"[s]\na=\n", "[s]\na=\r\n", "[s]\na=\t", "[s]\na= \t ",
		"[t.a]\nx= ", "[[t]]\nx= ",
	} {
		for _, compact := range []bool{false, true} {
			t.Run(fmt.Sprintf("%q/compact=%t", source, compact), func(t *testing.T) {
				input := []byte(source)
				parser := gts.NewParser(lang)
				parser.SetAdmissionCandidateRoute(compact)
				tree, err := parser.Parse(input)
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				assertErrorModeExternalRecoveryLockedC(t, tree, lang, cLang, input)
				t.Log(tree.ParseRuntime().Summary())
				allocs := testing.AllocsPerRun(5, func() {
					next, parseErr := parser.ParseIncremental(input, tree)
					if parseErr != nil {
						t.Fatal(parseErr)
					}
					next.Release()
				})
				if allocs != 0 {
					t.Fatalf("no-edit allocations=%g, want 0", allocs)
				}
			})
		}
	}
}

func TestTOMLUnfinishedPairIncrementalLockedC(t *testing.T) {
	lang := grammars.TomlLanguage()
	cLang, err := ParityCLanguage("toml")
	if err != nil {
		t.Fatal(err)
	}
	sources := []string{
		"[session]\nvalue = 0\nhalf = 0",
		"[session]\nvalue = 0\nhalf = ",
		"[session]\nvalue = 0\nhalf =",
		"[session]\nvalue = 0\nhalf =\n",
		"[session]\nvalue = 0\nhalf = \t ",
		"[session]\nvalue = 0\nhalf = 1\n",
	}
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(compact)
			source := []byte(sources[0])
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { tree.Release() }()
			assertErrorModeExternalRecoveryLockedC(t, tree, lang, cLang, source)
			for step, nextSource := range sources[1:] {
				edited := []byte(nextSource)
				start := 0
				for start < len(source) && start < len(edited) && source[start] == edited[start] {
					start++
				}
				tree.Edit(gts.InputEdit{
					StartByte: uint32(start), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited)),
					StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, len(source)), NewEndPoint: pointAtOffset(edited, len(edited)),
				})
				next, err := parser.ParseIncremental(edited, tree)
				if err != nil {
					t.Fatalf("step %d: %v", step, err)
				}
				tree.Release()
				tree = next
				source = edited
				assertErrorModeExternalRecoveryLockedC(t, tree, lang, cLang, source)
				fresh, err := parser.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				incrementalInspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				freshInspection, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				fresh.Release()
				if err != nil {
					t.Fatal(err)
				}
				if incrementalInspection.SHA256 != freshInspection.SHA256 {
					t.Fatalf("step %d: incremental=%s fresh=%s", step, incrementalInspection.SHA256, freshInspection.SHA256)
				}
			}
		})
	}
}

func TestErrorModeExternalRetryRONLockedC(t *testing.T) {
	source, err := os.ReadFile("../internal/benchfixtures/testdata/real/ron")
	if err != nil {
		t.Fatal(err)
	}
	const at = 237 // The first pinned R4 insertion, after tuple: (3, 7),.
	if source[at] != '\n' {
		t.Fatal("RON fixture's pinned insertion moved")
	}
	edited := append(append(append([]byte{}, source[:at]...), 'x'), source[at:]...)
	lang := grammars.RonLanguage()
	cLang, err := ParityCLanguage("ron")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(compact)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			old.Edit(gts.InputEdit{
				StartByte: at, OldEndByte: at, NewEndByte: at + 1,
				StartPoint: pointAtOffset(source, at), OldEndPoint: pointAtOffset(source, at), NewEndPoint: pointAtOffset(edited, at+1),
			})
			incremental, err := parser.ParseIncremental(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			assertErrorModeExternalRecoveryLockedC(t, incremental, lang, cLang, edited)
			fresh, err := parser.Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			assertErrorModeExternalRecoveryLockedC(t, fresh, lang, cLang, edited)
		})
	}
}

func TestErrorModeExternalRetryVHDLLockedC(t *testing.T) {
	source, err := os.ReadFile("../internal/benchfixtures/testdata/real/vhdl")
	if err != nil {
		t.Fatal(err)
	}
	seed := uint32(4242)
	seed = 1664525*seed + 1013904223
	at := int(seed % uint32(len(source)+1))
	edited := append(append(append([]byte{}, source[:at]...), 'x'), source[at:]...)
	lang := grammars.VhdlLanguage()
	cLang, err := ParityCLanguage("vhdl")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(compact)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			old.Edit(gts.InputEdit{
				StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
				StartPoint: pointAtOffset(source, at), OldEndPoint: pointAtOffset(source, at), NewEndPoint: pointAtOffset(edited, at+1),
			})
			incremental, err := parser.ParseIncremental(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer incremental.Release()
			assertErrorModeExternalRecoveryLockedC(t, incremental, lang, cLang, edited)
			fresh, err := parser.Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			assertErrorModeExternalRecoveryLockedC(t, fresh, lang, cLang, edited)
		})
	}
}

func TestTOMLFullBenchmarkFixturesLockedC(t *testing.T) {
	lang := grammars.TomlLanguage()
	cLang, err := ParityCLanguage("toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{32, 137, 1024} {
		t.Run(fmt.Sprintf("%dKiB", size), func(t *testing.T) {
			source, _, err := benchfixtures.GeneratedSource("toml", size*1024)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := gts.NewParser(lang).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			assertErrorModeExternalRecoveryLockedC(t, tree, lang, cLang, source)
		})
	}
}

func assertErrorModeExternalRecoveryLockedC(t *testing.T, tree *gts.Tree, lang *gts.Language, cLang *sitter.Language, source []byte) {
	t.Helper()
	cTree := compactT3ParseC(t, cLang, source)
	defer cTree.Close()
	if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
		t.Fatalf("Go/C shape mismatch: %+v", diff)
	}
	if err := firstLockedCTreeFlagDivergence(tree.RootNode(), lang, cTree.RootNode(), "/"); err != nil {
		t.Fatal(err)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != digest {
		t.Fatalf("Go/C digest: Go=%s C=%s", inspection.SHA256, digest)
	}
	if tree.ParseRuntime().StopReason != gts.ParseStopAccepted {
		t.Fatalf("parse did not accept: %s", tree.ParseRuntime().Summary())
	}
	if tree.RootNode().IsError() && !tree.RootNode().HasError() {
		t.Fatal("ERROR root reports no error")
	}
}

// Pair these Go timings with the locked static C perf_oracle on the same
// GeneratedSource fixtures. Keep the unfinished-pair witness as a recovery
// cost measurement; it intentionally has an error in both runtimes.
func BenchmarkTOMLUnfinishedPairFull(b *testing.B) {
	fixtures := []struct {
		name     string
		source   []byte
		hasError bool
	}{
		{"witness", []byte("[session]\nvalue = 0\nhalf = "), true},
	}
	for _, size := range []int{32, 137, 1024} {
		source, _, err := benchfixtures.GeneratedSource("toml", size*1024)
		if err != nil {
			b.Fatal(err)
		}
		fixtures = append(fixtures, struct {
			name     string
			source   []byte
			hasError bool
		}{fmt.Sprintf("%dKiB", size), source, false})
	}
	for _, fixture := range fixtures {
		b.Run(fixture.name, func(b *testing.B) {
			parser := gts.NewParser(grammars.TomlLanguage())
			b.SetBytes(int64(len(fixture.source)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree, err := parser.Parse(fixture.source)
				if err != nil {
					b.Fatal(err)
				}
				if tree.ParseRuntime().StopReason != gts.ParseStopAccepted || tree.RootNode().EndByte() != uint32(len(fixture.source)) || tree.RootNode().HasError() != fixture.hasError {
					b.Fatalf("incomplete or incorrect error flag: %s", tree.ParseRuntime().Summary())
				}
				tree.Release()
			}
		})
	}
}
