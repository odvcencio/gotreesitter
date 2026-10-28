package grammars

import (
	"fmt"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func parseEOFNewlineSample(t *testing.T, name, source string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()
	for _, entry := range AllLanguages() {
		if entry.Name != name {
			continue
		}
		lang := entry.Language()
		parser := gotreesitter.NewParser(lang)
		var (
			tree *gotreesitter.Tree
			err  error
		)
		if entry.TokenSourceFactory != nil {
			tree, err = parser.ParseWithTokenSource([]byte(source), entry.TokenSourceFactory([]byte(source), lang))
		} else {
			tree, err = parser.Parse([]byte(source))
		}
		if err != nil {
			t.Fatalf("parse %s %q: %v", name, source, err)
		}
		t.Cleanup(tree.Release)
		return tree, lang
	}
	t.Fatalf("language %q is not registered", name)
	return nil, nil
}

// A valid sample without its final newline makes the locked C parser insert a
// hidden MISSING token. C counts that leaf in the error cost of every
// ancestor, so HasError must hold from the root down to the node that owns it.
func TestEOFNewlineHiddenMissingMarksAncestors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		// path lists the child indexes that lead from the root to the node
		// that owns the hidden MISSING leaf.
		path []int
	}{
		{"ini", "[section]\nkey = value", []int{0, 1}},
		{"cylc", "[scheduling]", []int{0}},
		{"chatito", "%[greet]\n    hi", []int{0, 3}},
		{"ssh_config", "Host example\n  User admin", []int{0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, _ := parseEOFNewlineSample(t, tc.name, tc.source)
			node := tree.RootNode()
			if !node.HasError() {
				t.Fatalf("root of %q reports no error", tc.source)
			}
			for depth, idx := range tc.path {
				if idx >= node.ChildCount() {
					t.Fatalf("depth %d: child %d out of range (%d children)", depth, idx, node.ChildCount())
				}
				node = node.Child(idx)
				if !node.HasError() {
					t.Fatalf("depth %d: node reports no error although it owns a hidden MISSING token", depth)
				}
			}
		})
	}
}

// These grammars accept the end of input where a newline would go, through a
// zero-width token or a NUL token, so the sample is clean in C.
func TestEOFNewlineAcceptedAtEndOfInput(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"tlaplus", "---- MODULE Test ----\n===="},
		{"bitbake", "SUMMARY = \"hello\""},
		{"earthfile", "FROM alpine"},
		{"gitattributes", "*.txt text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, _ := parseEOFNewlineSample(t, tc.name, tc.source)
			if tree.RootNode().HasError() {
				t.Fatalf("%q reports an error", tc.source)
			}
		})
	}
}

// The pug scanner emits a newline only for a real newline byte, so the sample
// without one reaches the parser as a missing newline, as in C.
func TestEOFNewlinePugMissingNewline(t *testing.T) {
	tree, _ := parseEOFNewlineSample(t, "pug", "p hello")
	if !tree.RootNode().HasError() {
		t.Fatal("pug sample without a final newline reports no error")
	}
	tree, _ = parseEOFNewlineSample(t, "pug", "p hello\n")
	if tree.RootNode().HasError() {
		t.Fatal("pug sample with a final newline reports an error")
	}
}

// An incomplete forest parse is not a clean root: the clean rescue must keep
// the recovered production tree.
func TestEOFNewlineIncompleteForestRootHasError(t *testing.T) {
	tree, _ := parseEOFNewlineSample(t, "ledger", "2024-01-01 Groceries\n  Expenses:Food  $50\n  Assets:Bank")
	if !tree.RootNode().HasError() {
		t.Fatal("ledger sample without a final newline reports no error")
	}
}

func eofNewlineDump(node *gotreesitter.Node, lang *gotreesitter.Language, depth int, b *strings.Builder) {
	fmt.Fprintf(b, "%s%s[%d-%d] missing=%t error=%t has_error=%t\n", strings.Repeat(" ", depth),
		node.Type(lang), node.StartByte(), node.EndByte(), node.IsMissing(), node.IsError(), node.HasError())
	for i := 0; i < node.ChildCount(); i++ {
		eofNewlineDump(node.Child(i), lang, depth+1, b)
	}
}

// Adding or removing the final newline must give the same tree as a fresh
// parse, including the zero-width tokens and hidden MISSING tokens that only
// exist while the newline is absent.
func TestEOFNewlineIncrementalEqualsFresh(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"ini", "[section]\nkey = value"},
		{"cylc", "[scheduling]"},
		{"chatito", "%[greet]\n    hi"},
		{"ssh_config", "Host example\n  User admin"},
		{"tlaplus", "---- MODULE Test ----\n===="},
		{"bitbake", "SUMMARY = \"hello\""},
		{"earthfile", "FROM alpine"},
		{"gitattributes", "*.txt text"},
		{"pug", "p hello"},
		{"ledger", "2024-01-01 Groceries\n  Expenses:Food  $50\n  Assets:Bank"},
	} {
		for _, appending := range []bool{true, false} {
			name := tc.name + "/remove_newline"
			oldSource, newSource := tc.source+"\n", tc.source
			if appending {
				name = tc.name + "/append_newline"
				oldSource, newSource = tc.source, tc.source+"\n"
			}
			t.Run(name, func(t *testing.T) {
				oldTree, lang := parseEOFNewlineSample(t, tc.name, oldSource)
				shorter, longer := newSource, oldSource
				if appending {
					shorter, longer = oldSource, newSource
				}
				edit := gotreesitter.InputEdit{
					StartByte:   uint32(len(shorter)),
					OldEndByte:  uint32(len(oldSource)),
					NewEndByte:  uint32(len(newSource)),
					StartPoint:  eofNewlinePoint(shorter),
					OldEndPoint: eofNewlinePoint(oldSource),
					NewEndPoint: eofNewlinePoint(newSource),
				}
				_ = longer
				oldTree.Edit(edit)
				var entry *LangEntry
				for _, e := range AllLanguages() {
					if e.Name == tc.name {
						e := e
						entry = &e
					}
				}
				parser := gotreesitter.NewParser(lang)
				var (
					incremental *gotreesitter.Tree
					err         error
				)
				if entry.TokenSourceFactory != nil {
					incremental, err = parser.ParseIncrementalWithTokenSource([]byte(newSource), oldTree, entry.TokenSourceFactory([]byte(newSource), lang))
				} else {
					incremental, err = parser.ParseIncremental([]byte(newSource), oldTree)
				}
				if err != nil {
					t.Fatalf("incremental parse: %v", err)
				}
				defer incremental.Release()
				fresh, _ := parseEOFNewlineSample(t, tc.name, newSource)
				var got, want strings.Builder
				eofNewlineDump(incremental.RootNode(), lang, 0, &got)
				eofNewlineDump(fresh.RootNode(), lang, 0, &want)
				if got.String() != want.String() {
					t.Fatalf("incremental tree differs from fresh parse\nincremental:\n%sfresh:\n%s", got.String(), want.String())
				}
			})
		}
	}
}

func eofNewlinePoint(source string) gotreesitter.Point {
	row := strings.Count(source, "\n")
	column := len(source) - (strings.LastIndex(source, "\n") + 1)
	return gotreesitter.Point{Row: uint32(row), Column: uint32(column)}
}
