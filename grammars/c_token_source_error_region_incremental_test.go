package grammars_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

func TestCTokenSourceErrorRegionIncrementalMatchesFresh(t *testing.T) {
	cases := []struct {
		name   string
		before string
		edits  []struct {
			start, oldEnd int
			replacement  string
		}
	}{
		{
			name:   "replace token before directive",
			before: "m#d",
			edits: []struct {
				start, oldEnd int
				replacement  string
			}{{start: 0, oldEnd: 1, replacement: "x"}},
		},
		{
			name:   "multiple edits across error region",
			before: "}#e",
			edits: []struct {
				start, oldEnd int
				replacement  string
			}{{start: 0, oldEnd: 0, replacement: "x"}, {start: 1, oldEnd: 2, replacement: "x"}, {start: 0, oldEnd: 1, replacement: ""}},
		},
	}

	lang := grammars.CLanguage()
	factory := grammarruntime.TokenSourceFactory("c")
	if factory == nil {
		t.Fatal("C TokenSource is not registered")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := []byte(tc.before)
			old, err := gotreesitter.NewParser(lang).ParseWithTokenSource(current, factory(current, lang))
			if err != nil {
				t.Fatalf("parse before: %v", err)
			}
			for _, change := range tc.edits {
				next := make([]byte, 0, len(current)-(change.oldEnd-change.start)+len(change.replacement))
				next = append(next, current[:change.start]...)
				next = append(next, change.replacement...)
				next = append(next, current[change.oldEnd:]...)
				edit := gotreesitter.InputEdit{
					StartByte: uint32(change.start), OldEndByte: uint32(change.oldEnd),
					NewEndByte: uint32(change.start + len(change.replacement)),
					StartPoint: directiveTestPoint(current, change.start),
					OldEndPoint: directiveTestPoint(current, change.oldEnd),
					NewEndPoint: directiveTestPoint(next, change.start+len(change.replacement)),
				}
				old.Edit(edit)
				current = next
			}

			incremental, err := gotreesitter.NewParser(lang).ParseIncrementalWithTokenSource(current, old, factory(current, lang))
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			fresh, err := gotreesitter.NewParser(lang).ParseWithTokenSource(current, factory(current, lang))
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			if diff := directiveTreeDiff(incremental.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
				t.Fatalf("incremental tree differs from fresh parse at %s\nincremental: %s\nfresh:       %s",
					diff, incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
			}
		})
	}
}
