package grammars_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

// TestIssue454RegisteredTokenSourcesIncrementalMatchesFresh is the issue #454
// follow-up report's §6 finding, generalized to every registered TokenSource
// (grammars/runtime/token_source_factory_builtin.go), not just C: a
// Highlighter built with WithTokenSourceFactory drives ParseIncremental
// through the hand-written grammar-runtime lexer, not the DFA lexer the rest
// of this package's differential tests exercise. That lexer can diverge from
// a fresh parse on an incremental edit even when the DFA-lexer path is
// correct for the same edit, because the two lexers do not share any
// incremental-reuse state.
//
// The specific C defect this caught: CTokenSource.stringToken /
// scanDelimitedBody lex a string or char literal's body eagerly -- one
// Next() call for the opening quote queues every remaining piece (content,
// escape sequences, the closing quote) in ts.pending and advances the raw
// cursor past all of them. When incremental reuse resumed lexing at a reused
// leaf's end byte that fell inside that already-queued span (for example an
// unedited string_content run), SkipToByte saw the target behind its cursor,
// took its "skip backward" branch, and unconditionally discarded
// ts.pending. Top-level dispatch then resumed mid-literal with no memory of
// being inside one: a lone '\' matches no top-level rule and is silently
// skipped (the "unknown byte" fallback), so an escape sequence's trailing
// letter (for example the 'n' of '\n') gets lexed as a bare identifier
// instead. That single wrong token cascades into a materially different
// tree on ordinary insert/replace edits (grammars/runtime/c_lexer.go's
// SkipToByte now resumes from the still-valid queued tokens instead of
// discarding them).
//
// This test is deliberately generic across every registered TokenSource so a
// similar defect in another hand-written lexer (or a regression here) fails
// the same way: incremental vs fresh, for insert, delete and replace, per
// language.
func TestIssue454RegisteredTokenSourcesIncrementalMatchesFresh(t *testing.T) {
	const fixtureBytes = 24 * 1024

	for _, tf := range registeredTokenSourceDiffFixtures {
		tf := tf
		t.Run(tf.lang, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(tf.lang)
			if entry == nil || entry.Language() == nil {
				t.Skipf("language %q unavailable in this build", tf.lang)
			}
			factory := grammarruntime.TokenSourceFactory(tf.lang)
			if factory == nil {
				t.Skipf("no registered TokenSource for %q", tf.lang)
			}
			lang := entry.Language()
			src := tf.gen(fixtureBytes)
			site := bytes.Index(src, []byte(tf.marker))
			if site < 0 {
				t.Fatalf("fixture has no %q marker", tf.marker)
			}
			point := issue454PointAt(src, site)
			newTS := func(s []byte) gotreesitter.TokenSource { return factory(s, lang) }

			edits := []struct {
				name   string
				edited []byte
				edit   gotreesitter.InputEdit
			}{
				{
					name: "replace",
					edited: func() []byte {
						e := append([]byte(nil), src...)
						e[site]++
						return e
					}(),
					edit: gotreesitter.InputEdit{
						StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site + 1),
						StartPoint: point, OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1}, NewEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1},
					},
				},
				{
					name:   "insert",
					edited: append(append(append([]byte(nil), src[:site]...), src[site]), src[site:]...),
					edit: gotreesitter.InputEdit{
						StartByte: uint32(site), OldEndByte: uint32(site), NewEndByte: uint32(site + 1),
						StartPoint: point, OldEndPoint: point, NewEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1},
					},
				},
				{
					name:   "delete",
					edited: append(append([]byte(nil), src[:site]...), src[site+1:]...),
					edit: gotreesitter.InputEdit{
						StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site),
						StartPoint: point, OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + 1}, NewEndPoint: point,
					},
				},
			}

			for _, edit := range edits {
				edit := edit
				t.Run(edit.name, func(t *testing.T) {
					old, err := gotreesitter.NewParser(lang).ParseWithTokenSource(src, newTS(src))
					if err != nil {
						t.Fatalf("old ParseWithTokenSource: %v", err)
					}
					old.Edit(edit.edit)

					incremental, err := gotreesitter.NewParser(lang).ParseIncrementalWithTokenSource(edit.edited, old, newTS(edit.edited))
					if err != nil {
						t.Fatalf("ParseIncrementalWithTokenSource: %v", err)
					}
					fresh, err := gotreesitter.NewParser(lang).ParseWithTokenSource(edit.edited, newTS(edit.edited))
					if err != nil {
						t.Fatalf("fresh ParseWithTokenSource: %v", err)
					}

					incRoot, freshRoot := incremental.RootNode(), fresh.RootNode()
					if incRoot == nil || freshRoot == nil {
						t.Fatal("parse returned no root")
					}
					incNodes, freshNodes := countRegisteredTSNodes(incRoot), countRegisteredTSNodes(freshRoot)
					if incNodes != freshNodes || incRoot.SExpr(lang) != freshRoot.SExpr(lang) || incRoot.HasError() != freshRoot.HasError() {
						t.Fatalf("%s/%s via its registered TokenSource: incremental tree diverges from a fresh parse: incNodes=%d freshNodes=%d incHasError=%v freshHasError=%v",
							tf.lang, edit.name, incNodes, freshNodes, incRoot.HasError(), freshRoot.HasError())
					}
				})
			}
		})
	}
}

