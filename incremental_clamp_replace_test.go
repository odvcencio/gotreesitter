package gotreesitter_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// serializeNodeStructure renders a node as (type[start-end] child ...) so an
// incremental tree can be compared to a fresh tree for EXACT structural
// equality -- catching a mistyped leaf/value that an equal node COUNT would
// miss (the weakness the #382 review flagged in the original regression test).
func serializeNodeStructure(n *gts.Node, lang *gts.Language) string {
	if n == nil {
		return "<nil>"
	}
	var b strings.Builder
	var walk func(*gts.Node)
	walk = func(nd *gts.Node) {
		fmt.Fprintf(&b, "(%s[%d-%d]", nd.Type(lang), nd.StartByte(), nd.EndByte())
		for i := 0; i < int(nd.ChildCount()); i++ {
			b.WriteByte(' ')
			walk(nd.Child(i))
		}
		b.WriteByte(')')
	}
	walk(n)
	return b.String()
}

// clampReplaceEdit replaces the first occurrence of oldSub with newSub and
// returns the edited buffer plus the corresponding InputEdit. A same-position,
// length-changing replace whose old text straddles a token boundary clamps the
// straddled node's start -- the exact shape the #382 review exploited.
func clampReplaceEdit(src []byte, oldSub, newSub string) ([]byte, gts.InputEdit, bool) {
	i := bytes.Index(src, []byte(oldSub))
	if i < 0 {
		return nil, gts.InputEdit{}, false
	}
	edited := make([]byte, 0, len(src)-len(oldSub)+len(newSub))
	edited = append(edited, src[:i]...)
	edited = append(edited, newSub...)
	edited = append(edited, src[i+len(oldSub):]...)
	return edited, gts.InputEdit{
		StartByte:   uint32(i),
		OldEndByte:  uint32(i + len(oldSub)),
		NewEndByte:  uint32(i + len(newSub)),
		StartPoint:  suffixReusePointAt(src, i),
		OldEndPoint: suffixReusePointAt(src, i+len(oldSub)),
		NewEndPoint: suffixReusePointAt(edited, i+len(newSub)),
	}, true
}

// TestIncrementalLengthChangingReplaceMatchesFresh is the regression guard for
// the #382 review's BLOCKING finding: a length-changing replace whose edited
// range clamps a node's start must not let the reuse "un-dirty" optimization
// reverse-map the clamped coordinate and reuse a stale (genuinely-changed)
// subtree -- which produced a structurally-wrong incremental tree with
// HasError()==false (silent corruption). Asserts EXACT structural equality
// (full serialization) between incremental and fresh, not just node count.
func TestIncrementalLengthChangingReplaceMatchesFresh(t *testing.T) {
	cases := []struct {
		name, lang, base, oldSub, newSub string
	}{
		{
			// The review's exact repro: "margin: 14px;" -> "margin:px;". The
			// integer_value(unit) node for "14px" is clamped; a stale reuse
			// mistypes "px" as integer_value instead of plain_value.
			name: "css_integer_value_clamp", lang: "css",
			base:   "h1 {\n  color: red;\n  margin: 14px;\n  padding: 0;\n}\n",
			oldSub: ": 14", newSub: ":",
		},
		{
			// Delete-a-digit-inside-a-number variant, still a clamp-start replace.
			name: "css_shrink_number", lang: "css",
			base:   "a {\n  width: 1234px;\n  height: 5px;\n}\n",
			oldSub: "1234", newSub: "1",
		},
		{
			// Go: replace inside a composite-literal / call region near the top.
			name: "go_replace_ident", lang: "go",
			base:   "package main\n\nfunc main() {\n\tx := Thing{Val: 100}\n\t_ = x\n}\n",
			oldSub: "100", newSub: "7",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := grammars.DetectLanguageByName(tc.lang)
			if e == nil {
				t.Skipf("language %q unavailable", tc.lang)
			}
			lang := e.Language()
			src := []byte(tc.base)
			base, err := gts.NewParser(lang).Parse(src)
			if err != nil || base == nil || base.RootNode() == nil {
				t.Fatalf("baseline parse failed: %v", err)
			}
			edited, edit, ok := clampReplaceEdit(src, tc.oldSub, tc.newSub)
			if !ok {
				t.Fatalf("substring %q not found in base", tc.oldSub)
			}
			fresh, _ := gts.NewParser(lang).Parse(edited)
			if fresh == nil || fresh.RootNode() == nil {
				t.Fatal("fresh parse of edited text failed")
			}
			old := base.Copy()
			old.Edit(edit)
			incr, _, err := gts.NewParser(lang).ParseIncrementalProfiled(edited, old)
			if err != nil || incr == nil || incr.RootNode() == nil {
				t.Fatalf("incremental parse failed: %v", err)
			}
			freshS := serializeNodeStructure(fresh.RootNode(), lang)
			incrS := serializeNodeStructure(incr.RootNode(), lang)
			if freshS != incrS {
				t.Errorf("incremental tree diverges from fresh (silent corruption):\n  fresh: %s\n  incr:  %s", freshS, incrS)
			}
		})
	}
}

// TestIncrementalMultiEditSequenceMatchesFresh exercises the multi-edit
// reverse-map composition (oldByteForNew's loop) end-to-end -- applying several
// length-changing edits to the same tree in sequence and asserting the
// incremental result stays structurally identical to a fresh parse of the final
// text at every step. The #382 review noted the headline multi-edit capability
// shipped with no test.
func TestIncrementalMultiEditSequenceMatchesFresh(t *testing.T) {
	e := grammars.DetectLanguageByName("go")
	if e == nil {
		t.Skip("go unavailable")
	}
	lang := e.Language()
	src := []byte("package main\n\nfunc f(aa, bb int) int {\n\treturn aa + bb + 1234\n}\n\nfunc g() int { return f(10, 20) }\n")
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil || tree == nil {
		t.Fatalf("baseline parse failed: %v", err)
	}
	edits := []struct{ oldSub, newSub string }{
		{"1234", "5"},                          // shrink a literal (clamp)
		{"aa", "alpha"},                        // grow an identifier
		{"return f(10, 20)", "return f(1, 2)"}, // shrink args
	}
	cur := src
	for i, ed := range edits {
		edited, edit, ok := clampReplaceEdit(cur, ed.oldSub, ed.newSub)
		if !ok {
			t.Fatalf("edit %d: substring %q not found", i, ed.oldSub)
		}
		tree.Edit(edit)
		incr, _, err := gts.NewParser(lang).ParseIncrementalProfiled(edited, tree)
		if err != nil || incr == nil || incr.RootNode() == nil {
			t.Fatalf("edit %d: incremental parse failed: %v", i, err)
		}
		fresh, _ := gts.NewParser(lang).Parse(edited)
		if s1, s2 := serializeNodeStructure(fresh.RootNode(), lang), serializeNodeStructure(incr.RootNode(), lang); s1 != s2 {
			t.Fatalf("edit %d (%q->%q): incremental != fresh\n  fresh: %s\n  incr:  %s", i, ed.oldSub, ed.newSub, s1, s2)
		}
		tree, cur = incr, edited
	}
}
