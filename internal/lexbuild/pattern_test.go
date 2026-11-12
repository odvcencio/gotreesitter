package lexbuild

import (
	"fmt"
	"strings"
	"testing"
)

func hirString(h *hir) string {
	switch h.kind {
	case hirEmpty:
		return "E"
	case hirLook:
		return "Look"
	case hirLiteral:
		return fmt.Sprintf("L%q", string(h.lit))
	case hirClass:
		var b strings.Builder
		b.WriteString("[")
		for _, r := range h.class {
			if r.lo == r.hi {
				fmt.Fprintf(&b, "%X,", r.lo)
			} else {
				fmt.Fprintf(&b, "%X-%X,", r.lo, r.hi)
			}
		}
		b.WriteString("]")
		return b.String()
	case hirRepetition:
		return fmt.Sprintf("R{%d,%d,%v}(%s)", h.min, h.max, h.hasMax, hirString(h.sub))
	case hirCapture:
		return "C(" + hirString(h.sub) + ")"
	case hirConcat, hirAlternation:
		var parts []string
		for _, s := range h.subs {
			parts = append(parts, hirString(s))
		}
		name := "Cat"
		if h.kind == hirAlternation {
			name = "Alt"
		}
		return name + "(" + strings.Join(parts, " ") + ")"
	}
	return "?"
}

// TestParsePatternShapes checks the Hir that parsePattern builds against the
// shapes that regex_syntax 0.8.11 gives for the same patterns (read from its
// Hir::concat, Hir::alternation, Hir::class and Hir::repetition, and from the
// Expander of pattern.rs).
func TestParsePatternShapes(t *testing.T) {
	cases := []struct{ pat, want string }{
		{`abc`, `L"abc"`},
		{`a|b|c`, `[61-63,]`},
		{`a|bc`, `Alt(L"a" L"bc")`},
		{`ab|ac`, `Alt(L"ab" L"ac")`},
		{`a+b|a+c`, `Cat(R{1,0,false}(L"a") [62-63,])`},
		{`ab|cd`, `Alt(L"ab" L"cd")`},
		{`[a]`, `L"a"`},
		{`[a-c]+`, `R{1,0,false}([61-63,])`},
		{`a{2}`, `R{2,2,true}(L"a")`},
		{`a{1}`, `L"a"`},
		{`a{0}`, `E`},
		{`(?:ab)*`, `R{0,0,false}(L"ab")`},
		{`(ab)`, `C(L"ab")`},
		{`(?<n>a)`, `C(L"a")`},
		{`(?P<n>a)`, `C(L"a")`},
		{`\x41B\U00000043\x{44}`, `L"ABCD"`},
		{`\u{1F600}`, "L\"\U0001F600\""},
		{`(?i)a`, `[41,61,]`},
		{`(?i)k`, `[4B,6B,]`},
		{`(?i:a)b`, `Cat([41,61,] L"b")`},
		{`((?i)a)b`, `Cat(C([41,61,]) L"b")`},
		{`(?i)a(?-i)b`, `Cat([41,61,] L"b")`},
		{`(?s).`, `[0-10FFFF,]`},
		{`.`, `[0-9,B-10FFFF,]`},
		{`(?R).`, `[0-9,B-C,E-10FFFF,]`},
		{`\p{Zs}`, `[20,A0,1680,2000-200A,202F,205F,3000,]`},
		{`\pN`, ``},
		{`[^a]`, `[0-60,62-10FFFF,]`},
		{`(?i)[^k]`, `[0-4A,4C-6A,6C-10FFFF,]`},
	}
	for _, c := range cases {
		h, err := parsePattern(c.pat, false)
		if err != nil {
			t.Errorf("%s: %v", c.pat, err)
			continue
		}
		if c.want == "" {
			continue
		}
		if got := hirString(h); got != c.want {
			t.Errorf("%s: got %s, want %s", c.pat, got, c.want)
		}
	}
	for _, bad := range []string{`(?=a)`, `\x4`, `\u{110000}`, `(?x)a`, `(?-u)a`, `\p{Greek}`} {
		if _, err := parsePattern(bad, false); err == nil {
			t.Errorf("%s: expected an error", bad)
		}
	}
	h, err := parsePattern("k", true)
	if err != nil || hirString(h) != `[4B,6B,]` {
		t.Errorf("flag i: %v %v", h, err)
	}
}
