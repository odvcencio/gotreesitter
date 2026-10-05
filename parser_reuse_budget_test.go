package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestIncrementalReuseBudgetDeclinesReuseHostileEdit is the issue #454 C
// single-byte delete: the edit turns `x0` into `0` and old-tree reuse
// resynchronizes nowhere, so the incremental attempt used to build 3.2
// million nodes before the memory budget stopped it. The reuse budget stops
// the attempt after a bounded number of nodes, and the parser runs one plain
// full parse, which the returned tree must match exactly. Certified physical
// recovery ordering now avoids that runaway path and keeps useful suffix reuse;
// both paths retain the original memory bound and match the full fresh digest.
func TestIncrementalReuseBudgetDeclinesReuseHostileEdit(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("#include <stdio.h>\n\n")
	for i := 0; b.Len() < 137<<10; i++ {
		fmt.Fprintf(&b, "int f%d(int a, int b) {\n    int x0 = a + b;\n    printf(\"f%d %%d\\n\", x0);\n    return x0;\n}\n\n", i, i)
	}
	source := b.Bytes()
	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("fixture has no edit site")
	}
	edited := append(append([]byte{}, source[:site]...), source[site+1:]...)
	row, col := uint32(0), uint32(0)
	for _, c := range source[:site] {
		if c == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	edit := gts.InputEdit{
		StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site),
		StartPoint: gts.Point{Row: row, Column: col}, OldEndPoint: gts.Point{Row: row, Column: col + 1}, NewEndPoint: gts.Point{Row: row, Column: col},
	}
	for _, route := range []struct {
		name                  string
		physicalRecoveryOrder bool
		wantRetry             string
	}{
		{name: "legacy_budget", wantRetry: "incremental_parse_reuse_budget_full_retry"},
		{name: "certified_recovery", physicalRecoveryOrder: true},
	} {
		t.Run(route.name, func(t *testing.T) {
			language := *grammars.CLanguage()
			language.RecoveryStackVersionOrderEnabled = route.physicalRecoveryOrder
			lang := &language
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !route.physicalRecoveryOrder {
				old = withoutIncrementalRuntimeCertificate(t, old, lang)
			}
			defer old.Release()
			old.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, old)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer incremental.Release()
			fresh, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			incDigest, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			if incDigest.SHA256 != freshDigest.SHA256 {
				t.Fatal("incremental tree does not match the fresh parse")
			}
			if profile.ReuseUnsupportedReason != route.wantRetry {
				t.Fatalf("reuse unsupported reason = %q, want %q; profile=%+v", profile.ReuseUnsupportedReason, route.wantRetry, profile)
			}
			// Preserve the original memory bound for both recovery paths.
			if profile.NewNodesAllocated > 800_000 {
				t.Fatalf("reuse-hostile edit built %d nodes", profile.NewNodesAllocated)
			}
			if route.physicalRecoveryOrder && profile.ReusedBytes*8 < uint64(len(edited)) {
				t.Fatalf("certified recovery reused only %d of %d bytes", profile.ReusedBytes, len(edited))
			}
		})
	}
}

func TestDartTopLevelSiblingsPoorYieldRetriesFresh(t *testing.T) {
	for _, tc := range []struct {
		name      string
		largeSize int
		wantRetry bool
	}{
		{name: "poor_yield", largeSize: 135 << 10, wantRetry: true},
		{name: "profitable_suffix", largeSize: 80 << 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			b.WriteString("class Big {\n")
			for i := 0; b.Len() < tc.largeSize; i++ {
				fmt.Fprintf(&b, "  int f%d(int a, int b) {\n    var x%d = a + b;\n    return x%d;\n  }\n", i, i, i)
			}
			b.WriteString("}\n\n")
			for i := 0; b.Len() < 137<<10; i++ {
				fmt.Fprintf(&b, "class C%d { int f() { return %d; } }\n", i, i)
			}
			source := b.Bytes()
			site := bytes.Index(source, []byte("x0"))
			if site < 0 {
				t.Fatal("fixture has no edit site")
			}
			row := uint32(bytes.Count(source[:site], []byte{'\n'}))
			col := uint32(site - bytes.LastIndexByte(source[:site], '\n') - 1)
			sp := gts.Point{Row: row, Column: col}
			lang := grammars.DartLanguage()
			for _, route := range []struct {
				name      string
				candidate bool
			}{{name: "production"}, {name: "compact", candidate: true}} {
				t.Run(route.name, func(t *testing.T) {
					for _, kind := range []string{"insert", "delete"} {
						t.Run(kind, func(t *testing.T) {
							end := gts.Point{Row: row, Column: col + 1}
							edit := gts.InputEdit{StartByte: uint32(site), StartPoint: sp}
							var edited []byte
							if kind == "insert" {
								edited = append(append(append([]byte{}, source[:site]...), source[site]), source[site:]...)
								edit.OldEndByte, edit.NewEndByte = uint32(site), uint32(site+1)
								edit.OldEndPoint, edit.NewEndPoint = sp, end
							} else {
								edited = append(append([]byte{}, source[:site]...), source[site+1:]...)
								edit.OldEndByte, edit.NewEndByte = uint32(site+1), uint32(site)
								edit.OldEndPoint, edit.NewEndPoint = end, sp
							}
							parser := gts.NewParser(lang)
							parser.SetAdmissionCandidateRoute(route.candidate)
							old, err := parser.Parse(source)
							if err != nil {
								t.Fatal(err)
							}
							defer old.Release()
							if old.RootNode().ChildCount() <= 4 {
								t.Fatal("fixture lacks top-level siblings")
							}
							old.Edit(edit)
							inc, profile, err := parser.ParseIncrementalProfiled(edited, old)
							if err != nil {
								t.Fatal(err)
							}
							defer inc.Release()
							fresh, err := parser.Parse(edited)
							if err != nil {
								t.Fatal(err)
							}
							defer fresh.Release()
							incDigest, err := benchfixtures.InspectGoTree(inc.RootNode(), lang)
							if err != nil {
								t.Fatal(err)
							}
							freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
							if err != nil {
								t.Fatal(err)
							}
							if incDigest.SHA256 != freshDigest.SHA256 {
								t.Fatal("incremental tree differs from fresh")
							}
							if tc.wantRetry {
								if profile.ReuseUnsupportedReason != "incremental_parse_reuse_budget_full_retry" {
									t.Fatalf("reuse reason = %q, want full retry", profile.ReuseUnsupportedReason)
								}
								if profile.NewNodesAllocated > 210_000 {
									t.Fatalf("bounded parse built %d nodes", profile.NewNodesAllocated)
								}
							} else if profile.ReuseUnsupportedReason != "" || profile.ReusedBytes*5 < uint64(len(edited))*3 {
								t.Fatalf("profitable reuse was lost: reason=%q bytes=%d", profile.ReuseUnsupportedReason, profile.ReusedBytes)
							}
						})
					}
				})
			}
		})
	}
}
