package lexbuild

import (
	"fmt"
	"strings"
	"testing"
)

// Tests for the findings of the third review. RX is regex-syntax-0.8.11/src/, G is the
// generator's crates/generate/src/ of release v0.27.1.

// Finding 1. RX ast/parse.rs:1437 (parse_primitive: ^ and $ are assertions), :1479 (parse_escape:
// \A \z \b \B \< \> are assertions), hir/mod.rs:390 (Hir::repetition removes x{0}, even when x is
// an assertion); G prepare_grammar/expand_tokens.rs:363 (only a Look that survives is an error).
func TestAssertionsSurviveUntilExpansion(t *testing.T) {
	for pat, want := range map[string]string{
		`^{0}a`:       `L"a"`,
		`\b{0}a`:      `L"a"`,
		`(?:^a){0}b`:  `L"b"`,
		`\A{0}\z{0}c`: `L"c"`,
		`^a`:          `Cat(Look L"a")`,
		`a$`:          `Cat(L"a" Look)`,
		`\b{start}a`:  `Cat(Look L"a")`,
		`\B\<\>`:      `Cat(Look Look Look)`,
	} {
		if got := mustHir(t, pat); got != want {
			t.Errorf("%s: got %s, want %s", pat, got, want)
		}
	}
	mustFail(t, `[\b]`, `[^]\A]`, `\b{foo}`, `\b{start`)
	build := func(pat string) error {
		_, err := Prepare(gj(`"source":{"type":"PATTERN","value":`+jsonString(pat)+`}`, ""))
		return err
	}
	for _, ok := range []string{`^{0}a`, `\b{0}a`, `(?:^a){0}b`} {
		if err := build(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{`^a`, `a$`, `\ba`, `(?:^)*a`} {
		if err := build(bad); err == nil || !strings.Contains(err.Error(), "Assertions are not supported") {
			t.Errorf("%s: want the assertion error, got %v", bad, err)
		}
	}
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Finding 2. RX ast/parse.rs:1888 (parse_set_class: [ inside a class tries an ASCII class)
// and :2096 (maybe_parse_ascii_class: a failed attempt restores the position and the text is
// an ordinary nested class).
func TestAsciiClassFallback(t *testing.T) {
	for pat, want := range map[string]string{
		`[[:]]`:                `L":"`,
		`[[:loower:]]`:         `[3A,65,6C,6F,72,77,]`,
		`[[:lower]]`:           `[3A,65,6C,6F,72,77,]`,
		`[[:alpha:]]`:          `[41-5A,61-7A,]`,
		`[[:^alpha:]a]`:        `[0-40,5B-61,7B-10FFFF,]`,
		`[[:digit:][:space:]]`: `[9-D,20,30-39,]`,
		`(?i)[[:upper:]]`:      `[41-5A,61-7A,]`,
	} {
		if got := mustHir(t, pat); got != want {
			t.Errorf("%s: got %s, want %s", pat, got, want)
		}
	}
}

// Finding 6. RX ast/parse.rs:1024 and hir/mod.rs:378: the limit is checked on the syntax tree
// before Hir work, and Properties caches the length. A rejected input must be rejected after
// about 250 operators, not after reading all of them.
func TestNestLimitRejectsEarly(t *testing.T) {
	pat := "a" + strings.Repeat("*", 2000000)
	_, p, err := parsePatternWith(pat, false)
	if err == nil || !strings.Contains(err.Error(), "nest limit") {
		t.Errorf("want the nest limit error, got %v", err)
	}
	// The parser must stop at the operator that passes the limit (the 250th), not read all 2,000,000.
	if p.pos > 300 {
		t.Errorf("the parser read %d characters before it rejected the pattern; want it to stop near %d", p.pos, nestLimit+1)
	}
	if got := mustHir(t, "a"+strings.Repeat("*", 200)); !strings.HasPrefix(got, "R{0,0,false}(R{0,0,false}") {
		t.Errorf("200 operators: %s", got[:40])
	}
}

// Findings 3 to 5. G parse_grammar.rs:95 (GrammarJSON), :58 (RuleJSON), :249 and :290 (rules and
// reserved are read as serde_json::Value); serde's derive (de/struct_.rs:276-289: only recognised
// fields are checked for repetition); serde_json read.rs:1025 (an ignored string may hold a lone
// surrogate escape), de.rs:63 and :632 (the recursion limit of 128), de.rs:509-523 (-0 is a float),
// de.rs:1372 (a number out of range is an error when a value is read, not when it is skipped).
func TestJSONSerdeReading(t *testing.T) {
	const rule = `"source":{"type":"STRING","value":"x"}`
	ok := map[string]string{
		"unknown top-level field twice":           gjs(rule, `,"ignored":1,"ignored":2`),
		"unknown top-level lone surrogate":        gjs(rule, `,"ignored":"\ud800"`),
		"unknown top-level number too large":      gjs(rule, `,"ignored":1e400`),
		"unknown repeated field in a direct rule": gjs(rule, `,"extras":[{"type":"STRING","value":"y","z":1,"z":2}]`),
		"depth 124 in an ignored rule field":      gjs(`"source":{"type":"STRING","value":"x","z":`+strings.Repeat("[", 124)+"0"+strings.Repeat("]", 124)+`}`, ""),
		"unknown top-level field nested 130 deep": gjs(rule, `,"ignored":`+strings.Repeat("[", 130)+strings.Repeat("]", 130)),
		"numbers in range":                        gjs(`"source":{"type":"STRING","value":"x","z":1e300}`, ""),
		"valid key escapes":                       gjs(rule, `,"\ud83d\ude00":1,"\u0041":2`),
	}
	for name, doc := range ok {
		if _, err := ParseGrammar([]byte(doc)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := map[string]string{
		"repeated known top-level field":          gjs(rule, `,"word":"x","word":"x"`),
		"unknown field value in rules, 1e400":     gjs(`"source":{"type":"STRING","value":"x","z":1e400}`, ""),
		"replaced duplicate with 1e400":           gjs(`"source":{"type":"STRING","value":"x","z":1e400,"z":1}`, ""),
		"lone surrogate in an ignored rule field": gjs(`"source":{"type":"STRING","value":"x","z":"\ud800"}`, ""),
		"depth 125 in an ignored rule field":      gjs(`"source":{"type":"STRING","value":"x","z":`+strings.Repeat("[", 125)+"0"+strings.Repeat("]", 125)+`}`, ""),
		"lone surrogate in a top-level key":       gjs(rule, `,"\ud800":1`),
		"lone trailing surrogate in a key":        gjs(rule, `,"a\udc00":1`),
		"-0 in PREC_DYNAMIC":                      gjs(`"source":{"type":"PREC_DYNAMIC","value":-0,"content":{"type":"STRING","value":"x"}}`, ""),
		"-0 in PREC":                              gjs(`"source":{"type":"PREC","value":-0,"content":{"type":"STRING","value":"x"}}`, ""),
		"-0 in PREC_LEFT":                         gjs(`"source":{"type":"PREC_LEFT","value":-0,"content":{"type":"STRING","value":"x"}}`, ""),
		"repeated known field in a direct rule":   gjs(rule, `,"extras":[{"type":"STRING","value":"y","value":"z"}]`),
		"lone surrogate in a direct rule":         gjs(rule, `,"extras":[{"type":"STRING","value":"y","z":"\ud800"}]`),
	}
	for name, doc := range bad {
		if _, err := ParseGrammar([]byte(doc)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := ParseGrammar([]byte(gjs(`"source":{"type":"PREC_DYNAMIC","value":0,"content":{"type":"STRING","value":"x"}}`, ""))); err != nil {
		t.Errorf("0 must be accepted: %v", err)
	}
}

// Round 4, finding 2. serde_json de.rs: f64_from_parts (no float_roundtrip): the u64
// significand becomes an f64 and is multiplied by POW10[exponent]. Each row was run
// through the tree-sitter 0.27.1 binary ("rejected" = number out of range).
func TestJSONNumberRangeLikeSerde(t *testing.T) {
	rows := []struct {
		n      string
		reject bool
	}{
		{"1.7976931348623158e308", true},
		{"1.79769313486231581e308", false},
		{"1.7976931348623157e308", false},
		{"17976931348623158e292", true},
		{"1.8e308", true},
		{"1e309", true},
		{"1e308", false},
		{"0.00000e999", false},
		{"1e-400", false},
		{"1e99999999999", true},
		{"0e99999999999", false},
		{"1e-99999999999", false},
		{"123456789012345678901234567890e280", true},
		{"123456789012345678901234567890e289", true},
		{"18446744073709551616e289", true},
		{"1.8446744073709551616e300", false},
		{"1.8446744073709551616e290", false},
		{"18446744073709551615", false},
		{"-1e309", true},
	}
	for _, r := range rows {
		if got := numberOutOfRange(r.n); got != r.reject {
			t.Errorf("%s: out of range = %v, the generator says %v", r.n, got, r.reject)
		}
		doc := gjs(`"source":{"type":"STRING","value":"x","z":`+r.n+`}`, "")
		if _, err := ParseGrammar([]byte(doc)); (err != nil) != r.reject {
			t.Errorf("%s in a rule: error %v, want rejection %v", r.n, err, r.reject)
		}
	}
}

func gjs(rules, extra string) string { return string(gj(rules, extra)) }

// Finding 7. G rules.rs:546 and bitvec.rs:316: the key of a token set tells unequal sets apart.
func TestTokenSetKeyCollisions(t *testing.T) {
	var a, b TokenSet
	for _, i := range []int{0, 64, 65, 66, 67, 68, 69, 70, 71, 72, 16384} {
		a.Insert(terminalSym(i))
	}
	a.Insert(externalSym(1))
	b.Insert(terminalSym(0))
	for _, i := range []int{16304, 16368, 16369, 16370, 16371, 16372, 16373, 16374, 16375, 16376, 16385} {
		b.Insert(externalSym(i))
	}
	if a.Equal(&b) || a.Key() == b.Key() {
		t.Error("the two sets of the review must have different keys")
	}
	var c, d TokenSet
	c.Insert(terminalSym(3))
	d.Insert(terminalSym(3))
	if c.Key() != d.Key() || !c.Equal(&d) {
		t.Error("equal sets must have equal keys")
	}
	d.Insert(endSym())
	if c.Key() == d.Key() {
		t.Error("the end token must be part of the key")
	}
}

// The port bounds the automaton: the generator has no bound and runs on until it is killed for
// b{0,4294967295}; the port returns an error at three million states.
func TestAutomatonSizeBound(t *testing.T) {
	_, err := Prepare(gj(`"source":{"type":"PATTERN","value":"b{0,4294967295}"}`, ""))
	if err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("want the size error, got %v", err)
	}
}

// Round 4, finding 3. RX ast/parse.rs: NestLimiter runs before hir/translate.rs lowers a class:
// a set operation chain taller than the limit must not be evaluated. Distinct, separated
// characters make every symmetric difference grow its operand.
func TestClassOpChainBoundedWork(t *testing.T) {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < 10000; i++ {
		if i > 0 {
			b.WriteString("~~")
		}
		fmt.Fprintf(&b, `\x{%X}`, 0x10000+2*i)
	}
	b.WriteString("]")
	_, p, err := parsePatternWith(b.String(), false)
	if err == nil || !strings.Contains(err.Error(), "nest limit") {
		t.Fatalf("want the nest limit error, got %v", err)
	}
	if p.classOps >= nestLimit {
		t.Errorf("evaluated %d class operations; the height limit allows fewer than %d", p.classOps, nestLimit)
	}
	// Just inside the limit: 249 operations still build; 250 do not (the bracket is the 250th level up).
	for _, n := range []int{10, 200, 249} {
		var c strings.Builder
		c.WriteString("[a")
		for i := 0; i < n; i++ {
			c.WriteString("~~b")
		}
		c.WriteString("]")
		if _, err := parsePattern(c.String(), false); err != nil {
			t.Errorf("%d operations: %v", n, err)
		}
	}
	if _, err := parsePattern("[a"+strings.Repeat("~~b", 250)+"]", false); err == nil {
		t.Error("250 operations: want the nest limit error")
	}
	// A syntax error after the chain is still the error reported (as in Rust, which parses first).
	if _, err := parsePattern(strings.Repeat("[a"+strings.Repeat("~~b", 300)+"]", 1)+"[", false); err == nil || strings.Contains(err.Error(), "nest limit") {
		t.Errorf("want the unclosed class error, got %v", err)
	}
}
