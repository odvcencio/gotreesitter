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

func issue1335Source(functions int) []byte {
	var source strings.Builder
	for i := 0; i < functions; i++ {
		fmt.Fprintf(&source, "function fn%d(a, b) {\n\tvar x%d = a + b;\n\treturn x%d;\n}\n\n", i, i, i)
	}
	clean := source.String()
	at := strings.Index(clean, "x0")
	return []byte(clean[:at] + "\"" + clean[at:])
}

// The complete tree must equal a fresh locked-C parse, including fields,
// anonymous leaves, errors, points, and the root span. A program root alone
// would miss the lost variable declaration in the original recovery.
func TestJavaScriptMalformedQuoteLockedC(t *testing.T) {
	language := grammars.JavascriptLanguage()
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		source []byte
	}{
		{"minimal", []byte("var \"x = a;\n")},
		{"single_quote", []byte("var 'x = a;\n")},
		{"let", []byte("let \"x = a;\n")},
		{"const", []byte("const \"x = a;\n")},
		{"crlf", []byte("var \"x = a;\r\n")},
		{"closed_quote", []byte("var \"x\" = a;\n")},
		{"one_function", issue1335Source(1)},
		{"issue_1335", issue1335Source(40)},
		{"large", issue1335Source(16000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oracle := compactT3ParseC(t, cLanguage, tc.source)
			defer oracle.Close()
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(compact)
					tree, err := parser.Parse(tc.source)
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					requireIssue1335Tree(t, tree, language, oracle)
					t.Logf("bytes=%d tokens=%d nodes=%d stacks=%d", len(tc.source), tree.ParseRuntime().TokensConsumed, tree.ParseRuntime().NodesAllocated, tree.ParseRuntime().MaxStacksSeen)
				})
			}
		})
	}
}

func requireIssue1335Tree(t *testing.T, tree *gts.Tree, language *gts.Language, oracle *sitter.Tree) {
	t.Helper()
	if diff := FirstDivergenceDumpV1(tree.RootNode(), language, oracle.RootNode()); diff != nil {
		t.Fatalf("Go/C mismatch: %+v\nGo: %s\nC: %s", diff, tree.RootNode().SExpr(language), oracle.RootNode().ToSexp())
	}
	if diff := firstLockedCTreeFlagDivergence(tree.RootNode(), language, oracle.RootNode(), "/"); diff != nil {
		t.Fatal(diff)
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := COracleDeepDigest(oracle)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SHA256 != digest {
		t.Fatalf("Go/C digest: %s != %s", inspection.SHA256, digest)
	}
	t.Logf("exact fresh-C digest: %s", digest)
}

// BenchmarkJavaScriptMalformedQuoteLockedC pairs the same malformed input
// in Go-C-C-Go order. Both parsers are warm and each iteration owns a fresh
// tree. Run before/after samples with scripts/run_randomized_benchmarks.sh.
func BenchmarkJavaScriptMalformedQuoteLockedC(b *testing.B) {
	source := issue1335Source(40)
	language := grammars.JavascriptLanguage()
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"go_first", "c_first", "c_second", "go_second"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			if strings.HasPrefix(mode, "go_") {
				parser := gts.NewParser(language)
				parser.SetAdmissionCandidateRoute(false)
				warm, err := parser.Parse(source)
				if err != nil {
					b.Fatal(err)
				}
				warm.Release()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					tree, err := parser.Parse(source)
					if err != nil {
						b.Fatal(err)
					}
					tree.Release()
				}
			} else {
				parser := sitter.NewParser()
				defer parser.Close()
				if err := parser.SetLanguage(cLanguage); err != nil {
					b.Fatal(err)
				}
				warm := parser.Parse(source, nil)
				warm.Close()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					tree := parser.Parse(source, nil)
					if tree == nil {
						b.Fatal("C returned no tree")
					}
					tree.Close()
				}
			}
		})
	}
}

func TestJavaScriptMalformedQuoteIncrementalLockedC(t *testing.T) {
	language := grammars.JavascriptLanguage()
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}
	for _, broken := range [][]byte{[]byte("var \"x = a;\n"), issue1335Source(40)} {
		t.Run(fmt.Sprintf("bytes=%d", len(broken)), func(t *testing.T) {
			at := strings.IndexByte(string(broken), '"')
			clean := append(append([]byte{}, broken[:at]...), broken[at+1:]...)
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(compact)
					old, err := parser.Parse(clean)
					if err != nil {
						t.Fatal(err)
					}
					source := clean
					for _, after := range [][]byte{broken, clean} {
						oldEnd, newEnd := at, at+1
						if len(after) < len(source) {
							oldEnd, newEnd = at+1, at
						}
						old.Edit(gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd), StartPoint: pointAtOffset(source, at), OldEndPoint: pointAtOffset(source, oldEnd), NewEndPoint: pointAtOffset(after, newEnd)})
						next, err := parser.ParseIncremental(after, old)
						old.Release()
						if err != nil {
							t.Fatal(err)
						}
						oracle := compactT3ParseC(t, cLanguage, after)
						requireIssue1335Tree(t, next, language, oracle)
						oracle.Close()
						old, source = next, after
					}
					old.Release()
				})
			}
		})
	}
}
