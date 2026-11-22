package grammargen

import "testing"

func TestExternalOnlyNoLookahead(t *testing.T) {
	productions := []Production{{}, {IsExtra: true}}
	ordinary := lrAction{kind: lrReduce, prodIdx: 0}
	extra := lrAction{kind: lrReduce, prodIdx: 1}
	for _, tc := range []struct {
		name    string
		actions map[int][]lrAction
		want    bool
	}{
		{name: "ordinary end reduction", actions: map[int][]lrAction{0: {ordinary}}},
		{name: "extra completion", actions: map[int][]lrAction{0: {extra}}, want: true},
		{name: "mixed end reductions", actions: map[int][]lrAction{0: {ordinary, extra}}},
		{name: "external continuation", actions: map[int][]lrAction{0: {extra}, 1: {{kind: lrShift}}}},
		{name: "accept", actions: map[int][]lrAction{0: {{kind: lrAccept}}}},
		{name: "no end action", actions: map[int][]lrAction{1: {{kind: lrShift}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := externalOnlyNoLookahead(productions, tc.actions); got != tc.want {
				t.Fatalf("no-lookahead: got %t, want %t", got, tc.want)
			}
		})
	}
}
