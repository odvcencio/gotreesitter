package parsercorephase0

import (
	"slices"
	"testing"
)

func TestReductionIdentityMetadataMatchesRemapping(t *testing.T) {
	for _, extras := range []bool{false, true} {
		for _, width := range []int{0, 1, 3, 255} {
			for _, partial := range []bool{false, true} {
				c, err := New(&fakeTable{}, Limits{MaxNodes: 2048, MaxLinks: 2048, MaxSubtrees: 2048, MaxChildren: 2048})
				if err != nil {
					t.Fatal(err)
				}
				h, err := c.Seed(1, 0)
				if err != nil {
					t.Fatal(err)
				}
				cursor := uint32(0)
				for i := 0; i < width; i++ {
					if extras && i > 0 {
						h, err = c.appendDiagnosticPayload(h, 1, Token{Symbol: 11, StartByte: cursor, EndByte: cursor + 1, Extra: true}, pathMeta{})
						if err != nil {
							t.Fatal(err)
						}
						cursor++
					}
					h, err = c.appendDiagnosticPayload(h, 1, Token{Symbol: 10, StartByte: cursor, EndByte: cursor + 1}, pathMeta{})
					if err != nil {
						t.Fatal(err)
					}
					cursor++
				}
				paths, err := c.popPaths(h.Node, width)
				if err != nil || len(paths) != 1 {
					t.Fatalf("pop=%v/%v", paths, err)
				}
				var fields []FieldMapEntry
				var aliases []Symbol
				if width != 0 {
					fields = []FieldMapEntry{{FieldID: 1, ChildIndex: 0}, {FieldID: 2, ChildIndex: uint8(width - 1), Inherited: true}}
					aliases = make([]Symbol, width)
					aliases[0] = 31
					aliases[width-1] = 32
					if partial && width > 1 {
						aliases = aliases[:1]
					}
				}
				plan, err := NewReductionPlan(5, width, fields, aliases)
				if err != nil {
					t.Fatal(err)
				}
				var reference reductionOutputScratch
				wantFields, wantAliases, referenceErr := c.remapReductionPlan(paths[0].children, &plan, &reference)
				wantFields = append([]FieldMapEntry(nil), wantFields...)
				wantAliases = append([]Symbol(nil), wantAliases...)
				var scratch reductionOutputScratch
				id, _, _, err := c.reductionParentForPath(Action{Type: ActionReduce, Symbol: 20, ProductionID: 5, ChildCount: uint8(width)}, &plan, paths[0], c.boundaryKey(2, uint32(width)), ForkOrder{}, false, &scratch)
				if referenceErr != nil {
					if err == nil || err.Error() != referenceErr.Error() {
						t.Fatalf("remap errors differ: got %v want %v", err, referenceErr)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				view, err := c.Subtree(id)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(view.Fields, wantFields) || !slices.Equal(view.Aliases, wantAliases) || !slices.Equal(view.Children, paths[0].children) {
					t.Fatalf("width=%d extras=%t metadata changed: got fields=%v aliases=%v children=%v, want %v %v %v", width, extras, view.Fields, view.Aliases, view.Children, wantFields, wantAliases, paths[0].children)
				}
			}
		}
	}
}
