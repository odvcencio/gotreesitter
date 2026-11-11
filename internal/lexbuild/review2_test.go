package lexbuild

import (
	"strings"
	"testing"
)

// Tests for the findings of the second review. Each expected result is derived from
// the reference source named in its comment (RX is regex-syntax-0.8.11/src/, G is the
// generator's crates/generate/src/ of release v0.27.1).

func mustHir(t *testing.T, pat string) string {
	t.Helper()
	h, err := parsePattern(pat, false)
	if err != nil {
		t.Fatalf("%s: %v", pat, err)
	}
	return hirString(h)
}

func mustFail(t *testing.T, pats ...string) {
	t.Helper()
	for _, pat := range pats {
		if _, err := parsePattern(pat, false); err == nil {
			t.Errorf("%s: want an error", pat)
		}
	}
}

// Finding 1. RX hir/translate.rs:409 (unicode_fold_and_negate negates the class),
// hir/mod.rs:359 (Hir::class: an empty class is Hir::fail, a class, not a literal).
// \P{Any} matches nothing; it must not become the literal U+0000.
func TestEmptyPropertyClassIsAClass(t *testing.T) {
	if got := mustHir(t, `\P{Any}a`); got != `Cat([] L"a")` {
		t.Errorf("got %s", got)
	}
	if got := mustHir(t, `[\P{Any}]`); got != `[]` {
		t.Errorf("got %s", got)
	}
}

// Finding 2. RX hir/interval.rs:480 (Interval::difference) and :565 (char increment and
// decrement skip the surrogate gap). The difference of [U+D7FF-U+E000] and [U+E000] is
// U+D7FF alone; negating {U+D7FF, U+E000} keeps both ends in the middle range.
func TestDifferenceAtSurrogateBoundary(t *testing.T) {
	if got := mustHir(t, `[\u{D7FF}-\u{E000}--\u{E000}]`); got != `L"\ud7ff"` {
		t.Errorf("got %s", got)
	}
	if got := mustHir(t, `[^[\u{D7FF}-\u{E000}--\u{E000}]\u{E000}]`); got != `[0-D7FE,D7FF-E000,E001-10FFFF,]` {
		t.Errorf("got %s", got)
	}
}

// Finding 3. RX ast/parse.rs:1944 (parse_set_class_range): a range endpoint must be a
// literal; a class primitive there is an error (ClassRangeLiteral).
func TestPropertyClassIsNotARangeEndpoint(t *testing.T) {
	mustFail(t, `[\p{L}-a]`, `[a-\p{L}]`, `[\P{L}-z]`)
	if _, err := parsePattern(`[\p{L}-]`, false); err != nil { // - before ] is a literal
		t.Error(err)
	}
}

