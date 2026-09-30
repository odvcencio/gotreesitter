package gotreesitter

import (
	"fmt"
	"testing"
)

func TestLeafReuseRepetitionFoldRequiresRecordedFrontier(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, storedState := range []StateID{0, 4} {
			t.Run(fmt.Sprintf("reverse=%t/stored=%d", reverse, storedState), func(t *testing.T) {
				lang := buildArithmeticLanguage()
				lang.ExternalScanner = quiescenceStatelessScanner{}
				actions := []ParseAction{
					{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
					{Type: ParseActionShift, State: 4, Repetition: true},
				}
				if reverse {
					actions[0], actions[1] = actions[1], actions[0]
				}
				index := uint16(len(lang.ParseActions))
				lang.ParseActions = append(lang.ParseActions, ParseActionEntry{Actions: actions})
				lang.ParseTable[1][1] = index
				lang.ParseTable[2][1] = 6 // The same leaf shifts after the fold.
				parser := NewParser(lang)
				stack := glrStack{entries: []stackEntry{
					{state: 0},
					newStackEntryNode(1, &Node{symbol: 1, endByte: 1}),
				}}
				leaf := &Node{symbol: 1, parseState: storedState, preGotoState: 2, startByte: 1, endByte: 2}
				cursor := &reuseCursor{cachedStart: 1, cachedStartValid: true, cached: []*Node{leaf}}
				lookahead := Token{Symbol: 1, StartByte: 1, EndByte: 2}
				if next, ok := parser.reuseTargetState(1, leaf, lookahead); !ok || next != 4 {
					t.Fatalf("control matching shift: state=%d reusable=%t", next, ok)
				}
				if frontier, required := cursor.requiredReuseOwnershipFrontier(parser, &stack, lookahead); !required || frontier != 2 {
					t.Fatalf("leaf bypasses the repetition fold: frontier=%d required=%t", frontier, required)
				}
				if stack.top().state != 1 || stack.depth() != 2 {
					t.Fatal("checking the recorded frontier mutated the stack")
				}
				leaf.preGotoState = 3
				if _, required := cursor.requiredReuseOwnershipFrontier(parser, &stack, lookahead); required {
					t.Fatal("unreachable recorded frontier forced a reduction")
				}
				leaf.preGotoState = 2
				leaf.parseState = 3
				if _, required := cursor.requiredReuseOwnershipFrontier(parser, &stack, lookahead); required {
					t.Fatal("a leaf without a matching shift forced a reduction")
				}
			})
		}
	}
}
