package lexexact

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
)

// StandardHeader is the text of a tree_sitter/parser.h that tree-sitter's
// generator ships and that this package accepts (the header of the ada grammar;
// 109 of the pinned grammars use a byte-equal file). EmitC returns it next to the
// C text: the emitted source includes it as "tree_sitter/parser.h", and a C
// compiler compiles the pair.
//
//go:embed standard_parser.h
var StandardHeader string

// EmitC writes a C lexer for the record, in the forms that Parse reads, and
// the header to read it with. Per state: ACCEPT_TOKEN, then `if (eof)` with the
// end-of-input step, then one guarded step per own range in order (each guard
// starts with `!eof`, so that a range holding code point 0 does not fire at the
// end of input), then `if (lookahead < 0)` with the step for an ill-formed
// byte. A record without own ranges gets no steps for valid code points.
// Parse(EmitC(t)) followed by BuildUnchecked gives back the accept and step
// tables of t.
func EmitC(t *Tables) (parserC, parserH string, err error) {
	if err := t.validate(); err != nil {
		return "", "", err
	}
	syms := map[uint32]bool{}
	for _, f := range []*Func{&t.Main, &t.Keyword} {
		for _, a := range f.Accept {
			if a > 1 {
				syms[a-1] = true
			}
		}
	}
	var b strings.Builder
	b.WriteString("#include \"tree_sitter/parser.h\"\n\n")
	names := make([]uint32, 0, len(syms))
	for s := range syms {
		names = append(names, s)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	if len(names) > 0 { // no empty enum
		b.WriteString("enum ts_symbol_identifiers {\n")
		for _, s := range names {
			fmt.Fprintf(&b, "  sym_%d = %d,\n", s, s)
		}
		b.WriteString("};\n\n")
	}
	emitStart(&b, "ts_lex_start", &t.Main)
	emitStart(&b, "ts_lex_keywords_start", &t.Keyword)
	emitFunc(&b, "ts_lex", &t.Main)
	if len(t.Keyword.Accept) > 0 {
		emitFunc(&b, "ts_lex_keywords", &t.Keyword)
	}
	return b.String(), StandardHeader, nil
}

func emitFunc(b *strings.Builder, name string, f *Func) {
	fmt.Fprintf(b, "static bool %s(TSLexer *lexer, TSStateId state) {\n  START_LEXER();\n  eof = lexer->eof(lexer);\n  switch (state) {\n", name)
	step := func(v uint32) string {
		s, _ := UnpackStep(v)
		kind := "ADVANCE"
		if s.Skip {
			kind = "SKIP"
		}
		return fmt.Sprintf("%s(%d)", kind, s.Target)
	}
	for i := range f.Accept {
		fmt.Fprintf(b, "    case %d:\n", i)
		switch a := f.Accept[i]; {
		case a == 1:
			b.WriteString("      ACCEPT_TOKEN(ts_builtin_sym_end);\n")
		case a > 1:
			fmt.Fprintf(b, "      ACCEPT_TOKEN(sym_%d);\n", a-1)
		}
		if f.OnEOF[i] != 0 {
			fmt.Fprintf(b, "      if (eof) %s;\n", step(f.OnEOF[i]))
		}
		if f.Trans != nil {
			for _, r := range f.Trans[i] {
				tr := Step{Target: int(r.Target), Skip: r.Skip}
				kind := "ADVANCE"
				if tr.Skip {
					kind = "SKIP"
				}
				if r.Lo == r.Hi {
					fmt.Fprintf(b, "      if ((!eof && lookahead == %d)) %s(%d);\n", r.Lo, kind, tr.Target)
				} else {
					fmt.Fprintf(b, "      if ((!eof && (%d <= lookahead && lookahead <= %d))) %s(%d);\n", r.Lo, r.Hi, kind, tr.Target)
				}
			}
		}
		if f.OnInvalid[i] != 0 {
			fmt.Fprintf(b, "      if (lookahead < 0) %s;\n", step(f.OnInvalid[i]))
		}
		b.WriteString("      END_STATE();\n")
	}
	b.WriteString("    default:\n      return false;\n  }\n}\n\n")
}

// emitStart writes the start array of an own-range function; it marks the
// function as an own table for Parse.
func emitStart(b *strings.Builder, name string, f *Func) {
	if f.Trans == nil {
		return
	}
	fmt.Fprintf(b, "static const uint16_t %s[] = {", name)
	for i, s := range f.Start {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(b, " %d", s)
	}
	b.WriteString(" };\n\n")
}