// Finding 4. RX ast/parse.rs:2167 (parse_unicode_class: a ^ is part of the name, there is no
// negation syntax; ':' is looked for before '='), unicode.rs:902 (symbolic_name_normalize:
// "isc" is kept, it is not "c").
func TestUnicodePropertySyntax(t *testing.T) {
	mustFail(t, `\p{^L}`, `\P{^L}`, `\p{isc}`, `\p{IsC}`, `\p{gc=isc}`, `\p{Is_Foo}`)
	for _, ok := range []string{`\p{IsLu}`, `\p{gc=Lu}`, `\p{General_Category:Lu}`, `\p{gc!=Lu}`, `\pL`, `\p{Is Letter}`} {
		if _, err := parsePattern(ok, false); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	a, _ := parsePattern(`\p{gc!=Lu}`, false)
	b, _ := parsePattern(`\P{Lu}`, false)
	if hirString(a) != hirString(b) {
		t.Error("!= must negate")
	}
}

// Finding 5. RX ast/parse.rs:111 (is_capture_char), :452 (add_capture_name: duplicate names),
// :1298 (parse_capture_name: empty name), :1359 (parse_flags: a flag once, a negation once,
// FlagDanglingNegation; Flags::add_item compares the flag, not its sign).
func TestCaptureNamesAndFlags(t *testing.T) {
	mustFail(t, `(?<1>a)`, `(?<n>a)(?<n>b)`, `(?<>a)`, `(?<a-b>c)`, `(?ii)a`, `(?i-i)a`, `(?i-)a`, `(?i--s)a`, `(?q)a`, `(?)a`, `(?i`)
	for _, ok := range []string{`(?<_n.[]1>a)`, "(?<é>a)", `(?P<n>a)(?P<m>b)`, `(?i-s:a)`, `(?-i)a`} {
		if _, err := parsePattern(ok, false); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
}

// Finding 6. RX ast/parse.rs:1786 (parse_hex_brace: any number of digits, the value must be a
// scalar value) and :1843 (parse_decimal skips white space before and after the digits).
func TestBracedHexAndDecimalWhitespace(t *testing.T) {
	for _, pat := range []string{`\x{000000041}`, `\u{000000041}`, `\U{000000041}`, `\x41`, `A`, `\U00000041`} {
		if got := mustHir(t, pat); got != `L"A"` {
			t.Errorf("%s: got %s", pat, got)
		}
	}
	mustFail(t, `\x{110000}`, `\x{D800}`, `\x{}`, `\x{000000000041G}`)
	if got := mustHir(t, `a{ 2 }`); got != `R{2,2,true}(L"a")` {
		t.Errorf("a{ 2 }: %s", got)
	}
	if got := mustHir(t, `a{1 , 2 }`); got != `R{1,2,true}(L"a")` {
		t.Errorf("a{1 , 2 }: %s", got)
	}
	mustFail(t, `a{,2}`, `a{2,1}`, `a{99999999999}`)
}

// Finding 7 (the cases are also in review_test.go). RX ast/parse.rs:868 and :934, hir/translate.rs:616.
func TestEmptySetOperands(t *testing.T) {
	for pat, want := range map[string]string{`[a--]`: `L"a"`, `[a&&]`: `[]`, `[&&a]`: `[]`, `[a&&[b--b]]`: `[]`, `[a~~]`: `L"a"`} {
		if got := mustHir(t, pat); got != want {
			t.Errorf("%s: got %s, want %s", pat, got, want)
		}
	}
}

// Finding 8. RX ast/parse.rs:2279 (NestLimiter): groups, repetitions, bracketed classes,
// class unions of two or more items, set operations, alternations and concatenations of two or
// more items each count one level; the limit is 250.
func TestNestLimitCountsTheSyntaxTree(t *testing.T) {
	grp := func(n int, inner string) string { return strings.Repeat("(?:", n) + inner + strings.Repeat(")", n) }
	// An alternation of two branches is one level: 249 groups around it are 250 levels.
	mustHir(t, grp(249, "a|b"))
	mustFail(t, grp(250, "a|b"))
	// 249 chained && operators plus the bracket are 250 levels; 250 operators are 251.
	chain := func(n int) string { return "[a" + strings.Repeat("&&a", n) + "]" }
	mustHir(t, chain(248))
	mustFail(t, chain(250), chain(251))
	// Repetition operators count after their group closed.
	mustHir(t, grp(100, "a")+strings.Repeat("*", 150))
	mustFail(t, grp(100, "a")+strings.Repeat("*", 200))
	// A class union of two items is a level.
	nest := func(n int) string { return strings.Repeat("[ab", n) + strings.Repeat("]", n) }
	mustHir(t, nest(100))
	mustFail(t, nest(130))
}

// Finding 9. RX hir/mod.rs:378 (Hir::repetition: a sub-expression of maximum length 0 has its
// bounds clamped to 1; x{0} is empty; x{1} is x), ast/parse.rs:1017 and :1103 (a repetition needs a
// previous item that is not a flag directive; a group with an empty body is one).
func TestZeroLengthRepetition(t *testing.T) {
	if got := mustHir(t, `(){4294967295}a`); got != `Cat(C(E) L"a")` {
		t.Errorf("got %s", got)
	}
	if got := mustHir(t, `(?:)*a`); got != `Cat(R{0,1,true}(E) L"a")` {
		t.Errorf("got %s", got)
	}
	if got := mustHir(t, `a{0}*b`); got != `Cat(R{0,1,true}(E) L"b")` {
		t.Errorf("got %s", got)
	}
	mustFail(t, `(?i)*a`, `(?i){2}`, `a|*`)
	// The automaton for a huge count over an empty expression is built at once.
	p, err := Prepare(gj(`"source":{"type":"SEQ","members":[{"type":"PATTERN","value":"(){4294967295}a"},{"type":"STRING","value":"b"}]}`, ""))
	if err != nil || len(p.Lex.Variables) != 2 {
		t.Errorf("Prepare: %v", err)
	}
}

// Finding 10. G parse_grammar.rs:58 (RuleJSON), :95 (GrammarJSON), :249 and :290: rules and reserved
// sets are read as serde_json::Value first, which keeps the last of a repeated key; the other
// lists are deserialized directly, where a repeated field is an error. i32 fields do not take null.
func TestJSONValueBackedRules(t *testing.T) {
	p, err := Prepare(gj(`"source":{"type":"STRING","value":"x","value":"y"}`, ""))
	if err != nil || p.Lex.Variables[0].Ident != "y" {
		t.Errorf("a rule under rules keeps the last value: %v", err)
	}
	if _, err := ParseGrammar(gj(`"source":{"type":"STRING","value":"x"}`, `,"extras":[{"type":"STRING","value":"x","value":"y"}]`)); err == nil {
		t.Error("a repeated field in extras must be an error")
	}
	if _, err := ParseGrammar(gj(`"source":{"type":"PREC_DYNAMIC","value":null,"content":{"type":"STRING","value":"x"}}`, "")); err == nil {
		t.Error("PREC_DYNAMIC null must be an error")
	}
	bad := append([]byte(`{"name":"t","rules":{"source":{"type":"STRING","value":"`), 0xff, 0xfe)
	bad = append(bad, []byte(`"}}}`)...)
	if _, err := ParseGrammar(bad); err == nil {
		t.Error("invalid UTF-8 must be an error")
	}
}

// Finding 13. G extract_tokens.rs:400 and flatten_grammar.rs:365: a reserved word set holds tokens.
func TestReservedSetWithNonTerminalIsAnError(t *testing.T) {
	doc := gj(`"source":{"type":"SEQ","members":[{"type":"SYMBOL","name":"pair"},{"type":"STRING","value":"z"}]},"pair":{"type":"SEQ","members":[{"type":"STRING","value":"a"},{"type":"STRING","value":"b"}]}`,
		`,"reserved":{"global":[{"type":"SYMBOL","name":"pair"}]}`)
	if _, err := Prepare(doc); err == nil {
		t.Error("want an error")
	}
}

// Finding 14. RX hir/mod.rs:1799 (Capture has an index and a name, part of equality) and :3027
// (lift_common_prefix compares with Hir equality): two captures differ, so no prefix is lifted.
func TestCaptureIdentityInPrefixLifting(t *testing.T) {
	if got := mustHir(t, `(a)b|(a)c`); got != `Alt(Cat(C(L"a") L"b") Cat(C(L"a") L"c"))` {
		t.Errorf("got %s", got)
	}
	if got := mustHir(t, `a+b|a+c`); got != `Cat(R{1,0,false}(L"a") [62-63,])` {
		t.Errorf("got %s", got)
	}
}
