package lexexact

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A step code of 1 decodes to target -1 and is refused in either step table.
func TestStepCodeWithNegativeTargetIsRefused(t *testing.T) {
	for _, tab := range []*Tables{
		{Version: Version, Main: Func{Accept: []uint32{0}, OnEOF: []uint32{1}, OnInvalid: []uint32{0}}},
		{Version: Version, Main: Func{Accept: []uint32{0}, OnEOF: []uint32{0}, OnInvalid: []uint32{1}}},
	} {
		if err := tab.Validate(1, 0); !errors.Is(err, ErrFormat) {
			t.Errorf("Validate: %v", err)
		}
		if _, err := tab.Encode(); !errors.Is(err, ErrFormat) {
			t.Errorf("Encode: %v", err)
		}
	}
}

// Validate, Encode and Decode share one validation; bounds are compared as uint64.
func TestSharedValidation(t *testing.T) {
	own := func(f Func) *Tables { return &Tables{Version: Version, Main: f} }
	base := func() Func {
		return Func{Accept: []uint32{0}, OnEOF: []uint32{0}, OnInvalid: []uint32{0},
			Trans: [][]Range{{{Lo: 0, Hi: 5, Target: 0}}}, Start: []uint32{0}}
	}
	bad := map[string]*Tables{}
	f := base()
	f.Start = []uint32{1}
	bad["start out of range"] = own(f)
	f = base()
	f.Start = []uint32{0xffffffff}
	bad["start 0xffffffff"] = own(f)
	f = base()
	f.Trans = [][]Range{{{Lo: 0, Hi: 5, Target: 1}}}
	bad["range target out of range"] = own(f)
	f = base()
	f.Trans = [][]Range{{{Lo: 5, Hi: 0, Target: 0}}}
	bad["reversed range"] = own(f)
	f = base()
	f.Trans = [][]Range{{{Lo: 0, Hi: 0x110000, Target: 0}}}
	bad["range above U+10FFFF"] = own(f)
	bad["end-of-input self cycle"] = own(Func{Accept: []uint32{0}, OnEOF: []uint32{2}, OnInvalid: []uint32{0}})
	if err := own(base()).Validate(1, 0); err != nil {
		t.Fatalf("good own table: %v", err)
	}
	for name, tab := range bad {
		if err := tab.Validate(1, 0); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: Validate = %v", name, err)
		}
		if _, err := tab.Encode(); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: Encode = %v", name, err)
		}
	}
}

// Parse needs the grammar's own header.
func TestParseRequiresHeader(t *testing.T) {
	c := cfile("", "case 0:\n if (eof) ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();")
	if _, err := parseInc(c, ""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("empty header: err = %v", err)
	}
	if _, err := parseInc(c, "  \n"); err == nil {
		t.Error("blank header accepted")
	}
}

// Constants outside the signed domain and numbers that do not fit their C type are refused.
func TestLiteralDomain(t *testing.T) {
	for name, c := range map[string]struct{ body, want string }{
		"unsigned compare":   {"case 0:\n if (lookahead == 0xffffffff) ADVANCE(0);\n END_STATE();", "31-bit"},
		"above int":          {"case 0:\n if (lookahead == 2147483648) ADVANCE(0);\n END_STATE();", "31-bit"},
		"symbol too big":     {"case 0:\n ACCEPT_TOKEN(65536);\n END_STATE();", "TSSymbol"},
		"target too big":     {"case 0:\n if (lookahead == 'a') ADVANCE(65536);\n END_STATE();", "TSStateId"},
		"map target too big": {"case 0:\n ADVANCE_MAP('a', 65536);\n END_STATE();", "uint16_t"},
		"negative char":      {"case 0:\n if (lookahead == '\\xff') ADVANCE(0);\n END_STATE();", "escape"},
		"case above uint16":  {"case 65536:\n END_STATE();", "TSStateId"},
	} {
		_, err := parseInc(cfile("", c.body), StandardHeader)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, c.want)
		}
	}
	if _, err := parseInc(cfile("", "case 0:\n ACCEPT_TOKEN(65535);\n if (lookahead == 2147483647) ADVANCE(0);\n END_STATE();"), StandardHeader); err != nil {
		t.Errorf("largest values: %v", err)
	}
}

// A function that holds only a map state does not need state 0.
func TestMapOnlyFunctionWithoutStateZero(t *testing.T) {
	c := cfile("", "case 1:\n ADVANCE_MAP('a', 1);\n END_STATE();")
	src := mustParse(t, c, StandardHeader)
	if got := stepOf(src.Main, 1, false, 'a'); got != "adv>1" {
		t.Errorf("step = %s", got)
	}
	if _, err := parseInc(cfile("", "case 1:\n ADVANCE_MAP('a', 2);\n END_STATE();"), StandardHeader); err == nil {
		t.Error("a map target without a case was accepted")
	}
}

