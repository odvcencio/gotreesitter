//go:build !grammar_subset

package grammars

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// Before the converter read end-of-input conditions the way C evaluates them,
// it gave 31 fish lexer states an end-of-input edge to the word state, where
// C's ts_lex takes no step: `!sym_word_character_set_3(lookahead)` is false at
// the end of input because the set contains 0. So `set x 1` without a final
// newline lexed 1 as word, where C lexes integer.

// TestFishLexerEndOfInputEdgesMatchC compares every main lexer state's
// end-of-input edge in the shipped fish blob with the step that the exact-lexer
// record (built from the pinned parser.c) gives C at the end of input.
func TestFishLexerEndOfInputEdgesMatchC(t *testing.T) {
	lang := FishLanguage()
	sha, ok := lang.GrammarBlobSHA256()
	if !ok {
		t.Fatal("fish reports no blob hash")
	}
	encoded, ok := lexexact.Lookup(sha)
	if !ok {
		t.Fatalf("no exact-lexer record for fish blob %x", sha)
	}
	rec, err := lexexact.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Main.OnEOF) != len(lang.LexStates) {
		t.Fatalf("record has %d lexer states, blob %d", len(rec.Main.OnEOF), len(lang.LexStates))
	}
	differ := 0
	for i, code := range rec.Main.OnEOF {
		want := -1
		if step, ok := lexexact.UnpackStep(code); ok {
			want = step.Target
		}
		if got := lang.LexStates[i].EOF; got != want {
			if differ < 5 {
				t.Errorf("lexer state %d: end-of-input edge %d, C steps to %d", i, got, want)
			}
			differ++
		}
	}
	if differ > 0 {
		t.Fatalf("%d of %d fish lexer states have an end-of-input edge that differs from C", differ, len(lang.LexStates))
	}
}

// fishEOFEdgeShape writes a node as (type [start,end] flags children...), with
// the field name before each child that has one.
func fishEOFEdgeShape(b *strings.Builder, node *gotreesitter.Node, lang *gotreesitter.Language) {
	typ := node.Type(lang)
	if !node.IsNamed() {
		typ = fmt.Sprintf("%q", typ)
	}
	fmt.Fprintf(b, "(%s [%d,%d]", typ, node.StartByte(), node.EndByte())
	if node.IsExtra() {
		b.WriteString(" extra")
	}
	if node.IsMissing() {
		b.WriteString(" missing")
	}
	if node.HasError() {
		b.WriteString(" error")
	}
	for i := 0; i < node.ChildCount(); i++ {
		b.WriteByte(' ')
		if field := node.FieldNameForChild(i, lang); field != "" {
			b.WriteString(field + ": ")
		}
		fishEOFEdgeShape(b, node.Child(i), lang)
	}
	b.WriteByte(')')
}

// TestFishIntegerAtEndOfInputMatchesC checks the tree that the pinned C parser
// (tree-sitter-fish fa2143f5, runtime 0.27.1) gives for `set x 1` without a
// final newline.
func TestFishIntegerAtEndOfInputMatchesC(t *testing.T) {
	const source = "set x 1"
	const want = `(program [0,7] error (ERROR [0,7] extra error (command [0,7] name: (word [0,3]) argument: (word [4,5]) argument: (integer [6,7]))))`
	lang := FishLanguage()
	tree, err := gotreesitter.NewParser(lang).Parse([]byte(source))
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	defer tree.Release()
	var b strings.Builder
	fishEOFEdgeShape(&b, tree.RootNode(), lang)
	if got := b.String(); got != want {
		t.Fatalf("parse %q:\n got  %s\n want %s", source, got, want)
	}
}
