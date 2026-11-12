//go:build cgo && (treesitter_c_parity || treesitter_c_bench)

package cgoharness

import (
	"strings"
	"testing"
)

// The recorded compile flags of a grammar oracle must be the flags it is compiled with,
// and they must turn strict aliasing off and make char signed.
func TestCOracleGrammarCompileFlagsMatchRecordedFlags(t *testing.T) {
	if !strings.Contains(COracleGrammarCFlags, "-fno-strict-aliasing") {
		t.Fatalf("recorded flags %q lack -fno-strict-aliasing", COracleGrammarCFlags)
	}
	if !strings.Contains(COracleGrammarCFlags, "-fsigned-char") {
		t.Fatalf("recorded flags %q lack -fsigned-char", COracleGrammarCFlags)
	}
	c := parityGrammarCCompileArgs("scanner.c", "scanner.o")
	if got := strings.Join(c[:7], " "); got != COracleGrammarCFlags {
		t.Fatalf("C compile flags %q differ from the recorded flags %q", got, COracleGrammarCFlags)
	}
	cxx := parityGrammarCXXCompileArgs("scanner.cc", "scanner.o")
	for _, flag := range []string{"-fno-strict-aliasing", "-fsigned-char"} {
		if !strings.Contains(strings.Join(cxx, " "), flag) {
			t.Fatalf("C++ compile flags %q lack %s", cxx, flag)
		}
	}
	if c[7] != "-c" || c[8] != "scanner.c" || c[9] != "-o" || c[10] != "scanner.o" {
		t.Fatalf("unexpected tail of the C compile arguments: %q", c)
	}
}