// The checked build compares the blob transitions with the C text.
func TestBuildChecksBlobTransitions(t *testing.T) {
	c := cfile("", "case 0:\n if (lookahead == 'a') ADVANCE(1);\n END_STATE();\n case 1:\n END_STATE();")
	src := mustParse(t, c, StandardHeader)
	good := [][]Transition{{{Lo: 'a', Hi: 'a', Next: 1}}, nil}
	if _, err := Build(src, c, good, nil); err != nil {
		t.Fatalf("matching blob: %v", err)
	}
	omit := [][]Transition{nil, nil}
	if _, err := Build(src, c, omit, nil); err == nil || !strings.Contains(err.Error(), "blob differs") {
		t.Errorf("blob without 'a': err = %v", err)
	}
	if _, err := Build(src, c, good, [][]Transition{nil}); err == nil {
		t.Error("keyword states without a keyword function accepted")
	}
	if _, err := BuildUnchecked(src, c); err != nil {
		t.Errorf("BuildUnchecked: %v", err)
	}
}

// parseInc(EmitC(record)) gives the whole record back, the emitted pair compiles,
// and the limits of the representable domain round-trip.
func emitFixtures(t *testing.T) map[string]*Tables {
	mk := fullExample(t)
	kw := fullExample(t)
	kw.Keyword = Func{Accept: []uint32{2, 1}, OnEOF: []uint32{4, 0}, OnInvalid: []uint32{0, 3}}
	return map[string]*Tables{"mirror": mk, "mirror with keyword": kw, "own": ownExample(t)}
}

func TestEmitCRoundTrip(t *testing.T) {
	for name, rec := range emitFixtures(t) {
		c, h, err := EmitC(rec)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		src, err := parseInc(c, h)
		if err != nil {
			t.Fatalf("%s: parseInc(EmitC): %v\n%s", name, err, c)
		}
		back, err := BuildUnchecked(src, c)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(back.Main, rec.Main) {
			t.Errorf("%s: main differs:\n got %+v\nwant %+v", name, back.Main, rec.Main)
		}
		if !reflect.DeepEqual(back.Keyword, rec.Keyword) {
			t.Errorf("%s: keyword differs:\n got %+v\nwant %+v", name, back.Keyword, rec.Keyword)
		}
		// Emitted ranges, read by the C-text evaluator, equal the record's.
		for fn, pair := range map[string]struct {
			f *Func
			l *Lexer
		}{"main": {&rec.Main, src.Main}, "keyword": {&rec.Keyword, src.Keyword}} {
			if pair.f.Trans == nil {
				continue
			}
			blob := make([][]Transition, len(pair.f.Trans))
			for i, rs := range pair.f.Trans {
				for _, r := range rs {
					blob[i] = append(blob[i], Transition{Lo: rune(r.Lo), Hi: rune(r.Hi), Next: int(r.Target), Skip: r.Skip})
				}
			}
			if err := pair.l.CheckTransitions(blob); err != nil {
				t.Errorf("%s %s: emitted ranges differ from the record's: %v", name, fn, err)
			}
		}
	}
}

func ownExample(t *testing.T) *Tables {
	t.Helper()
	return &Tables{Version: Version,
		Main: Func{
			Accept: []uint32{0, 3, 1}, OnEOF: []uint32{2 << 1, 0, 0}, OnInvalid: []uint32{0, 1<<1 | 1, 0},
			Trans: [][]Range{
				{{Lo: 0, Hi: 0, Target: 1}, {Lo: 'a', Hi: 'z', Target: 0, Skip: true}},
				{{Lo: 0x100, Hi: 0x10FFFF, Target: 2}}, nil},
			Start: []uint32{0, 1},
		},
		Keyword: Func{
			Accept: []uint32{4, 0}, OnEOF: []uint32{2 << 1, 0}, OnInvalid: []uint32{0, 1 << 1},
			Trans: [][]Range{{{Lo: 0, Hi: 0x7f, Target: 1}}, {{Lo: 'a', Hi: 'a', Target: 0, Skip: true}}},
			Start: []uint32{1, 0},
		},
	}
}

// The emitted C and its header compile (needs a C compiler; skipped without one).
func TestEmitCCompiles(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler")
	}
	fixtures := emitFixtures(t)
	fixtures["end token only"] = &Tables{Version: Version, Main: Func{Accept: []uint32{1}, OnEOF: []uint32{0}, OnInvalid: []uint32{0}}}
	for name, rec := range fixtures {
		compileEmitted(t, cc, name, rec)
	}
}

func compileEmitted(t *testing.T, cc, name string, rec *Tables) {
	t.Helper()
	c, h, err := EmitC(rec)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tree_sitter"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tree_sitter", "parser.h"), []byte(h), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lex.c"), []byte(c), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(cc, "-std=c11", "-fsyntax-only", "-I", dir, filepath.Join(dir, "lex.c"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("%s: cc -fsyntax-only: %v\n%s", name, err, out)
		return
	}
	t.Logf("%s: %s: ok", name, strings.Join(cmd.Args[:4], " ")+" lex.c")
}