func countRegisteredTSNodes(n *gotreesitter.Node) int {
	if n == nil {
		return 0
	}
	c := 1
	for i := 0; i < n.ChildCount(); i++ {
		c += countRegisteredTSNodes(n.Child(i))
	}
	return c
}

type registeredTokenSourceFixture struct {
	lang   string
	marker string
	gen    func(n int) []byte
}

// registeredTokenSourceDiffFixtures covers every language with a registered
// TokenSource (grammars/runtime/token_source_factory_builtin.go). Each
// fixture repeats a small unit containing the marker until it reaches the
// requested size, mirroring cmd/issue454bench's generator so the edit
// happens at a representative "near-top" site inside a real declaration.
// The C and C++ fixtures include a printf-style string literal with a '\n'
// escape sequence in every unit, matching the shape that exposed the
// CTokenSource defect above.
var registeredTokenSourceDiffFixtures = []registeredTokenSourceFixture{
	{lang: "c", marker: "x0", gen: genRegisteredTSFixtureC},
	{lang: "cpp", marker: "x0", gen: genRegisteredTSFixtureCpp},
	{lang: "java", marker: "x0", gen: genRegisteredTSFixtureJava},
	{lang: "json", marker: "x0", gen: genRegisteredTSFixtureJSON},
	{lang: "authzed", marker: "x0", gen: genRegisteredTSFixtureAuthzed},
}

func genRegisteredTSFixtureC(n int) []byte {
	var b bytes.Buffer
	b.WriteString("#include <stdio.h>\n\n")
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "int f%d(int a, int b) {\n    int x0 = a + b;\n    printf(\"f%d %%d\\n\", x0);\n    return x0;\n}\n\n", i, i)
	}
	return b.Bytes()
}

func genRegisteredTSFixtureCpp(n int) []byte {
	var b bytes.Buffer
	b.WriteString("#include <cstdio>\n\n")
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "int f%d(int a, int b) {\n    int x0 = a + b;\n    printf(\"f%d %%d\\n\", x0);\n    return x0;\n}\n\n", i, i)
	}
	return b.Bytes()
}

func genRegisteredTSFixtureJava(n int) []byte {
	var b bytes.Buffer
	b.WriteString("public class Sample {\n")
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "    static int f%d(int a, int b) {\n        int x0 = a + b;\n        System.out.printf(\"f%d %%d\\n\", x0);\n        return x0;\n    }\n\n", i, i)
	}
	b.WriteString("}\n")
	return b.Bytes()
}

func genRegisteredTSFixtureJSON(n int) []byte {
	var b bytes.Buffer
	b.WriteString("[\n")
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "  {\"x0\": %d, \"name\": \"f%d line\\n\"},\n", i, i)
	}
	s := strings.TrimSuffix(b.String(), ",\n") + "\n]\n"
	return []byte(s)
}

func genRegisteredTSFixtureAuthzed(n int) []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "definition f%d {\n\trelation x0: person\n\tpermission x0s = x0\n}\n\n", i)
	}
	return b.Bytes()
}
