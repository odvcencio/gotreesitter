//go:build !grammar_subset

package grammars

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// The fish blob that shipped until November 2026 had no ExternalLexStates
// table. Without it the C-recovery gate refused fish, and every fish input
// with an error recovered on the legacy path. The blob regenerated from the
// same pinned parser.c carries the table's 8 rows. The trees below are the
// ones the pinned C parser (tree-sitter-fish fa2143f5, runtime 0.27.1) gives;
// cgo_harness TestFishExternalLexStatesLockedCParity compares the same inputs
// with C directly. Issue #1529 reported the first one: valid input without a
// final newline.
var fishExternalLexStatesWitnesses = []struct {
	name, source, want string
}{
	{
		name:   "issue-1529-no-final-newline",
		source: "echo hi",
		want:   `(program [0,7] error (ERROR [0,7] extra error (command [0,7] name: (word [0,4]) argument: (word [5,7]))))`,
	},
	{
		name:   "missing-terminator",
		source: "complete -x",
		want:   `(program [0,11] error (command [0,11] name: (word [0,8]) argument: (word [9,11])) (";" [11,11] missing error))`,
	},
	{
		name:   "lone-redirect",
		source: "<",
		want:   `(program [0,1] error (ERROR [0,1] extra error (direction [0,1])))`,
	},
}

// fishTreeShape writes a node as (type [start,end] flags children...), with
// the field name before each child that has one. Anonymous types are quoted.
func fishTreeShape(b *strings.Builder, node *gotreesitter.Node, lang *gotreesitter.Language) {
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
		fishTreeShape(b, node.Child(i), lang)
	}
	b.WriteByte(')')
}

func fishShape(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language) string {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		t.Fatalf("no root, stop %s", tree.ParseStopReason())
	}
	var b strings.Builder
	fishTreeShape(&b, root, lang)
	return b.String()
}

func TestFishBlobCarriesExternalLexStates(t *testing.T) {
	lang := FishLanguage()
	if got := len(lang.ExternalLexStates); got != 8 {
		t.Fatalf("fish ExternalLexStates rows = %d, want the 8 rows of the pinned parser.c", got)
	}
	if !lang.CRecoveryCostCompetitionCapable || !lang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatalf("fish C recovery capable=%v default=%v, want both true; gate: %s",
			lang.CRecoveryCostCompetitionCapable, lang.CRecoveryCostCompetitionEnabledByDefault,
			gotreesitter.DiagnoseCRecoveryGate(lang).Reason)
	}
}

func TestFishExternalLexStatesTreesMatchC(t *testing.T) {
	lang := FishLanguage()
	for _, tt := range fishExternalLexStatesWitnesses {
		t.Run(tt.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(tt.source))
			if err != nil {
				t.Fatalf("parse %q: %v", tt.source, err)
			}
			defer tree.Release()
			if got := fishShape(t, tree, lang); got != tt.want {
				t.Fatalf("parse %q:\n got  %s\n want %s", tt.source, got, tt.want)
			}
			if stop := tree.ParseStopReason(); stop != gotreesitter.ParseStopAccepted {
				t.Fatalf("parse %q: stop reason %s, want %s", tt.source, stop, gotreesitter.ParseStopAccepted)
			}

			// Delete a final newline: the incremental parse must give the
			// same tree as the fresh parse.
			withNewline := []byte(tt.source + "\n")
			old, err := parser.Parse(withNewline)
			if err != nil {
				t.Fatalf("parse %q: %v", withNewline, err)
			}
			defer old.Release()
			n := uint32(len(tt.source))
			old.Edit(gotreesitter.InputEdit{
				StartByte: n, OldEndByte: n + 1, NewEndByte: n,
				StartPoint:  gotreesitter.Point{Row: 0, Column: n},
				OldEndPoint: gotreesitter.Point{Row: 1, Column: 0},
				NewEndPoint: gotreesitter.Point{Row: 0, Column: n},
			})
			incremental, err := parser.ParseIncremental([]byte(tt.source), old)
			if err != nil {
				t.Fatalf("incremental parse %q: %v", tt.source, err)
			}
			defer incremental.Release()
			if got := fishShape(t, incremental, lang); got != tt.want {
				t.Fatalf("incremental parse %q after deleting the final newline:\n got  %s\n want %s", tt.source, got, tt.want)
			}
		})
	}
}
