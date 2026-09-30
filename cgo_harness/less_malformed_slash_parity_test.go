//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func issue1336LessSource(rules int, slash bool) []byte {
	var source strings.Builder
	for i := 0; i < rules; i++ {
		fmt.Fprintf(&source, ".rule%d {\n  padding: 10px;\n  color: red;\n}\n", i)
	}
	text := source.String()
	if slash {
		at := strings.Index(text, "padding:") + len("padding:")
		text = text[:at] + "/" + text[at:]
	}
	return []byte(text)
}

func assertIssue1336FreshC(t *testing.T, tree *gts.Tree, lang *gts.Language, cLang *sitter.Language, source []byte) {
	t.Helper()
	cTree := compactT3ParseC(t, cLang, source)
	defer cTree.Close()
	if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cTree.RootNode()); diff != nil {
		t.Fatalf("fresh Go/C divergence: %+v", diff)
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
		t.Fatalf("deep digest Go=%s C=%s", inspection.SHA256, digest)
	}
}

func TestIssue1336LessMalformedSlashFreshC(t *testing.T) {
	lang := grammars.LessLanguage()
	cLang, err := ParityCLanguage("less")
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name   string
		source []byte
	}{
		{"shrunk-nesting", []byte("0{a:/a:} {}")},
		{"hidden-missing-integer", []byte(".a{a:/0}")},
		{"one-rule", issue1336LessSource(1, true)},
		{"two-rules", issue1336LessSource(2, true)},
		{"reported-80-rules", issue1336LessSource(80, true)},
		{"tab-padding", []byte(".a{a:/\t10px;} .b{}")},
		{"newline-padding", []byte(".a{a:/\n10px;} .b{}")},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gts.NewParser(lang)
					parser.SetAdmissionCandidateRoute(compact)
					tree, err := parser.Parse(fixture.source)
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					assertIssue1336FreshC(t, tree, lang, cLang, fixture.source)
				})
			}
		})
	}
}

func TestIssue1336LessSlashEditSessionFreshC(t *testing.T) {
	lang := grammars.LessLanguage()
	cLang, err := ParityCLanguage("less")
	if err != nil {
		t.Fatal(err)
	}
	clean, malformed := issue1336LessSource(80, false), issue1336LessSource(80, true)
	at := strings.Index(string(clean), "padding:") + len("padding:")
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(compact)
			tree, err := parser.Parse(clean)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { tree.Release() }()
			before := clean
			for step, source := range [][]byte{malformed, clean, malformed} {
				oldEnd, newEnd := at, at+1
				if len(source) < len(before) {
					oldEnd, newEnd = at+1, at
				}
				tree.Edit(gts.InputEdit{
					StartByte: uint32(at), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
					StartPoint: pointAtOffset(before, at), OldEndPoint: pointAtOffset(before, oldEnd), NewEndPoint: pointAtOffset(source, newEnd),
				})
				next, err := parser.ParseIncremental(source, tree)
				if err != nil {
					t.Fatal(err)
				}
				tree.Release()
				tree = next
				assertIssue1336FreshC(t, tree, lang, cLang, source)
				fresh, err := parser.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				incDigest, _ := benchfixtures.InspectGoTree(tree.RootNode(), lang)
				freshDigest, _ := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				fresh.Release()
				if incDigest.SHA256 != freshDigest.SHA256 {
					t.Fatalf("D8 mismatch at step %d", step)
				}
				allocations := testing.AllocsPerRun(10, func() {
					unchanged, err := parser.ParseIncremental(source, tree)
					if err != nil {
						panic(err)
					}
					unchanged.Release()
				})
				if allocations != 0 {
					t.Fatalf("no-edit allocations = %g", allocations)
				}
				before = source
			}
		})
	}
}
