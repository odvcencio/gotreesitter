//go:build !grammar_subset || grammar_subset_mojo

package grammarruntime

import "testing"

// TestMojoNulIsNotTheEndLikeC: with indents [0,4], input
// "\x00x" and only DEDENT valid, C does not see the end of the input at the NUL
// byte and returns false (a NUL is not the end: lexer->eof is false); the
// legacy mode treats the NUL as the end and dedents.
func TestMojoNulIsNotTheEndLikeC(t *testing.T) {
	scanner := MojoExternalScanner{}
	valid := make([]bool, mojoTokenCount)
	valid[mojoTokDedent] = true
	for _, cfOn := range []bool{true, false} {
		st := scanner.Create().(*pythonScannerState)
		st.SetCFaithful(cfOn)
		st.Indents = []uint16{0, 4}
		got := portScan(scanner, st, "\x00x", valid)
		if cfOn && got.ok {
			t.Errorf("C-faithful: dedent at a NUL byte, got %+v", got)
		}
		if !cfOn && !got.ok {
			t.Errorf("legacy: expected the NUL read as the end (dedent), got %+v", got)
		}
	}
}
