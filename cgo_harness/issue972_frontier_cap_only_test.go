//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Conservative bounds for the 72-entry uniform initializer. The C oracle tree
// and the Go tree stay byte-identical at every entry count; these bounds only
// guard against runaway GLR growth.
//
// tree-sitter-c-sharp@9150f7d56bb4 adds five grammar conflicts, one of which
// ("lvalue_expression" against "non_lvalue_expression") forks on almost every
// expression. Measured on the 72-entry witness in Docker against the C oracle:
//
//	88366631d598: max_stacks=257 nodes=36194  arena=9.7 MiB   elapsed=0.74 s
//	9150f7d56bb4: max_stacks=524 nodes=139778 arena=102.6 MiB  elapsed=2.24 s
//
// The parse still finishes well inside the 300000-node and 512 MiB arena
// budgets and stays exact against C, so the bounds move with the measurement
// and keep about 20 percent headroom.
const (
	issue972Uniform72MaxStacks = 640
	issue972Uniform72MaxNodes  = 160000
)

func issue972UniformCSharpSource(entries int) []byte {
	var source strings.Builder
	source.WriteString("class C\n{\n    object x = new D()\n    {\n")
	for index := 0; index < entries; index++ {
		source.WriteString("        { typeof(int), Formatter.Instance },\n")
	}
	source.WriteString("    };\n}\n")
	return []byte(source.String())
}

func issue972ParseCSharp(t *testing.T, cLanguage *sitter.Language, source []byte) (*sitter.Tree, string) {
	t.Helper()
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		cParser.Close()
		t.Fatal(err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		cParser.Close()
		t.Fatal("locked C parser returned a nil tree")
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		cTree.Close()
		cParser.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cTree.Close()
		cParser.Close()
	})
	return cTree, cDigest
}

func issue972ParseGoCSharp(t *testing.T, source []byte) (*gotreesitter.Tree, *gotreesitter.Language, time.Duration) {
	t.Helper()
	language := grammars.CSharpLanguage()
	started := time.Now()
	parser := gotreesitter.NewParser(language)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("gotreesitter parse: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatal("gotreesitter returned a nil tree")
	}
	t.Cleanup(tree.Release)
	return tree, language, time.Since(started)
}

func issue972GoDigest(t *testing.T, tree *gotreesitter.Tree, language *gotreesitter.Language) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}

// TestIssue972FrontierCapOnlyUniformParity keeps the cap-only candidate exact
// on the uniform C# initializer family from four through 72 entries.
func TestIssue972FrontierCapOnlyUniformParity(t *testing.T) {
	cLanguage, err := COracleLanguage("c_sharp")
	if err != nil {
		t.Fatal(err)
	}
	for _, entries := range []int{4, 8, 16, 32, 40, 48, 64, 72} {
		entries := entries
		t.Run(fmt.Sprintf("entries-%d", entries), func(t *testing.T) {
			source := issue972UniformCSharpSource(entries)
			cTree, cDigest := issue972ParseCSharp(t, cLanguage, source)
			goTree, goLanguage, elapsed := issue972ParseGoCSharp(t, source)
			goDigest := issue972GoDigest(t, goTree, goLanguage)
			diff := FirstDivergenceDumpV1(goTree.RootNode(), goLanguage, cTree.RootNode())
			if diff != nil || goDigest != cDigest {
				t.Fatalf("cap-only uniform parity diverged: digest=%s C=%s first=%+v runtime=%s", goDigest, cDigest, diff, goTree.ParseRuntime().Summary())
			}
			if goTree.RootNode().HasError() || goTree.RootNode().EndByte() != uint32(len(source)) || goTree.ParseRuntime().Truncated {
				t.Fatalf("cap-only uniform parse is incomplete: root_error=%t root_end=%d source=%d runtime=%s", goTree.RootNode().HasError(), goTree.RootNode().EndByte(), len(source), goTree.ParseRuntime().Summary())
			}
			if entries == 72 {
				runtime := goTree.ParseRuntime()
				if runtime.MaxStacksSeen > issue972Uniform72MaxStacks || runtime.NodesAllocated > issue972Uniform72MaxNodes {
					t.Fatalf("cap-only 72-entry workload exceeded conservative bounds: max_stacks=%d/%d nodes=%d/%d runtime=%s", runtime.MaxStacksSeen, issue972Uniform72MaxStacks, runtime.NodesAllocated, issue972Uniform72MaxNodes, runtime.Summary())
				}
			}
			t.Logf("entries=%d bytes=%d elapsed=%s digest=%s runtime=%s", entries, len(source), elapsed, goDigest, goTree.ParseRuntime().Summary())
		})
	}
}
