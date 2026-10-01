package gotreesitter

import (
	"reflect"
	"testing"
)

// Compare the candidate index with an exhaustive root scan. Include duplicate
// alternation branches, anonymous aliases, escaped punctuation and wildcards;
// the index may remove attempts, but must preserve captures and their order.
func TestQueryRootLiteralCandidateIndex(t *testing.T) {
	lang := queryTestLanguage()
	lang.SymbolNames = append(lang.SymbolNames, "func", `\?`)
	lang.SymbolMetadata = append(lang.SymbolMetadata, SymbolMetadata{Name: "func", Visible: true}, SymbolMetadata{Name: `\?`, Visible: true})
	root := NewParentNode(7, true, []*Node{
		NewLeafNode(8, false, 0, 4, Point{}, Point{Column: 4}),
		NewLeafNode(16, false, 5, 9, Point{Column: 5}, Point{Column: 9}),
		NewLeafNode(1, true, 10, 11, Point{Column: 10}, Point{Column: 11}),
		NewLeafNode(9, false, 12, 18, Point{Column: 12}, Point{Column: 18}),
		NewLeafNode(17, false, 19, 20, Point{Column: 19}, Point{Column: 20}),
	}, nil, 0)
	for _, source := range []string{
		`"func" @keyword ["return" (identifier)] @value ["func" "func" (identifier)] @duplicate`,
		`"?" @punctuation ["?" (identifier)] @mixed _ @any`,
		`["func" _ "return"] @mixed (program (identifier) @child)`,
		`"absent" @absent (identifier) @id`,
	} {
		t.Run(source, func(t *testing.T) {
			q, err := NewQuery(source, lang)
			if err != nil {
				t.Fatal(err)
			}
			exhaustive := *q
			exhaustive.rootCandidatesDense, exhaustive.rootCandidatesBySymbol = nil, nil
			exhaustive.rootFallbackCandidates = make([]int, len(q.patterns))
			for i := range exhaustive.rootFallbackCandidates {
				exhaustive.rootFallbackCandidates[i] = i
			}
			exhaustive.canSkipExactRootLeaves = false
			for _, streaming := range []bool{false, true} {
				execute := func(query *Query) []QueryMatch {
					if !streaming {
						return query.ExecuteNode(root, lang, nil)
					}
					cursor := query.Exec(root, lang, nil)
					var matches []QueryMatch
					for {
						match, ok := cursor.NextMatch()
						if !ok {
							return matches
						}
						matches = append(matches, match)
					}
				}
				got, want := execute(q), execute(&exhaustive)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("streaming=%t: indexed=%+v exhaustive=%+v", streaming, got, want)
				}
			}
		})
	}
}
