package grammars

import (
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

// Two no-action re-lex steps can undo each other on one stack (identifier ->
// keyword -> identifier at the same byte span) without consuming input; the
// plain DFA route must still terminate. See issue #1558.
func TestCppNoActionRelexCycleTerminates(t *testing.T) {
	inputs := []string{
		"",
		";e s d>f(T);template",
		"(T);n s e>f(T);template",
		"using l=[:s:]<([:v:])>;^^n#t o=[:^^x:];auto",
		"[:^^x:];o o^v:k=[:^^::m:];typename",
	}
	for _, kw := range []string{"typename", "auto", "class", "struct", "namespace", "using", "operator", "const", "static", "return", "if", "template<typename T>"} {
		inputs = append(inputs, ";e s d>f(T);"+kw)
	}
	for _, src := range inputs {
		parser := ts.NewParser(CppLanguage())
		parser.SetTimeoutMicros(2_000_000)
		tree, err := parser.Parse([]byte(src))
		if err != nil {
			t.Fatalf("%q: parse failed: %v", src, err)
		}
		if got := tree.ParseStopReason(); got == ts.ParseStopTimeout {
			t.Errorf("%q: parse hit the timeout; runtime=%s", src, tree.ParseRuntime().Summary())
		}
		if end := tree.RootNode().EndByte(); end != uint32(len(src)) {
			t.Errorf("%q: root ends at %d, want %d", src, end, len(src))
		}
	}
}