// json's committed record, when the tree has it, round-trips through C and compiles.
func TestEmitCJsonRecord(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "grammars", "runtime", "lex_exact", "json.lexx"))
	if err != nil {
		t.Skip("no committed json record in this tree")
	}
	rec, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	c, h, err := EmitC(rec)
	if err != nil {
		t.Fatal(err)
	}
	src, err := parseInc(c, h)
	if err != nil {
		t.Fatal(err)
	}
	back, err := BuildUnchecked(src, c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Main, rec.Main) || !reflect.DeepEqual(back.Keyword, rec.Keyword) {
		t.Error("json record does not round-trip through C")
	}
	if cc, err := exec.LookPath("cc"); err == nil {
		compileEmitted(t, cc, "json", rec)
	}
}

// The limits of the representable domain round-trip; one past them is refused.
func TestEmitCDomainLimits(t *testing.T) {
	zero := func(n int) Func {
		return Func{Accept: make([]uint32, n), OnEOF: make([]uint32, n), OnInvalid: make([]uint32, n)}
	}
	atLimit := zero(65536)
	atLimit.Accept[65535] = 65536 // symbol 65535
	atLimit.OnEOF[0] = 65536 << 1 // step to state 65535
	own := zero(65536)
	own.Trans = make([][]Range, 65536)
	own.Trans[65535] = []Range{{Lo: 1, Hi: 2, Target: 65535}}
	own.Start = []uint32{65535}
	for name, rec := range map[string]*Tables{
		"65536 states, symbol 65535": {Version: Version, Main: atLimit},
		"65536 own states":           {Version: Version, Main: own},
	} {
		if err := rec.Validate(65536, 0); err != nil {
			t.Fatalf("%s: Validate: %v", name, err)
		}
		c, h, err := EmitC(rec)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		src, err := parseInc(c, h)
		if err != nil {
			t.Fatalf("%s: Parse: %v", name, err)
		}
		back, err := BuildUnchecked(src, c)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(back.Main, rec.Main) {
			t.Errorf("%s: round trip differs", name)
		}
	}
	over := map[string]*Tables{
		"accept code 65537": {Version: Version, Main: Func{Accept: []uint32{65537}, OnEOF: []uint32{0}, OnInvalid: []uint32{0}}},
		"65537 states":      {Version: Version, Main: zero(65537)},
		"keyword 65537":     {Version: Version, Main: zero(1), Keyword: zero(65537)},
	}
	for name, rec := range over {
		if err := rec.Validate(len(rec.Main.Accept), len(rec.Keyword.Accept)); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: Validate = %v", name, err)
		}
		if _, err := rec.Encode(); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: Encode = %v", name, err)
		}
		if _, _, err := EmitC(rec); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: EmitC = %v", name, err)
		}
	}
}

// A comparison with a Boolean operand is refused.
func TestBooleanComparisonRefused(t *testing.T) {
	for _, cond := range []string{"lookahead == (lookahead < 'a')", "(lookahead < 'a') == lookahead", "eof == 1", "lookahead != !eof"} {
		c := cfile("", "case 0:\n if ("+cond+") ADVANCE(0);\n END_STATE();")
		if _, err := parseInc(c, StandardHeader); err == nil {
			t.Errorf("%q accepted", cond)
		}
	}
}

// Recursion and long helper chains are refused; short chains work.
func TestHelperCallGraph(t *testing.T) {
	// chain(n) defines h(n-1) first and h0 last, so that each call goes to an earlier definition.
	chain := func(n int) string {
		var b strings.Builder
		for i := n - 1; i >= 0; i-- {
			b.WriteString("static inline bool h" + itoa(i) + "(int32_t c) { return ")
			if i == n-1 {
				b.WriteString("true; }\n")
			} else {
				b.WriteString("h" + itoa(i+1) + "(c); }\n")
			}
		}
		return b.String()
	}
	body := "case 0:\n if (h0(lookahead)) ADVANCE(0);\n END_STATE();"
	src := mustParse(t, cfile(chain(10), body), StandardHeader)
	if got := stepOf(src.Main, 0, false, 'a'); got != "adv>0" {
		t.Errorf("chain of 10: %s", got)
	}
	if _, err := parseInc(cfile(chain(35), body), StandardHeader); err == nil || !strings.Contains(err.Error(), "nest deeper") {
		t.Errorf("chain of 35: err = %v", err)
	}
	rec := "static inline bool r(int32_t c) { return r(c); }\n"
	if _, err := parseInc(cfile(rec, "case 0:\n if (r(lookahead)) ADVANCE(0);\n END_STATE();"), StandardHeader); err == nil {
		t.Error("recursive helper accepted")
	}
}
