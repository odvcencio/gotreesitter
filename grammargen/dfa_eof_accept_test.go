package grammargen

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestLexMinimizePreservesEOFAccept(t *testing.T) {
	states := []gts.LexState{
		{Default: -1, EOF: -1},
		{AcceptEOF: true, Default: -1, EOF: -1},
		{AcceptEOF: true, Default: -1, EOF: -1},
	}
	compacted, offsets := minimizeLexStates(states, []int{0, 1, 2})
	if len(compacted) != 2 || offsets[0] == offsets[1] || offsets[1] != offsets[2] {
		t.Fatalf("end acceptance partition: states=%+v offsets=%v", compacted, offsets)
	}
	if compacted[offsets[0]].AcceptEOF || !compacted[offsets[1]].AcceptEOF {
		t.Fatal("compaction changed end acceptance")
	}
}

func TestCCodegenPreservesEOFAccept(t *testing.T) {
	states := []gts.LexState{
		{Default: -1, EOF: 1},
		{AcceptEOF: true, Default: -1, EOF: -1},
	}
	var output strings.Builder
	emitLexFunction(&output, "ts_lex", states, &gts.Language{}, []string{"ts_builtin_sym_end"}, map[int]bool{0: true})
	if !strings.Contains(output.String(), "case 1:\n      ACCEPT_TOKEN(ts_builtin_sym_end);") {
		t.Fatalf("C lexer lost end acceptance:\n%s", output.String())
	}
}
