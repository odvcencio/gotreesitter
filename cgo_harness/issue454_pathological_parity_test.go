//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func issue454PathologicalSource(name string, size int) []byte {
	var out bytes.Buffer
	switch name {
	case "make", "make-two-errors":
		for i := 0; out.Len() < size; i++ {
			fmt.Fprintf(&out, "VAR%d = value%d\ntarget%d: dep%d\n\t@echo target%d\n\n", i, i, i, i, i)
		}
		src := out.Bytes()
		at := bytes.Index(src, []byte("target0:")) + len("target")
		src = append(append(append([]byte(nil), src[:at]...), '('), src[at:]...)
		if name == "make-two-errors" {
			at = bytes.Index(src, []byte("target1:")) + len("target")
			src = append(append(append([]byte(nil), src[:at]...), '('), src[at:]...)
		}
		return src
	case "http":
		for i := 0; out.Len() < size; i++ {
			fmt.Fprintf(&out, "# note %d\n", i)
		}
	case "dart":
		for i := 0; out.Len() < size; i++ {
			fmt.Fprintf(&out, "class C%d {\n  int f%d(int a, int b) {\n    var x%d = a + b;\n    return x%d;\n  }\n}\n\n", i, i, i, i)
		}
	case "dart-single":
		out.WriteString("class C {\n")
		for i := 0; out.Len() < size; i++ {
			fmt.Fprintf(&out, "  int f%d(int a, int b) {\n    var x%d = a + b;\n    return x%d;\n  }\n", i, i, i)
		}
		out.WriteString("}\n")
	}
	return out.Bytes()
}

func TestIssue454PathologicalFreshCOracle(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{"make", 3584},
		{"make-two-errors", 3584},
		{"http", 2048},
		{"http", 4096},
		{"http", 8192},
		{"http", 16384},
		{"http", 32768},
		{"dart", 137 << 10},
		{"dart-single", 137 << 10},
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.name, tc.size), func(t *testing.T) {
			source := issue454PathologicalSource(tc.name, tc.size)
			grammarName := tc.name
			if grammarName == "dart-single" {
				grammarName = "dart"
			} else if grammarName == "make-two-errors" {
				grammarName = "make"
			}
			cLanguage, err := COracleLanguage(grammarName)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no root")
			}
			defer cTree.Close()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}
			goLanguage := grammars.DetectLanguageByName(grammarName).Language()
			for _, candidate := range []bool{false, true} {
				name := "production"
				if candidate {
					name = "compact"
				}
				t.Run(name, func(t *testing.T) {
					parser := gotreesitter.NewParser(goLanguage)
					parser.SetAdmissionCandidateRoute(candidate)
					tree, err := parser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					if tree.ParseStoppedEarly() {
						t.Fatalf("parse stopped: %s", tree.ParseRuntime().Summary())
					}
					if diff := FirstDivergenceDumpV1(tree.RootNode(), goLanguage, cTree.RootNode()); diff != nil {
						t.Fatalf("fresh tree differs from locked C: %+v", *diff)
					}
					inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), goLanguage)
					if err != nil {
						t.Fatal(err)
					}
					if inspection.SHA256 != cDigest {
						t.Fatalf("deep digest=%s, locked C=%s", inspection.SHA256, cDigest)
					}
				})
			}
		})
	}
}

func TestIssue454HTTPOracleSectionGroups(t *testing.T) {
	lang, err := COracleLanguage("http")
	if err != nil {
		t.Fatal(err)
	}
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(lang); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 16, 17, 20, 32, 197, 755, 1500} {
		var src bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&src, "# note %d\n", i)
		}
		tree := parser.Parse(src.Bytes(), nil)
		if tree == nil || tree.RootNode() == nil {
			t.Fatal("missing C tree")
		}
		root := tree.RootNode()
		wantSections := (n + 7) / 8
		if int(root.ChildCount()) != wantSections {
			t.Fatalf("%d lines: got %d C sections, want %d", n, root.ChildCount(), wantSections)
		}
		for i := 0; i < wantSections; i++ {
			wantComments := 8
			if i == 0 && n%8 != 0 {
				wantComments = n % 8
			}
			if got := int(root.Child(uint(i)).ChildCount()); got != wantComments {
				t.Fatalf("%d lines, section %d: got %d comments, want %d", n, i, got, wantComments)
			}
		}
		tree.Close()
	}
}

func TestIssue454HTTPCommentVariantsCOracle(t *testing.T) {
	lang, err := COracleLanguage("http")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		line func(int) string
	}{
		{"punctuation", func(i int) string { return fmt.Sprintf("# note %d !?:[]\n", i) }},
		{"crlf", func(i int) string { return fmt.Sprintf("# note %d\r\n", i) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			for i := 0; i < 30; i++ {
				b.WriteString(tc.line(i))
			}
			source := b.Bytes()
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(lang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no root")
			}
			defer cTree.Close()
			goLang := grammars.HttpLanguage()
			for _, candidate := range []bool{false, true} {
				parser := gotreesitter.NewParser(goLang)
				parser.SetAdmissionCandidateRoute(candidate)
				tree, err := parser.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				if diff := FirstDivergenceDumpV1(tree.RootNode(), goLang, cTree.RootNode()); diff != nil {
					t.Fatalf("compact=%v: %+v", candidate, *diff)
				}
				tree.Release()
			}
		})
	}
}
