package lexexact

import (
	"crypto/sha256"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The tests below use small synthetic parser.c text in the shapes of real
// states: ADVANCE_MAP with key 0 against the guarded zero test
// (TestMapKeyZeroAndGuardedZero), a comparison chain against a set test
// (TestInvalidByteComparisonVersusSet), an accept followed by an end-of-input step
// to the end-token state (TestAcceptThenEOFStep), and a negated set function that
// is true at 0 (TestNegatedSetFunction).

// cfile builds a small parser.c: the symbol enum, optional extra text (sets
// and functions) and a ts_lex function whose cases are body.
func cfile(extra, body string) string {
	return `#include "tree_sitter/parser.h"
enum ts_symbol_identifiers {
  sym_a = 1,
  sym_b = 2,
  aux_sym_c_token1 = 3,
};
` + extra + `
static bool ts_lex(TSLexer *lexer, TSStateId state) {
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
` + body + `
    default:
      return false;
  }
}
`
}

func mustParse(t *testing.T, c, h string) *Source {
	t.Helper()
	s, err := parseInc(c, h)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

func stepOf(l *Lexer, state int, eof bool, la int32) string {
	s, ok := l.Step(state, eof, la)
	if !ok {
		return "none"
	}
	if s.Skip {
		return "skip>" + itoa(s.Target)
	}
	return "adv>" + itoa(s.Target)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// ADVANCE_MAP with key 0 matches at the end of input (lookahead 0); the
// guarded form `(!eof && lookahead == 00)` does not. Both look like the range
// {0,0} in a blob.
func TestMapKeyZeroAndGuardedZero(t *testing.T) {
	body := `
    case 0:
      ADVANCE_MAP(
        0, 2,
        '\n', 3,
      );
      END_STATE();
    case 1:
      if ((!eof && lookahead == 00)) ADVANCE(2);
      if (lookahead == '\n') ADVANCE(3);
      END_STATE();
    case 2:
      ACCEPT_TOKEN(sym_a);
      END_STATE();
    case 3:
      ACCEPT_TOKEN(sym_b);
      END_STATE();`
	l := mustParse(t, cfile("", body), StandardHeader).Main
	for _, c := range []struct {
		state int
		eof   bool
		la    int32
		want  string
	}{
		{0, true, 0, "adv>2"},  // key 0 fires at the end of input
		{1, true, 0, "none"},   // guarded form does not
		{0, false, 0, "adv>2"}, // a real NUL byte
		{1, false, 0, "adv>2"}, // a real NUL byte
		{0, false, -1, "none"}, // -1 never matches a uint16_t key
		{1, false, -1, "none"}, // nor an equality test
		{1, false, '\n', "adv>3"},
	} {
		if got := stepOf(l, c.state, c.eof, c.la); got != c.want {
			t.Errorf("state %d eof=%v la=%d: got %s, want %s", c.state, c.eof, c.la, got, c.want)
		}
	}
}

// A chain with `lookahead != 0 && ...` is true for -1 and false at the end of
// input; `(!eof && set_contains(...))` is false for both.
func TestInvalidByteComparisonVersusSet(t *testing.T) {
	extra := `static const TSCharacterRange sym_x_character_set_1[] = {
  {'+', '-'}, {'0', '9'}, {'a', 'z'},
};
`
	body := `
    case 0:
      if (lookahead != 0 &&
          (lookahead < '&' || ')' < lookahead) &&
          lookahead != '?') ADVANCE(1);
      END_STATE();
    case 1:
      ACCEPT_TOKEN(sym_a);
      if ((!eof && set_contains(sym_x_character_set_1, 3, lookahead))) ADVANCE(1);
      END_STATE();`
	l := mustParse(t, cfile(extra, body), StandardHeader).Main
	for _, c := range []struct {
		state int
		eof   bool
		la    int32
		want  string
	}{
		{0, false, -1, "adv>1"},
		{0, true, 0, "none"},
		{1, false, -1, "none"},
		{1, true, 0, "none"},
		{1, false, 'q', "adv>1"},
		{1, false, ',', "adv>1"},
		{1, false, '.', "none"},
	} {
		if got := stepOf(l, c.state, c.eof, c.la); got != c.want {
			t.Errorf("state %d eof=%v la=%d: got %s, want %s", c.state, c.eof, c.la, got, c.want)
		}
	}
	tab, err := BuildUnchecked(mustParse(t, cfile(extra, body), StandardHeader), "x")
	if err != nil {
		t.Fatal(err)
	}
	if tab.Main.OnInvalid[0] == 0 || tab.Main.OnInvalid[1] != 0 {
		t.Errorf("OnInvalid = %v", tab.Main.OnInvalid)
	}
}

// Two accepts at one position: state 0 accepts and steps on eof to state 1,
// which accepts the end token (symbol 0, stored as 1).
func TestAcceptThenEOFStep(t *testing.T) {
	body := `
    case 0:
      ACCEPT_TOKEN(sym_a);
      if (eof) ADVANCE(1);
      END_STATE();
    case 1:
      ACCEPT_TOKEN(ts_builtin_sym_end);
      END_STATE();`
	src := mustParse(t, cfile("", body), StandardHeader)
	tab, err := BuildUnchecked(src, "x")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tab.Main.Accept, []uint32{2, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("Accept = %v, want %v", got, want)
	}
	if got, want := tab.Main.OnEOF, []uint32{2 << 1, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("OnEOF = %v, want %v", got, want)
	}
}

// A negated character-set function whose value is true at 0: C stops at the
// end of input; the value for -1 follows the function body.
func TestNegatedSetFunction(t *testing.T) {
	extra := `static inline bool sym_plain_value_character_set_1(int32_t c) {
  return (c < ';'
    ? (c < ' '
      ? (c < '\t'
        ? c == 0
        : c <= '\r')
      : (c <= '!' || (c < ','
        ? (c >= '(' && c <= ')')
        : c <= ',')))
    : (c <= ';' || (c < '{'
      ? (c < ']'
        ? c == '['
        : c <= ']')
      : (c <= '{' || c == '}'))));
}
`
	body := `
    case 0:
      ACCEPT_TOKEN(sym_a);
      if (lookahead == '.') ADVANCE(1);
      if (!sym_plain_value_character_set_1(lookahead)) ADVANCE(1);
      END_STATE();
    case 1:
      ACCEPT_TOKEN(sym_b);
      END_STATE();`
	l := mustParse(t, cfile(extra, body), StandardHeader).Main
	if got := stepOf(l, 0, true, 0); got != "none" {
		t.Errorf("end of input: %s, want none", got)
	}
	if got := stepOf(l, 0, false, -1); got != "adv>1" {
		t.Errorf("-1: %s, want adv>1", got)
	}
	if got := stepOf(l, 0, false, 'x'); got != "adv>1" {
		t.Errorf("'x': %s", got)
	}
	if got := stepOf(l, 0, false, '('); got != "none" {
		t.Errorf("'(': %s", got)
	}
}

func TestSkipFlagAndPackedSteps(t *testing.T) {
	body := `
    case 0:
      if (eof) SKIP(1);
      if (lookahead == ' ') SKIP(1)
      END_STATE()
    case 1:
      END_STATE();`
	src := mustParse(t, cfile("", body), StandardHeader)
	tab, err := BuildUnchecked(src, "x")
	if err != nil {
		t.Fatal(err)
	}
	if tab.Main.OnEOF[0] != 2<<1|1 {
		t.Errorf("OnEOF[0] = %d", tab.Main.OnEOF[0])
	}
	s, ok := UnpackStep(tab.Main.OnEOF[0])
	if !ok || s != (Step{Target: 1, Skip: true}) {
		t.Errorf("UnpackStep = %v %v", s, ok)
	}
	if _, ok := UnpackStep(0); ok {
		t.Error("UnpackStep(0) should report no step")
	}
}

func TestParseRefusesUnknownConstructs(t *testing.T) {
	for name, body := range map[string]string{
		"bit and":        "case 0:\n if (lookahead & 1) ADVANCE(0);\n END_STATE();",
		"minus one":      "case 0:\n if (lookahead == -1) ADVANCE(0);\n END_STATE();",
		"missing target": "case 0:\n if (lookahead == 'a') ADVANCE(7);\n END_STATE();",
		"accept late":    "case 0:\n if (lookahead == 'a') ADVANCE(0);\n ACCEPT_TOKEN(sym_a);\n END_STATE();",
		"unknown symbol": "case 0:\n ACCEPT_TOKEN(sym_nope);\n END_STATE();",
		"two cases":      "case 0:\n END_STATE();\n case 0:\n END_STATE();",
		"unknown call":   "case 0:\n if (nope_character_set_1(lookahead)) ADVANCE(0);\n END_STATE();",
	} {
		if _, err := parseInc(cfile("", body), StandardHeader); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestBuildRejectsEOFCycle(t *testing.T) {
	body := `
    case 0:
      if (eof) ADVANCE(1);
      END_STATE();
    case 1:
      if (eof) ADVANCE(0);
      END_STATE();`
	src := mustParse(t, cfile("", body), StandardHeader)
	if _, err := BuildUnchecked(src, "x"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err = %v", err)
	}
}

func TestKeywordFunctionAndHoles(t *testing.T) {
	c := cfile("", "case 0:\n ACCEPT_TOKEN(sym_a);\n if (lookahead == 'x') ADVANCE(2);\n END_STATE();\n case 2:\n ACCEPT_TOKEN(sym_b);\n END_STATE();") + `
static bool ts_lex_keywords(TSLexer *lexer, TSStateId state) {
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
    case 0:
      if (eof) ADVANCE(1);
      END_STATE();
    case 1:
      ACCEPT_TOKEN(sym_b);
      END_STATE();
    default:
      return false;
  }
}
`
	src := mustParse(t, c, StandardHeader)
	tab, err := BuildUnchecked(src, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Main.Accept) != 3 || tab.Main.Accept[1] != 0 || tab.Main.Accept[2] != 3 {
		t.Errorf("main Accept = %v (state 1 is a hole)", tab.Main.Accept)
	}
	if len(tab.Keyword.Accept) != 2 || tab.Keyword.OnEOF[0] != 2<<1 {
		t.Errorf("keyword record = %+v", tab.Keyword)
	}
	if tab.SourceSHA256 != sha256.Sum256([]byte(c)) {
		t.Error("SourceSHA256 is not the hash of the text")
	}
}

func fullExample(t *testing.T) *Tables {
	t.Helper()
	body := `
    case 0:
      ACCEPT_TOKEN(sym_a);
      if (eof) ADVANCE(1);
      ADVANCE_MAP(
        'a', 2,
        0, 3,
      );
      if (lookahead != 0 && lookahead != '\n') SKIP(2);
      END_STATE();
    case 1:
      ACCEPT_TOKEN(ts_builtin_sym_end);
      END_STATE();
    case 2:
      if (lookahead > ' ') ADVANCE(2);
      END_STATE();
    case 3:
      ACCEPT_TOKEN(aux_sym_c_token1);
      END_STATE();`
	c := cfile("", body)
	tab, err := BuildUnchecked(mustParse(t, c, StandardHeader), c)
	if err != nil {
		t.Fatal(err)
	}
	tab.BlobSHA256 = sha256.Sum256([]byte("blob"))
	return tab
}

func TestRoundTrip(t *testing.T) {
	tab := fullExample(t)
	// The extractor must have filled the tables: known values of fullExample.
	if want := []uint32{2, 1, 0, 4}; !reflect.DeepEqual(tab.Main.Accept, want) {
		t.Fatalf("Accept = %v, want %v", tab.Main.Accept, want)
	}
	if want := []uint32{4, 0, 0, 0}; !reflect.DeepEqual(tab.Main.OnEOF, want) {
		t.Fatalf("OnEOF = %v, want %v", tab.Main.OnEOF, want)
	}
	if want := []uint32{7, 0, 0, 0}; !reflect.DeepEqual(tab.Main.OnInvalid, want) {
		t.Fatalf("OnInvalid = %v, want %v", tab.Main.OnInvalid, want)
	}
	b, err := tab.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, tab) {
		t.Errorf("round trip differs:\n got %+v\nwant %+v", got, tab)
	}
	if err := got.Validate(4, 0); err != nil {
		t.Errorf("Validate: %v", err)
	}
	if err := got.Validate(5, 0); !errors.Is(err, ErrFormat) {
		t.Errorf("Validate with a wrong state count: %v", err)
	}
	b2, _ := got.Encode()
	if string(b) != string(b2) {
		t.Error("encoding is not deterministic")
	}
}

func TestRoundTripOwnRanges(t *testing.T) {
	tab := &Tables{Version: Version}
	tab.Main = Func{
		Accept: []uint32{0, 3}, OnEOF: []uint32{2 << 1, 0}, OnInvalid: []uint32{0, 1<<1 | 1},
		Trans: [][]Range{{{Lo: 0, Hi: 0x10FFFF, Target: 1, Skip: true}}, {{Lo: 'a', Hi: 'z', Target: 0}, {Lo: 0x100, Hi: 0x100, Target: 1}}},
		Start: []uint32{0, 1, 0},
	}
	b, err := tab.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, tab) {
		t.Errorf("differs:\n got %+v\nwant %+v", got, tab)
	}
	if err := got.Validate(99, 0); err != nil {
		t.Errorf("own ranges are exempt from the state-count check: %v", err)
	}
}

func TestDecodeRejects(t *testing.T) {
	ex := fullExample(t)
	if ex.Main.OnEOF[0] == 0 || ex.Main.Accept[0] == 0 || ex.Main.OnInvalid[0] == 0 {
		t.Fatalf("extraction returned zero tables: %+v", ex.Main)
	}
	good, _ := ex.Encode()
	mut := func(f func(b []byte) []byte) []byte { return f(append([]byte(nil), good...)) }
	hdr := len(Magic) + 1 + 64
	cases := map[string][]byte{
		"empty":         nil,
		"bad magic":     mut(func(b []byte) []byte { b[0] = 'X'; return b }),
		"bad version":   mut(func(b []byte) []byte { b[len(Magic)] = 2; return b }),
		"truncated":     good[:len(good)-1],
		"trailing byte": append(append([]byte(nil), good...), 0),
		"huge count":    mut(func(b []byte) []byte { b[hdr] = 0xff; return append(b[:hdr+1], 0xff, 0xff, 0xff, 0x0f) }),
		// state count 4; the first OnEOF entry is the 5th varint after it
		"step code 1 in OnEOF":     append(append([]byte(nil), good[:hdr]...), 1, 0, 1, 0, 0, 0, 0),
		"step code 1 in OnInvalid": append(append([]byte(nil), good[:hdr]...), 1, 0, 0, 1, 0, 0, 0),
		"target outside":           mut(func(b []byte) []byte { b[hdr+1+4+0] = 0x7e; return b }),
	}
	for name, b := range cases {
		if _, err := Decode(b); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := Decode(good); err != nil {
		t.Errorf("good record: %v", err)
	}
}

func TestRegistry(t *testing.T) {
	var k [32]byte
	k[0] = 7
	t.Cleanup(func() {
		regMu.Lock()
		delete(reg, k)
		regMu.Unlock()
	})
	if _, ok := Lookup(k); ok {
		t.Fatal("unexpected record")
	}
	Register(k, []byte("abc"))
	if b, ok := Lookup(k); !ok || string(b) != "abc" {
		t.Errorf("Lookup = %q %v", b, ok)
	}
	defer func() {
		if recover() == nil {
			t.Error("second Register did not panic")
		}
	}()
	Register(k, []byte("def"))
}

func TestCheckTransitions(t *testing.T) {
	body := `
    case 0:
      ACCEPT_TOKEN(sym_a);
      if (eof) ADVANCE(1);
      ADVANCE_MAP(
        'a', 1,
        'b', 1,
      );
      if (('\t' <= lookahead && lookahead <= '\r') ||
          lookahead == ' ') SKIP(2);
      if (lookahead != 0 && lookahead > 'z') ADVANCE(2);
      END_STATE();
    case 1:
      END_STATE();
    case 2:
      END_STATE();`
	l := mustParse(t, cfile("", body), StandardHeader).Main
	blob := [][]Transition{
		{
			{Lo: 'a', Hi: 'b', Next: 1},
			{Lo: '\t', Hi: '\r', Next: 2, Skip: true},
			{Lo: ' ', Hi: ' ', Next: 2, Skip: true},
			{Lo: 'z' + 1, Hi: MaxCodePoint, Next: 2},
		},
		nil, nil,
	}
	if err := l.CheckTransitions(blob); err != nil {
		t.Fatalf("exact blob: %v", err)
	}
	blob[0][3].Lo = 'z' + 2
	if err := l.CheckTransitions(blob); err == nil {
		t.Error("an altered range was not found")
	}
	blob[0][3].Lo = 'z' + 1
	blob[0][1].Skip = false
	if err := l.CheckTransitions(blob); err == nil {
		t.Error("an altered skip flag was not found")
	}
	if err := l.CheckTransitions(blob[:2]); err == nil {
		t.Error("a wrong state count was not found")
	}
}

// The package must not name a grammar (repository rule D6): the engine file
// text holds no language name.
func TestNoLanguageNames(t *testing.T) {
	for _, name := range []string{"earthfile", "fennel", "caddy", "scss", "json", "python", "promql", "nginx"} {
		for _, f := range nonTestFiles(t) {
			b, err := readFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.ToLower(string(b)), name) {
				t.Errorf("%s names %s", f, name)
			}
		}
	}
}

// nonTestFiles lists the package's Go files that are not tests.
func nonTestFiles(t *testing.T) []string {
	t.Helper()
	all, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range all {
		if !strings.HasSuffix(f, "_test.go") {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		t.Fatal("no source files found")
	}
	return out
}
