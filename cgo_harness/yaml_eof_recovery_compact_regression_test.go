//go:build cgo && treesitter_c_parity && !gts_no_parsercorephase0

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestYAMLEOFRecoverWrapCompactRoute keeps the compact route aligned with the
// locked C recover_eof root for this certified YAML boundary.
func TestYAMLEOFRecoverWrapCompactRoute(t *testing.T) {
	source := []byte("[\n")
	cLanguage, err := ParityCLanguage("yaml")
	if err != nil {
		t.Fatalf("load YAML C oracle: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("YAML C oracle returned no root")
	}
	if cRoot.Kind() != "ERROR" || cRoot.IsExtra() || !cRoot.IsError() || !cRoot.HasError() {
		t.Fatalf("C root kind=%q extra=%t is_error=%t has_error=%t, want non-extra ERROR with both error flags", cRoot.Kind(), cRoot.IsExtra(), cRoot.IsError(), cRoot.HasError())
	}
	if cRoot.StartByte() != 0 || cRoot.EndByte() != 1 {
		t.Fatalf("C root span=%d..%d, want 0..1", cRoot.StartByte(), cRoot.EndByte())
	}

	wantChildren := []struct {
		kind       string
		start, end uint64
	}{
		{kind: "[", start: 0, end: 1},
	}
	if got := int(cRoot.ChildCount()); got != len(wantChildren) {
		t.Fatalf("C root child count=%d, want %d", got, len(wantChildren))
	}
	for i, want := range wantChildren {
		child := cRoot.Child(uint(i))
		if child == nil {
			t.Fatalf("C root child[%d] is nil", i)
		}
		if child.Kind() != want.kind || uint64(child.StartByte()) != want.start || uint64(child.EndByte()) != want.end {
			t.Fatalf("C root child[%d]=%q[%d..%d], want %q[%d..%d]", i, child.Kind(), child.StartByte(), child.EndByte(), want.kind, want.start, want.end)
		}
	}

	goLanguage := grammars.YamlLanguage()
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	compactTree := compactT3ParseGo(t, goLanguage, source, true)
	defer compactTree.Release()
	goRoot := compactTree.RootNode()
	if goRoot == nil {
		t.Fatal("compact Go route returned no root")
	}
	if goRoot.Type(goLanguage) != "ERROR" || goRoot.IsExtra() || !goRoot.IsError() || !goRoot.HasError() ||
		goRoot.StartByte() != 0 || goRoot.EndByte() != 1 {
		t.Fatalf("compact Go root kind=%q span=%d..%d extra=%t is_error=%t has_error=%t, want non-extra ERROR 0..1 with both error flags", goRoot.Type(goLanguage), goRoot.StartByte(), goRoot.EndByte(), goRoot.IsExtra(), goRoot.IsError(), goRoot.HasError())
	}
	if divergences := compactT3StructuralDivergences(goRoot, goLanguage, cRoot); len(divergences) != 0 {
		formatted := make([]string, len(divergences))
		for index, divergence := range divergences {
			formatted[index] = compactT3FormatDivergence(divergence)
		}
		t.Fatalf("compact Go/C YAML recover_eof tree diverged:\n%s\nGo: %s\n%s\nC: %s\n%s", strings.Join(formatted, "\n"), goRoot.SExpr(goLanguage), dumpGoTree(goRoot, goLanguage, 0), cRoot.ToSexp(), dumpCTree(cRoot, 0))
	}
	if got := goRoot.ChildCount(); got != len(wantChildren) {
		t.Fatalf("compact Go root child count=%d, want %d", got, len(wantChildren))
	}
	for i := 0; i < goRoot.ChildCount(); i++ {
		child := goRoot.Child(i)
		want := wantChildren[i]
		if child == nil || uint64(child.StartByte()) != want.start || uint64(child.EndByte()) != want.end || child.IsExtra() || child.IsError() || child.Parent() != goRoot {
			t.Fatalf("compact Go root child[%d] invalid: child=%v, want span=%d..%d non-extra non-error", i, child, want.start, want.end)
		}
	}
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter-routedBefore != 1 || fallbackAfter-fallbackBefore != 0 {
		t.Fatalf("compact route counters=%d/%d, want publication 1/0; reason=%q", routedAfter-routedBefore, fallbackAfter-fallbackBefore, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
}

func TestYAMLEOFRecoverWrapUnfinishedMappingDeclinesCompact(t *testing.T) {
	source := []byte("a: [1\n")
	cLanguage, err := ParityCLanguage("yaml")
	if err != nil {
		t.Fatalf("load YAML C oracle: %v", err)
	}
	cTree := compactT3ParseC(t, cLanguage, source)
	defer cTree.Close()
	cRoot := cTree.RootNode()
	if cRoot == nil {
		t.Fatal("YAML C oracle returned no root")
	}
	if cRoot.Kind() != "ERROR" || cRoot.IsExtra() || !cRoot.IsError() || !cRoot.HasError() ||
		cRoot.StartByte() != 0 || cRoot.EndByte() != 5 || cRoot.ChildCount() != 4 {
		t.Fatalf("C root=%q[%d..%d] extra=%t is_error=%t has_error=%t children=%d, want non-extra ERROR 0..5 with four children", cRoot.Kind(), cRoot.StartByte(), cRoot.EndByte(), cRoot.IsExtra(), cRoot.IsError(), cRoot.HasError(), cRoot.ChildCount())
	}
	wantChildren := []struct {
		kind                  string
		start, end            uint
		named, missing, extra bool
	}{
		{kind: "flow_node", start: 0, end: 1, named: true},
		{kind: ":", start: 1, end: 2},
		{kind: "[", start: 3, end: 4},
		{kind: "flow_node", start: 4, end: 5, named: true},
	}
	for index, want := range wantChildren {
		child := cRoot.Child(uint(index))
		if child == nil || child.Kind() != want.kind || child.StartByte() != want.start || child.EndByte() != want.end ||
			child.IsNamed() != want.named || child.IsMissing() != want.missing || child.IsExtra() != want.extra || child.IsError() || child.HasError() {
			t.Fatalf("C root child[%d]=%v, want %q[%d..%d] named=%t ordinary", index, child, want.kind, want.start, want.end, want.named)
		}
	}
	goLanguage := grammars.YamlLanguage()
	goParser := gotreesitter.NewParser(goLanguage)
	goParser.SetAdmissionCandidateRoute(true)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	goTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("unfinished mapping admission parse: %v", err)
	}
	defer goTree.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter-routedBefore != 0 || fallbackAfter-fallbackBefore != 1 ||
		gotreesitter.AdmissionCandidateLastFallbackReason() == "" {
		t.Fatalf("unfinished mapping compact direct admission routed=%d fallback=%d reason=%q, want 0/1 decline", routedAfter-routedBefore, fallbackAfter-fallbackBefore, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
}

func TestYAMLOneBytePrintableEOFAdmissionMatchesLockedC(t *testing.T) {
	cLanguage, err := ParityCLanguage("yaml")
	if err != nil {
		t.Fatalf("load YAML C oracle: %v", err)
	}
	goLanguage := grammars.YamlLanguage()
	for value := byte(0x21); value <= 0x7e; value++ {
		value := value
		t.Run(fmt.Sprintf("byte_%02x", value), func(t *testing.T) {
			source := []byte{value, '\n'}
			cTree := compactT3ParseC(t, cLanguage, source)
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot == nil {
				t.Fatal("YAML C oracle returned no root")
			}

			goParser := gotreesitter.NewParser(goLanguage)
			goParser.SetAdmissionCandidateRoute(true)
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			goTree, err := goParser.Parse(source)
			if err != nil {
				t.Fatalf("one-byte YAML admission parse: %v", err)
			}
			defer goTree.Release()
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			routedDelta := routedAfter - routedBefore
			fallbackDelta := fallbackAfter - fallbackBefore
			if routedDelta == 0 {
				if fallbackDelta != 1 {
					t.Fatalf("declined one-byte YAML route counters=%d/%d, want 0/1", routedDelta, fallbackDelta)
				}
				return
			}
			if routedDelta != 1 || fallbackDelta != 0 {
				t.Fatalf("admitted one-byte YAML route counters=%d/%d, want 1/0", routedDelta, fallbackDelta)
			}
			goRoot := goTree.RootNode()
			if goRoot == nil {
				t.Fatal("admitted one-byte YAML route returned no root")
			}
			if divergences := compactT3StructuralDivergences(goRoot, goLanguage, cRoot); len(divergences) != 0 {
				formatted := make([]string, len(divergences))
				for index, divergence := range divergences {
					formatted[index] = compactT3FormatDivergence(divergence)
				}
				t.Fatalf("admitted one-byte YAML Go/C tree diverged:\n%s\nGo: %s\n%s\nC: %s\n%s", strings.Join(formatted, "\n"), goRoot.SExpr(goLanguage), dumpGoTree(goRoot, goLanguage, 0), cRoot.ToSexp(), dumpCTree(cRoot, 0))
			}
		})
	}
}
